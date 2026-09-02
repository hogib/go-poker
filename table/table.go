package table

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"ssh_holdem/game"
)

var errSessionGone = errors.New("session disconnected")

// Config is the table's house rules.
type Config struct {
	SmallBlind int
	BigBlind   int
	BuyIn      int

	// TurnTimeout is the shot clock. A player who lets it run out checks
	// if they can and folds otherwise.
	TurnTimeout time.Duration

	// HandDelay is the pause between hands so players can read the
	// showdown before the next deal.
	HandDelay time.Duration
}

func (c Config) withDefaults() Config {
	if c.SmallBlind <= 0 {
		c.SmallBlind = 10
	}
	if c.BigBlind <= 0 {
		c.BigBlind = c.SmallBlind * 2
	}
	if c.BuyIn <= 0 {
		c.BuyIn = c.BigBlind * 100
	}
	if c.TurnTimeout <= 0 {
		c.TurnTimeout = 30 * time.Second
	}
	if c.HandDelay <= 0 {
		c.HandDelay = 3 * time.Second
	}
	return c
}

// Table runs one poker table. Exactly one goroutine -- the one in Run --
// touches the embedded *game.Game. Everything reaching it from a session
// goroutine goes through a channel, and everything going back is a
// redacted snapshot pushed to the session's outbox.
type Table struct {
	cfg Config

	// game is owned by the Run goroutine. Nothing else may read or write
	// it, including ViewFor, which walks every seat.
	game *game.Game

	mu       sync.Mutex
	sessions map[string]*Session
	lastView map[string]game.PlayerView

	// lastPublic is the most recent spectator snapshot. It lets a session
	// that has only just connected draw something immediately, without
	// calling ViewFor from its own goroutine.
	lastPublic game.PlayerView

	// banked holds the chips of players who have disconnected, keyed by
	// their SSH fingerprint. Without it a player who drops and comes
	// back is a brand new seat with a fresh buy-in, which is both a
	// worse experience and free money.
	//
	// Only the Run goroutine touches it, for the same reason it is the
	// only one that touches Player.Chips.
	banked map[string]int

	seatOps chan seatOp
	wake    chan struct{}

	turnMu sync.Mutex
	turn   *turnState
}

type seatOpKind int

const (
	opSit seatOpKind = iota
	opStand
	opRun
)

type seatOp struct {
	kind    seatOpKind
	session *Session

	// fn is set for opRun: a closure the Run goroutine executes between
	// hands. It exists so callers can read table state that only that
	// goroutine may touch, without a lock that would have to be taken on
	// every chip movement.
	fn func()
}

type turnState struct {
	sessionID string
	replies   chan game.Decision
}

func New(cfg Config) *Table {
	cfg = cfg.withDefaults()

	g := game.NewGame(cfg.SmallBlind, cfg.BigBlind)
	g.TurnTimeout = cfg.TurnTimeout

	t := &Table{
		cfg:      cfg,
		game:     &g,
		sessions: make(map[string]*Session),
		lastView: make(map[string]game.PlayerView),
		banked:   make(map[string]int),
		seatOps:  make(chan seatOp, 32),
		wake:     make(chan struct{}, 1),
	}

	g.Watch = t

	return t
}

func (t *Table) Config() Config { return t.cfg }

// Join connects a player as a spectator. Taking a seat is a separate
// step, so a new arrival lands in the lobby rather than being dealt into
// whatever is already going on.
//
// Reconnecting with the same ID reattaches to the existing session rather
// than creating a new one, so a dropped connection costs a player their
// view and not their chips -- and a turn already in flight survives it.
func (t *Table) Join(id, name string, notify func(any)) *Session {
	t.mu.Lock()

	if existing, ok := t.sessions[id]; ok {
		existing.attach(notify)
		view, hasView := t.lastView[id]
		lobby := t.lobbyLocked(existing)
		t.mu.Unlock()

		if hasView {
			existing.send(StateMsg{View: view})
		}
		existing.send(lobby)
		return existing
	}

	// The stack is left at zero here and filled in by the Run goroutine
	// when the player is seated, since that goroutine owns every chip
	// count on the table.
	s := newSession(id, name, 0, notify)
	t.sessions[id] = s
	public := t.lastPublic
	lobby := t.lobbyLocked(s)
	t.mu.Unlock()

	if len(public.Seats) > 0 {
		s.send(StateMsg{View: public})
	}
	s.send(lobby)

	return s
}

// Refresh re-sends a session everything it needs to draw itself.
//
// A client that has only just finished starting up will have missed what
// Join pushed -- there was nowhere to deliver it yet -- and an idle table
// broadcasts nothing until something changes, so without this the lobby
// would sit on "connecting..." until the next hand.
func (t *Table) Refresh(sessionID string) bool {
	t.mu.Lock()
	s, ok := t.sessions[sessionID]
	if !ok {
		t.mu.Unlock()
		return false
	}

	view, hasView := t.lastView[sessionID]
	public := t.lastPublic
	lobby := t.lobbyLocked(s)
	t.mu.Unlock()

	switch {
	case hasView:
		s.send(StateMsg{View: view})
	case len(public.Seats) > 0:
		s.send(StateMsg{View: public})
	}
	s.send(lobby)

	return true
}

// Sit asks for a seat at the next hand. A player who is already seated,
// or who has no session, is ignored.
func (t *Table) Sit(sessionID string) bool {
	t.mu.Lock()
	s, ok := t.sessions[sessionID]
	t.mu.Unlock()

	if !ok {
		return false
	}

	// Whether they need chips, and whether they already have a seat, is
	// decided by the Run goroutine -- the only one allowed to look.
	t.seatOps <- seatOp{kind: opSit, session: s}
	t.signal()

	return true
}

// Stand gives up a seat without disconnecting: the player drops back to
// the lobby and keeps watching. It takes effect between hands, so chips
// already in the pot stay in play.
func (t *Table) Stand(sessionID string) bool {
	t.mu.Lock()
	s, ok := t.sessions[sessionID]
	t.mu.Unlock()

	if !ok {
		return false
	}

	t.seatOps <- seatOp{kind: opStand, session: s}
	t.signal()

	return true
}

// Rebuy is Sit under the name a busted player would look for. The stack
// is topped up when the seat is granted.
func (t *Table) Rebuy(sessionID string) bool { return t.Sit(sessionID) }

// lobbyLocked builds the menu's view of the table. The caller holds t.mu.
func (t *Table) lobbyLocked(s *Session) LobbyMsg {
	msg := LobbyMsg{
		Rules:    t.cfg,
		Watching: len(t.sessions),
		Seated:   len(t.lastPublic.Seats),
	}

	if s != nil {
		if view, ok := t.lastView[s.ID]; ok && view.Seat != game.SpectatorSeat {
			msg.YouAreSeated = true
			msg.YourChips = view.Seats[view.Seat].Chips
		}
	}

	// Watchers are everyone connected who is not in a seat.
	if msg.Watching -= msg.Seated; msg.Watching < 0 {
		msg.Watching = 0
	}

	return msg
}

func (t *Table) broadcastLobby() {
	t.mu.Lock()
	sessions := make([]*Session, 0, len(t.sessions))
	msgs := make([]LobbyMsg, 0, len(t.sessions))
	for _, s := range t.sessions {
		sessions = append(sessions, s)
		msgs = append(msgs, t.lobbyLocked(s))
	}
	t.mu.Unlock()

	for i, s := range sessions {
		s.send(msgs[i])
	}
}

// Leave disconnects a player. If a hand is in progress their seat stays
// until it finishes -- their chips are already in the pot -- but the turn
// they are blocking on gives up straight away.
func (t *Table) Leave(s *Session) {
	if s == nil {
		return
	}

	t.mu.Lock()
	if t.sessions[s.ID] == s {
		delete(t.sessions, s.ID)
		delete(t.lastView, s.ID)
	}
	t.mu.Unlock()

	s.close()

	t.seatOps <- seatOp{kind: opStand, session: s}
	t.signal()
}

// Act delivers a decision for the seat currently on the clock. It returns
// false when it is not that session's turn, which is what a UI should
// treat as "ignore that keypress".
func (t *Table) Act(sessionID string, d game.Decision) bool {
	t.turnMu.Lock()
	defer t.turnMu.Unlock()

	if t.turn == nil || t.turn.sessionID != sessionID {
		return false
	}

	select {
	case t.turn.replies <- d:
		return true
	default:
		// A decision is already in flight for this turn.
		return false
	}
}

// do runs fn on the Run goroutine and waits for it. It is the only safe
// way to read state the table owns -- chip counts, banked stacks, seat
// positions -- from anywhere else. It returns false if the table does not
// get to it before the deadline, which happens while a hand is in
// progress and someone is on the clock.
//
// fn must not call Join, Leave or Rebuy. Those enqueue onto the same
// channel fn is already running off, so a full queue would leave the Run
// goroutine blocked sending to a channel only it can drain, taking the
// whole table down with it.
func (t *Table) do(fn func(), within time.Duration) bool {
	done := make(chan struct{})

	select {
	case t.seatOps <- seatOp{kind: opRun, fn: func() { fn(); close(done) }}:
	case <-time.After(within):
		return false
	}
	t.signal()

	select {
	case <-done:
		return true
	case <-time.After(within):
		return false
	}
}

// Session returns the connected session with this id, or nil.
func (t *Table) Session(id string) *Session { return t.session(id) }

// SeatedCount is how many players are in seats, read from the cached
// public snapshot so it is safe to ask from any goroutine.
func (t *Table) SeatedCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.lastPublic.Seats)
}

// Watchers is how many sessions are connected but not seated.
func (t *Table) Watchers() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	if n := len(t.sessions) - len(t.lastPublic.Seats); n > 0 {
		return n
	}
	return 0
}

func (t *Table) session(id string) *Session {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sessions[id]
}

func (t *Table) snapshot() []*Session {
	t.mu.Lock()
	defer t.mu.Unlock()

	out := make([]*Session, 0, len(t.sessions))
	for _, s := range t.sessions {
		out = append(out, s)
	}
	return out
}

func (t *Table) signal() {
	select {
	case t.wake <- struct{}{}:
	default:
	}
}

// Run drives the table until the context is cancelled. It is the only
// goroutine permitted to touch t.game.
func (t *Table) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		t.applySeatOps()

		// Clear out busted players before counting. PlayHand does this
		// too, but at its start, so leaving it to PlayHand means a table
		// that has just lost a player tries to deal and reports "not
		// enough players" to everyone still watching.
		t.game.RemoveBustedPlayers()
		t.reportBustouts()

		if len(t.game.Players) < 2 {
			t.broadcastInfo("Waiting for players...")
			select {
			case <-ctx.Done():
				return
			case <-t.wake:
			}
			continue
		}

		if _, err := t.game.PlayHand(); err != nil {
			t.broadcastInfo(fmt.Sprintf("Hand could not be played: %v", err))
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(t.cfg.HandDelay):
		}
	}
}

// applySeatOps folds queued sits and stands into the game between hands,
// which is also when poker says a player may join or leave.
func (t *Table) applySeatOps() {
	changed := false
	defer func() {
		if changed {
			// The seat and watcher counts on everyone's menu just moved.
			t.publishSeats()
			t.broadcastLobby()
		}
	}()

	for {
		select {
		case op := <-t.seatOps:
			switch op.kind {
			case opSit:
				if t.game.SeatOf(op.session.Player) != game.SpectatorSeat {
					continue
				}
				t.fundSeat(op.session)
				src := &remoteSource{table: t, id: op.session.ID}
				if err := t.game.AddPlayerWithSource(op.session.Player, src); err != nil {
					op.session.send(InfoMsg{Text: err.Error()})
					continue
				}
				t.broadcastInfo(fmt.Sprintf("%s sits down with %d.",
					op.session.Name, op.session.Player.Chips))
				changed = true

			case opRun:
				op.fn()

			case opStand:
				// Bank the stack before the seat goes, so a player who
				// sits back down gets their own chips rather than a
				// fresh buy-in.
				t.banked[op.session.ID] = op.session.Player.Chips
				if t.game.RemovePlayer(op.session.Player) {
					t.broadcastInfo(fmt.Sprintf("%s leaves the table.", op.session.Name))
					changed = true
				}
			}
		default:
			return
		}
	}
}

// publishSeats refreshes the cached public snapshot after the seating has
// changed but before any hand has been dealt, so the lobby's counts are
// right the moment someone sits down.
func (t *Table) publishSeats() {
	public := t.game.ViewFor(game.SpectatorSeat)

	t.mu.Lock()
	t.lastPublic = public
	t.mu.Unlock()

	for _, s := range t.snapshot() {
		seat := t.game.SeatOf(s.Player)
		view := t.game.ViewFor(seat)

		t.mu.Lock()
		t.lastView[s.ID] = view
		t.mu.Unlock()

		s.send(StateMsg{View: view})
	}
}

// fundSeat gives a player the stack they are entitled to: the one they
// left with if they are returning, and a fresh buy-in if they are new or
// busted. It runs on the Run goroutine, the only one that may touch a
// chip count.
func (t *Table) fundSeat(s *Session) {
	if s.Player.Chips > 0 {
		return
	}

	if banked, ok := t.banked[s.ID]; ok {
		delete(t.banked, s.ID)
		if banked > 0 {
			s.Player.Chips = banked
			return
		}
	}

	s.Player.Chips = t.cfg.BuyIn
}

// reportBustouts tells anyone who has run out of chips that they can buy
// in again.
//
// Seat position is deliberately not part of the test. A player who busts
// in this hand is still in g.Players when this runs -- RemoveBustedPlayers
// happens at the start of the next PlayHand, not the end of this one --
// and if they were the second-to-last player there may not be a next hand
// at all. Keying off the empty stack alone tells them straight away.
func (t *Table) reportBustouts() {
	for _, s := range t.snapshot() {
		if s.Player.Chips == 0 {
			s.send(InfoMsg{Text: "You are out of chips. Press r to buy in again."})
		}
	}
}

// TableChanged implements game.Watcher. It runs on the Run goroutine, the
// only place ViewFor may be called, and pushes each session its own
// redacted snapshot.
func (t *Table) TableChanged(g *game.Game) {
	public := g.ViewFor(game.SpectatorSeat)

	t.mu.Lock()
	t.lastPublic = public
	t.mu.Unlock()

	for _, s := range t.snapshot() {
		seat := g.SeatOf(s.Player)
		view := g.ViewFor(seat)

		t.mu.Lock()
		t.lastView[s.ID] = view
		t.mu.Unlock()

		s.send(StateMsg{View: view})

		if seat != game.SpectatorSeat && seat == g.ActingSeat {
			s.send(TurnMsg{View: view})
		}
	}
}

// HandFinished implements game.Watcher.
func (t *Table) HandFinished(g *game.Game, r game.HandResult) {
	seats := g.ViewFor(game.SpectatorSeat).Seats

	for _, s := range t.snapshot() {
		s.send(ResultMsg{Result: r, Seats: seats})
	}

	t.TableChanged(g)
}

func (t *Table) broadcastInfo(text string) {
	for _, s := range t.snapshot() {
		s.send(InfoMsg{Text: text})
	}
}

func (t *Table) armTurn(sessionID string, replies chan game.Decision) {
	t.turnMu.Lock()
	t.turn = &turnState{sessionID: sessionID, replies: replies}
	t.turnMu.Unlock()
}

func (t *Table) disarmTurn() {
	t.turnMu.Lock()
	t.turn = nil
	t.turnMu.Unlock()
}

// remoteSource is the bridge between the engine's synchronous
// ActionSource and a player sitting at a terminal somewhere.
type remoteSource struct {
	table *Table
	id    string
}

func (r *remoteSource) RequestAction(ctx context.Context, v game.PlayerView) (game.Decision, error) {
	sess := r.table.session(r.id)
	if sess == nil {
		return game.Decision{}, errSessionGone
	}

	replies := make(chan game.Decision, 1)
	r.table.armTurn(r.id, replies)
	defer r.table.disarmTurn()

	select {
	case d := <-replies:
		return d, nil
	case <-sess.Done():
		// A disconnect gives up the turn immediately rather than holding
		// the table for the rest of the shot clock.
		return game.Decision{}, errSessionGone
	case <-ctx.Done():
		return game.Decision{}, ctx.Err()
	}
}
