package game

import (
	"context"
	"fmt"
	"time"

	"ssh_holdem/deck"
	"ssh_holdem/player"
)

// maxIllegalActions caps how many times one seat may return an illegal
// decision before the engine acts for them. A buggy bot must not be able
// to stall the table, and a human on a flaky connection shouldn't either.
const maxIllegalActions = 3

// SpectatorSeat is the seat index for anyone not in the hand: a railbird,
// or a player waiting to be dealt in. ViewFor gives it the public state
// with no hole cards.
const SpectatorSeat = -1

type Game struct {
	// Players and Sources are parallel slices: Sources[i] answers for
	// Players[i]. AddPlayer maintains both, so nothing else should append
	// to either one directly.
	Players []*player.Player
	Sources []ActionSource

	Deck  deck.Deck
	Board deck.Board

	// Pot is the running total of chips in the middle. The side-pot
	// layering is derived from each player's TotalBet at showdown, so
	// there is no second source of truth to keep in sync.
	Pot int

	SmallBlind int
	BigBlind   int

	// ButtonIndex is where the dealer button sits in Players, or
	// SpectatorSeat when the button is dead -- parked on a seat whose
	// occupant has left. It is derived from the seating ring; set it with
	// SetButton rather than assigning to it.
	ButtonIndex int
	Street      Street

	// ActingSeat is whose decision the table is waiting on, or
	// SpectatorSeat when no one is being asked. ActingDeadline is when
	// their time runs out, zero when there is no clock running.
	ActingSeat     int
	ActingDeadline time.Time

	// CurrentBet is the amount each player must have in for the street.
	// MinRaise is the size of the last full raise, so the smallest legal
	// raise is to CurrentBet + MinRaise.
	CurrentBet int
	MinRaise   int

	// TurnTimeout bounds how long a source has to answer. Zero means no
	// limit, which is fine for bots and tests but not for a table with a
	// human who can close their laptop mid-hand.
	TurnTimeout time.Duration

	// order is the seating ring. A nil entry is a seat someone has left,
	// held open until the button has passed over it.
	//
	// Compacting Players on every departure is what makes a naive button
	// unfair: the seats renumber underneath it, so a player can end up on
	// the button twice running, or post the big blind twice, or skip it.
	// Keeping the vacated seat in the ring for one orbit is the dead
	// button rule, and it costs one slice of bookkeeping.
	order []*player.Player

	// buttonPos, sbPos and bbPos index order and are three consecutive
	// seats. Any of them may land on a vacated seat: a dead button, or a
	// dead small blind that simply is not posted.
	buttonPos int
	sbPos     int
	bbPos     int

	deadSmallBlind bool
	positioned     bool

	// Watch, if set, is notified whenever the table state changes so a UI
	// can redraw. It is called synchronously from whichever goroutine is
	// driving the hand, so implementations must not block.
	Watch Watcher
}

// Watcher observes a hand as it plays out. The engine hands it the whole
// Game; redaction happens in ViewFor, which is what any implementation
// should use to build what it sends to a given seat.
type Watcher interface {
	TableChanged(g *Game)
	HandFinished(g *Game, r HandResult)
}

// turnContext bounds one source's answer. A human who walks away must not
// hold the table, and a bot that hangs must not either.
func (g *Game) turnContext(parent context.Context) (context.Context, context.CancelFunc) {
	if g.TurnTimeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, g.TurnTimeout)
}

func (g *Game) notifyChanged() {
	if g.Watch != nil {
		g.Watch.TableChanged(g)
	}
}

func NewGame(sb, bb int) Game {
	return Game{
		Players:     make([]*player.Player, 0, 9),
		Sources:     make([]ActionSource, 0, 9),
		SmallBlind:  sb,
		BigBlind:    bb,
		ButtonIndex: 0,
		ActingSeat:  SpectatorSeat,
		order:       make([]*player.Player, 0, 9),
	}
}

// AddPlayer seats a player who always checks or calls. Use
// AddPlayerWithSource to attach a real decision-maker.
func (g *Game) AddPlayer(p *player.Player) error {
	return g.AddPlayerWithSource(p, CallingStation{})
}

func (g *Game) AddPlayerWithSource(p *player.Player, s ActionSource) error {
	if len(g.Players) >= 9 {
		return fmt.Errorf("Table is full (max 9 players)")
	}

	g.Players = append(g.Players, p)
	g.Sources = append(g.Sources, s)
	g.order = append(g.order, p)
	return nil
}

// RemoveBustedPlayers drops anyone with no chips left between hands.
// Their seat stays in the ring as a vacated slot so the blinds keep
// advancing by one seat per hand.
func (g *Game) RemoveBustedPlayers() {
	for i := len(g.Players) - 1; i >= 0; i-- {
		if g.Players[i].Chips <= 0 {
			g.removeSeat(i)
		}
	}
	g.syncButtonIndex()
}

// RemovePlayer drops one seat from the table. Callers hold a
// *player.Player rather than a seat index because indices shift when
// anyone leaves, and a stale index silently addresses the wrong seat.
func (g *Game) RemovePlayer(target *player.Player) bool {
	for i, p := range g.Players {
		if p != target {
			continue
		}
		g.removeSeat(i)
		g.syncButtonIndex()
		return true
	}
	return false
}

// removeSeat takes a player out of Players and vacates their slot in the
// ring, leaving the ring's length and every position index untouched.
func (g *Game) removeSeat(i int) {
	target := g.Players[i]

	g.Players = append(g.Players[:i], g.Players[i+1:]...)
	g.Sources = append(g.Sources[:i], g.Sources[i+1:]...)

	for pos, p := range g.order {
		if p == target {
			g.order[pos] = nil
			break
		}
	}
}

// dropSeat removes a vacated slot from the ring entirely, once it has
// served its orbit, sliding the position indices down to match.
func (g *Game) dropSeat(pos int) {
	g.order = append(g.order[:pos], g.order[pos+1:]...)

	for _, idx := range []*int{&g.buttonPos, &g.sbPos, &g.bbPos} {
		if *idx > pos {
			*idx--
		}
	}
}

// occupied counts the seats in the ring that still have a player in them.
func (g *Game) occupied() int {
	n := 0
	for _, p := range g.order {
		if p != nil {
			n++
		}
	}
	return n
}

// nextOccupied walks forward from pos to the next seat with a player in
// it, or returns pos when the ring is empty.
func (g *Game) nextOccupied(pos int) int {
	n := len(g.order)
	for i := 1; i <= n; i++ {
		candidate := ((pos+i)%n + n) % n
		if g.order[candidate] != nil {
			return candidate
		}
	}
	return pos
}

// SetButton parks the button on a seat in Players and derives the blinds
// from it. Use it to arrange a table; the button moves on its own after
// that.
func (g *Game) SetButton(seat int) {
	if seat < 0 || seat >= len(g.Players) {
		return
	}

	for pos, p := range g.order {
		if p == g.Players[seat] {
			g.buttonPos = pos
			if g.occupied() <= 2 {
				// Heads-up the button posts the small blind, so the big
				// blind is the very next seat, not the one after that.
				g.bbPos = g.nextOccupied(pos)
			} else {
				g.bbPos = g.nextOccupied(g.nextOccupied(pos))
			}
			g.derivePositions()
			g.positioned = true
			return
		}
	}
}

// derivePositions fills the small blind and button from the big blind,
// which is the position the dead button rule actually pins down: the big
// blind advances exactly one occupied seat per hand, and everything else
// is measured back from it.
func (g *Game) derivePositions() {
	n := len(g.order)
	if n == 0 {
		return
	}

	g.bbPos = ((g.bbPos % n) + n) % n

	// The big blind is the one position that can never be dead: somebody
	// has to have a live hand for there to be a hand at all. It can land
	// on a vacated seat between hands, when its occupant busts after the
	// button has already moved, and passes to the next live seat.
	if g.order[g.bbPos] == nil {
		g.bbPos = g.nextOccupied(g.bbPos)
	}

	if g.occupied() <= 2 {
		// Heads-up the button is the small blind, and there is no third
		// seat for a dead button to sit on.
		other := g.nextOccupied(g.bbPos)
		g.sbPos, g.buttonPos = other, other
		g.deadSmallBlind = false
		g.syncButtonIndex()
		return
	}

	// Counted back through the ring, not through Players: walking over a
	// vacated seat rather than skipping it is what keeps every remaining
	// player advancing by exactly one position.
	g.sbPos = ((g.bbPos-1)%n + n) % n
	g.buttonPos = ((g.bbPos-2)%n + n) % n
	g.deadSmallBlind = g.order[g.sbPos] == nil

	g.syncButtonIndex()
}

func (g *Game) syncButtonIndex() {
	if g.buttonPos < 0 || g.buttonPos >= len(g.order) || g.order[g.buttonPos] == nil {
		g.ButtonIndex = SpectatorSeat
		return
	}
	g.ButtonIndex = g.SeatOf(g.order[g.buttonPos])
}

// ensurePositioned puts the blinds somewhere sensible the first time a
// hand is dealt.
func (g *Game) ensurePositioned() {
	if g.positioned || len(g.Players) == 0 {
		return
	}
	g.SetButton(0)
}

// clockwiseFromButton lists seats in Players starting with the one left of
// the button. It reads the ring rather than Players so a dead button still
// has a well-defined place to count from.
func (g *Game) clockwiseFromButton() []int {
	n := len(g.order)
	seats := make([]int, 0, len(g.Players))

	for i := 1; i <= n; i++ {
		p := g.order[((g.buttonPos+i)%n+n)%n]
		if p == nil {
			continue
		}
		if seat := g.SeatOf(p); seat != SpectatorSeat {
			seats = append(seats, seat)
		}
	}

	return seats
}

// SeatOf reports where a player is sitting, or SpectatorSeat if they are
// not at the table.
func (g *Game) SeatOf(target *player.Player) int {
	for i, p := range g.Players {
		if p == target {
			return i
		}
	}
	return SpectatorSeat
}

// getBlindIndices returns the seats posting the blinds. The small blind
// is SpectatorSeat when it is dead: the seat that owes it has been
// vacated, so nobody posts it this hand.
func (g *Game) getBlindIndices() (int, int) {
	g.ensurePositioned()

	if len(g.order) == 0 {
		return SpectatorSeat, SpectatorSeat
	}

	sb := SpectatorSeat
	if !g.deadSmallBlind && g.order[g.sbPos] != nil {
		sb = g.SeatOf(g.order[g.sbPos])
	}

	bb := SpectatorSeat
	if g.order[g.bbPos] != nil {
		bb = g.SeatOf(g.order[g.bbPos])
	}

	return sb, bb
}

func (g *Game) StartNewHand() error {
	if len(g.Players) < 2 {
		return fmt.Errorf("not enough players for new game")
	}

	g.ensurePositioned()
	// Occupancy may have changed since the button last moved -- a player
	// busting takes effect between hands -- so the blinds are re-derived
	// against who is actually here. This never advances the big blind, it
	// only settles where the other two positions fall.
	g.derivePositions()

	g.Deck = deck.NewDeck()
	g.Board = deck.Board{}
	g.Pot = 0
	g.Street = Preflop

	for _, p := range g.Players {
		if err := p.ResetForNewHand(&g.Deck); err != nil {
			return fmt.Errorf("failed to deal hand to %s: %w", p.Name, err)
		}
		// A seat with no chips sits the hand out rather than blocking it.
		if p.Chips == 0 {
			p.Folded = true
		}
	}

	sbIndex, bbIndex := g.getBlindIndices()

	// Blinds go in through the normal betting path so a short stack posts
	// what it has and is marked all-in, which is the earliest side pot
	// any table will see.
	//
	// A dead small blind is simply not posted: the seat that owed it has
	// been vacated, and nobody covers for them.
	if sbIndex != SpectatorSeat {
		g.Pot += g.postBlind(g.Players[sbIndex], g.SmallBlind)
	}
	if bbIndex != SpectatorSeat {
		g.Pot += g.postBlind(g.Players[bbIndex], g.BigBlind)
	}

	// Players still owe the full big blind even when the blind itself was
	// posted short.
	g.CurrentBet = g.BigBlind
	g.MinRaise = g.BigBlind

	return nil
}

func (g *Game) postBlind(p *player.Player, amount int) int {
	if p.Chips == 0 {
		return 0
	}
	posted, err := p.Bet(amount)
	if err != nil {
		return 0
	}
	return posted
}

// MoveButton advances the blinds one seat for the next hand.
//
// The big blind is what moves; the small blind and button are derived
// back from it. A vacated seat that has just served as the dead button
// has finished its orbit and leaves the ring here.
func (g *Game) MoveButton() {
	if len(g.order) == 0 {
		return
	}
	g.ensurePositioned()

	if g.order[g.buttonPos] == nil {
		g.dropSeat(g.buttonPos)
		if len(g.order) == 0 {
			return
		}
	}

	g.bbPos = g.nextOccupied(g.bbPos)
	g.derivePositions()
}

// firstToAct returns the seat that opens the given street. Callers should
// never compute this themselves; the heads-up rules invert both cases and
// getting them wrong is silent.
func (g *Game) firstToAct(s Street) int {
	order := g.clockwiseFromButton()
	if len(order) == 0 {
		return 0
	}

	_, bbIndex := g.getBlindIndices()

	if s == Preflop {
		if len(g.Players) == 2 && g.ButtonIndex != SpectatorSeat {
			// Heads-up the button is the small blind and acts first
			// before the flop.
			return g.ButtonIndex
		}
		// Under the gun: the seat after the big blind in ring order.
		for i, seat := range order {
			if seat == bbIndex {
				return order[(i+1)%len(order)]
			}
		}
		return order[0]
	}

	if len(g.Players) == 2 && bbIndex != SpectatorSeat {
		// After the flop the button acts last, so the big blind opens.
		return bbIndex
	}

	// First live seat left of the button, dead or not.
	return order[0]
}

func (g *Game) contestingCount() int {
	n := 0
	for _, p := range g.Players {
		if p.IsContesting() {
			n++
		}
	}
	return n
}

func (g *Game) canActCount() int {
	n := 0
	for _, p := range g.Players {
		if p.CanAct() {
			n++
		}
	}
	return n
}

// ViewFor builds the redacted snapshot for one seat. Only that seat's hole
// cards go in, which is what makes it safe to send over the wire.
//
// A seat outside the table -- use SpectatorSeat -- gets the same public
// state with no hole cards and no betting figures, which is what a
// railbird or a player waiting for the next hand sees.
func (g *Game) ViewFor(seat int) PlayerView {
	var p *player.Player
	if seat >= 0 && seat < len(g.Players) {
		p = g.Players[seat]
	}

	seats := make([]SeatInfo, 0, len(g.Players))
	for i, q := range g.Players {
		seats = append(seats, SeatInfo{
			Index:      i,
			Name:       q.Name,
			Chips:      q.Chips,
			CurrentBet: q.CurrentBet,
			TotalBet:   q.TotalBet,
			Folded:     q.Folded,
			AllIn:      q.AllIn,
			IsButton:   i == g.ButtonIndex,
		})
	}

	view := PlayerView{
		Seat:       SpectatorSeat,
		Deadline:   g.ActingDeadline,
		TurnLength: g.TurnTimeout,
		Board:      append([]deck.Card(nil), g.Board.Cards...),
		Seats:      seats,
		Street:     g.Street,
		Acting:     g.ActingSeat,
		Pot:        g.Pot,
		CurrentBet: g.CurrentBet,
	}

	if p == nil {
		return view
	}

	toCall := g.CurrentBet - p.CurrentBet
	if toCall > p.Chips {
		toCall = p.Chips
	}

	view.Seat = seat
	view.Hole = append([]deck.Card(nil), p.Hand.Cards...)
	view.ToCall = toCall
	view.MinRaiseTo = g.CurrentBet + g.MinRaise
	view.MaxRaiseTo = p.CurrentBet + p.Chips

	return view
}

// ExecuteBettingRound runs one street of betting, starting at startIndex
// and continuing until every player who can act has either matched the
// current bet or folded.
//
// Termination is tracked with a needsToAct set rather than an action
// counter: only that models the big blind's option to raise a limped pot,
// and it makes a raise's effect obvious -- everyone else who can act owes
// another decision.
func (g *Game) ExecuteBettingRound(startIndex int) error {
	ctx := context.Background()
	n := len(g.Players)
	if n == 0 {
		return nil
	}

	needsToAct := make([]bool, n)
	multiway := g.canActCount() >= 2

	for i, p := range g.Players {
		if !p.CanAct() {
			continue
		}
		// A lone player with nothing to call has no decision to make.
		needsToAct[i] = multiway || g.CurrentBet-p.CurrentBet > 0
	}

	currentIndex := startIndex % n

	for {
		if g.contestingCount() <= 1 {
			return nil
		}

		pending := false
		for i, p := range g.Players {
			if needsToAct[i] && p.CanAct() {
				pending = true
				break
			}
		}
		if !pending {
			return nil
		}

		p := g.Players[currentIndex]
		if !needsToAct[currentIndex] || !p.CanAct() {
			currentIndex = (currentIndex + 1) % n
			continue
		}

		raised, err := g.resolveTurn(ctx, currentIndex)
		if err != nil {
			return err
		}

		needsToAct[currentIndex] = false
		g.notifyChanged()

		if raised {
			// A full raise reopens the betting for everyone else.
			for i, q := range g.Players {
				if i != currentIndex && q.CanAct() {
					needsToAct[i] = true
				}
			}
		}

		currentIndex = (currentIndex + 1) % n
	}
}

// resolveTurn asks one seat for a decision, retrying on illegal input, and
// applies the result. It reports whether the action was a full raise, which
// is what reopens the betting.
func (g *Game) resolveTurn(ctx context.Context, seat int) (bool, error) {
	p := g.Players[seat]
	view := g.ViewFor(seat)

	// The clock is started before the source is asked, so every other
	// seat can see whose decision the table is waiting on and how long
	// they have left.
	if g.TurnTimeout > 0 {
		g.ActingDeadline = time.Now().Add(g.TurnTimeout)
		view.Deadline = g.ActingDeadline
		view.TurnLength = g.TurnTimeout
	}

	g.ActingSeat = seat
	g.notifyChanged()
	defer func() {
		g.ActingSeat = SpectatorSeat
		g.ActingDeadline = time.Time{}
	}()

	// One deadline covers the whole turn, retries included. That is what a
	// poker clock means, and it stops a source that spams illegal actions
	// from buying itself several times the allotted think time.
	turnCtx, cancel := g.turnContext(ctx)
	defer cancel()

	var decision Decision

	for attempt := 0; ; attempt++ {
		d, err := g.Sources[seat].RequestAction(turnCtx, view)
		if err != nil {
			// A source that cannot answer -- a disconnected session, a
			// blown deadline -- gives up its turn the cheapest legal way.
			decision = defaultDecision(view)
			break
		}
		if legalErr := validate(view, d); legalErr == nil { //nolint
			decision = d
			break
		}
		if attempt >= maxIllegalActions {
			decision = defaultDecision(view)
			break
		}
	}

	switch decision.Action {
	case Fold:
		p.Fold()
		return false, nil

	case Check:
		return false, nil

	case Call:
		if view.ToCall > 0 {
			paid, err := p.Bet(view.ToCall)
			if err != nil {
				return false, fmt.Errorf("%s could not call: %w", p.Name, err)
			}
			g.Pot += paid
		}
		return false, nil

	case Raise:
		target := decision.Amount
		if target > view.MaxRaiseTo {
			target = view.MaxRaiseTo
		}
		increment := target - p.CurrentBet
		if increment <= 0 {
			return false, nil
		}

		paid, err := p.Bet(increment)
		if err != nil {
			return false, fmt.Errorf("%s could not raise: %w", p.Name, err)
		}
		g.Pot += paid

		// An all-in that falls short of a full raise still puts more
		// money in, but it does not reopen the betting and does not
		// change the minimum raise.
		fullRaise := target >= view.MinRaiseTo

		if target > g.CurrentBet {
			if fullRaise {
				g.MinRaise = target - g.CurrentBet
			}
			g.CurrentBet = target
		}

		return fullRaise, nil
	}

	return false, nil
}

// validate reports why a decision is not available in this spot, or nil if
// it is fine.
func validate(v PlayerView, d Decision) error {
	switch d.Action {
	case Fold, Check, Call:
		if !v.Legal(d.Action) {
			return fmt.Errorf("%s is not available here", d.Action)
		}
		return nil

	case Raise:
		if !v.Legal(Raise) {
			return fmt.Errorf("no chips left to raise with")
		}
		if d.Amount > v.MaxRaiseTo {
			return fmt.Errorf("raise to %d exceeds the %d available", d.Amount, v.MaxRaiseTo)
		}
		// Shoving is always allowed even when it falls short of a full raise.
		if d.Amount < v.MinRaiseTo && d.Amount != v.MaxRaiseTo {
			return fmt.Errorf("raise to at least %d", v.MinRaiseTo)
		}
		if d.Amount <= v.CurrentBet {
			return fmt.Errorf("raise to %d does not beat the current bet of %d", d.Amount, v.CurrentBet)
		}
		return nil
	}

	return fmt.Errorf("unknown action %q", d.Action)
}

// defaultDecision is what the engine does on a player's behalf when they
// cannot or will not act: never put chips at risk, never fold for free.
func defaultDecision(v PlayerView) Decision {
	if v.ToCall == 0 {
		return Decision{Action: Check}
	}
	return Decision{Action: Fold}
}

func (g *Game) dealStreet(s Street) error {
	switch s {
	case Flop:
		return deck.DealFlop(&g.Deck, &g.Board)
	case Turn, River:
		return deck.DealTurnOrRiver(&g.Deck, &g.Board)
	}
	return nil
}

// PlayHand runs one complete hand: blinds, four streets of betting, then
// showdown and payout. The button moves at the end.
func (g *Game) PlayHand() (HandResult, error) {
	g.RemoveBustedPlayers()

	if err := g.StartNewHand(); err != nil {
		return HandResult{}, err
	}
	g.notifyChanged()

	for _, street := range []Street{Preflop, Flop, Turn, River} {
		g.Street = street

		if street != Preflop {
			// The board is dealt even when everyone left is all-in: the
			// hand still has to be decided, and the evaluator needs all
			// five community cards.
			if err := g.dealStreet(street); err != nil {
				return HandResult{}, err
			}
			for _, p := range g.Players {
				p.ResetForBettingRound()
			}
			g.CurrentBet = 0
			g.MinRaise = g.BigBlind
			g.notifyChanged()
		}

		if g.contestingCount() <= 1 {
			break
		}

		if g.canActCount() > 0 {
			if err := g.ExecuteBettingRound(g.firstToAct(street)); err != nil {
				return HandResult{}, err
			}
		}
	}

	result := g.Payout()
	g.MoveButton()

	if g.Watch != nil {
		g.Watch.HandFinished(g, result)
	}

	return result, nil
}
