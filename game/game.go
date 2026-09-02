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

	ButtonIndex int
	Street      Street

	// ActingSeat is whose decision the table is waiting on, or
	// SpectatorSeat when no one is being asked.
	ActingSeat int

	// CurrentBet is the amount each player must have in for the street.
	// MinRaise is the size of the last full raise, so the smallest legal
	// raise is to CurrentBet + MinRaise.
	CurrentBet int
	MinRaise   int

	// TurnTimeout bounds how long a source has to answer. Zero means no
	// limit, which is fine for bots and tests but not for a table with a
	// human who can close their laptop mid-hand.
	TurnTimeout time.Duration

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
	return nil
}

// RemoveBustedPlayers drops anyone with no chips left between hands.
//
// The button follows the nearest surviving seat at or before its old
// position, so when the button player themselves busts the button steps
// back one seat and PlayHand's MoveButton then advances it. The dealer
// button can therefore rest on the same seat for two hands after a bust-
// out. That is a deliberate simplification; a full dead-button rule is
// phase 2 work.
func (g *Game) RemoveBustedPlayers() {
	survivors := make([]*player.Player, 0, len(g.Players))
	sources := make([]ActionSource, 0, len(g.Sources))
	newButton := 0

	for i, p := range g.Players {
		if p.Chips <= 0 {
			continue
		}
		if i <= g.ButtonIndex {
			newButton = len(survivors)
		}
		survivors = append(survivors, p)
		sources = append(sources, g.Sources[i])
	}

	g.Players = survivors
	g.Sources = sources

	if len(g.Players) == 0 {
		g.ButtonIndex = 0
		return
	}
	g.ButtonIndex = newButton % len(g.Players)
}

// RemovePlayer drops one seat from the table. Callers hold a
// *player.Player rather than a seat index because indices shift when
// anyone leaves, and a stale index silently addresses the wrong seat.
func (g *Game) RemovePlayer(target *player.Player) bool {
	for i, p := range g.Players {
		if p != target {
			continue
		}
		g.Players = append(g.Players[:i], g.Players[i+1:]...)
		g.Sources = append(g.Sources[:i], g.Sources[i+1:]...)

		if len(g.Players) == 0 {
			g.ButtonIndex = 0
		} else if g.ButtonIndex >= len(g.Players) {
			g.ButtonIndex = 0
		}
		return true
	}
	return false
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

func (g *Game) getBlindIndices() (int, int) {
	numPlayers := len(g.Players)

	if numPlayers == 2 {
		return g.ButtonIndex, (g.ButtonIndex + 1) % numPlayers
	}

	sbIndex := (g.ButtonIndex + 1) % numPlayers
	bbIndex := (g.ButtonIndex + 2) % numPlayers

	return sbIndex, bbIndex
}

func (g *Game) StartNewHand() error {
	if len(g.Players) < 2 {
		return fmt.Errorf("not enough players for new game")
	}

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
	g.Pot += g.postBlind(g.Players[sbIndex], g.SmallBlind)
	g.Pot += g.postBlind(g.Players[bbIndex], g.BigBlind)

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

func (g *Game) MoveButton() {
	g.ButtonIndex = (g.ButtonIndex + 1) % len(g.Players)
}

// firstToAct returns the seat that opens the given street. Callers should
// never compute this themselves; the heads-up rules invert both cases and
// getting them wrong is silent.
func (g *Game) firstToAct(s Street) int {
	n := len(g.Players)
	_, bbIndex := g.getBlindIndices()

	if s == Preflop {
		if n == 2 {
			// Heads-up the button is the small blind and acts first
			// before the flop.
			return g.ButtonIndex
		}
		return (bbIndex + 1) % n
	}

	if n == 2 {
		// After the flop the button acts last, so the big blind opens.
		return bbIndex
	}
	return (g.ButtonIndex + 1) % n
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

	if g.TurnTimeout > 0 {
		view.Deadline = time.Now().Add(g.TurnTimeout)
	}

	// The turn is announced before the source is asked, so every other
	// seat can see whose decision the table is waiting on.
	g.ActingSeat = seat
	g.notifyChanged()
	defer func() {
		g.ActingSeat = SpectatorSeat
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
