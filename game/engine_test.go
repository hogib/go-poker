package game

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"ssh_holdem/deck"
	"ssh_holdem/player"
)

func seat(g *Game, name string, chips int) *player.Player {
	p := player.NewPlayer(name, chips)
	g.AddPlayer(&p)
	return g.Players[len(g.Players)-1]
}

func totalChips(g *Game) int {
	sum := 0
	for _, p := range g.Players {
		sum += p.Chips
	}
	return sum
}

func TestBuildPotsLayersBySideStack(t *testing.T) {
	gv := NewGame(5, 10)
	g := &gv
	a := seat(g, "AllInShort", 0)
	b := seat(g, "Big1", 0)
	c := seat(g, "Big2", 0)
	d := seat(g, "Folder", 0)

	// A is all-in for 50, B and C got to 200, D folded having put in 20.
	a.TotalBet, a.AllIn = 50, true
	b.TotalBet = 200
	c.TotalBet = 200
	d.TotalBet, d.Folded = 20, true

	pots := g.buildPots()

	if len(pots) != 2 {
		t.Fatalf("expected 2 pot layers, got %d: %+v", len(pots), pots)
	}
	// Levels 20 and 50 share the same contestants, so they merge:
	// 20*4 + 30*3 = 170 contested by A, B and C.
	if pots[0].Amount != 170 || len(pots[0].Eligible) != 3 {
		t.Errorf("main pot: expected 170 for 3 seats, got %d for %v",
			pots[0].Amount, pots[0].Eligible)
	}
	// 150*2 = 300 above A's stack, contested by B and C only.
	if pots[1].Amount != 300 || len(pots[1].Eligible) != 2 {
		t.Errorf("side pot: expected 300 for 2 seats, got %d for %v",
			pots[1].Amount, pots[1].Eligible)
	}
	if pots[1].Eligible[0] != 1 || pots[1].Eligible[1] != 2 {
		t.Errorf("side pot should exclude the short stack, got %v", pots[1].Eligible)
	}

	total := 0
	for _, pot := range pots {
		total += pot.Amount
	}
	if want := 50 + 200 + 200 + 20; total != want {
		t.Errorf("layers sum to %d, but %d was contributed", total, want)
	}
}

// An all-in on an early street is the only case that distinguishes a
// correct TotalBet reset from one that also clears TotalBet between
// streets. Every single-street test passes either way.
func TestAllInOnFlopSurvivesLaterStreets(t *testing.T) {
	gv := NewGame(5, 10)
	g := &gv
	short := seat(g, "Short", 100)
	mid := seat(g, "Mid", 300)
	deep := seat(g, "Deep", 300)

	// Preflop: everyone in for 40.
	for _, p := range g.Players {
		p.Bet(40)
		p.ResetForBettingRound()
	}

	// Flop: the short stack shoves its last 60 and the others cover.
	for _, p := range g.Players {
		p.Bet(60)
		p.ResetForBettingRound()
	}
	if !short.AllIn {
		t.Fatalf("the short stack should be all-in after shoving its last 60")
	}

	// Turn: the two deep stacks keep betting past the short stack.
	mid.Bet(100)
	deep.Bet(100)

	if short.TotalBet != 100 {
		t.Fatalf("TotalBet was cleared between streets: expected 100, got %d", short.TotalBet)
	}

	pots := g.buildPots()
	if len(pots) != 2 {
		t.Fatalf("expected a main pot and a side pot, got %d layers: %+v", len(pots), pots)
	}
	if pots[0].Amount != 300 {
		t.Errorf("main pot should be 100*3 = 300, got %d", pots[0].Amount)
	}
	if pots[1].Amount != 200 {
		t.Errorf("side pot should be 100*2 = 200, got %d", pots[1].Amount)
	}
	if len(pots[1].Eligible) != 2 {
		t.Errorf("short stack must not be eligible for the side pot, got %v", pots[1].Eligible)
	}
}

func TestPayoutSplitsTiedPot(t *testing.T) {
	gv := NewGame(5, 10)
	g := &gv
	a := seat(g, "Alice", 900)
	b := seat(g, "Bob", 900)

	a.TotalBet, b.TotalBet = 100, 100
	g.Pot = 200

	// Both play the board: the same straight, so the pot splits.
	a.Hand = deck.Hand{Cards: []deck.Card{
		deck.NewCard(deck.Two, deck.Spades), deck.NewCard(deck.Three, deck.Hearts),
	}}
	b.Hand = deck.Hand{Cards: []deck.Card{
		deck.NewCard(deck.Two, deck.Clubs), deck.NewCard(deck.Three, deck.Diamonds),
	}}
	g.Board = deck.Board{Cards: []deck.Card{
		deck.NewCard(deck.Ten, deck.Spades), deck.NewCard(deck.Jack, deck.Hearts),
		deck.NewCard(deck.Queen, deck.Clubs), deck.NewCard(deck.King, deck.Diamonds),
		deck.NewCard(deck.Ace, deck.Spades),
	}}

	result := g.Payout()

	if len(result.Pots) != 1 || len(result.Pots[0].Winners) != 2 {
		t.Fatalf("expected one pot split two ways, got %+v", result.Pots)
	}
	if a.Chips != 1000 || b.Chips != 1000 {
		t.Errorf("expected 1000 each after the split, got %d and %d", a.Chips, b.Chips)
	}
}

func TestPayoutOddChipGoesLeftOfButton(t *testing.T) {
	gv := NewGame(5, 10)
	g := &gv
	a := seat(g, "Alice", 0)
	b := seat(g, "Bob", 0)
	g.ButtonIndex = 0 // so Bob, in seat 1, is first left of the button

	a.TotalBet, b.TotalBet = 50, 51
	g.Pot = 101

	board := deck.Board{Cards: []deck.Card{
		deck.NewCard(deck.Ten, deck.Spades), deck.NewCard(deck.Jack, deck.Hearts),
		deck.NewCard(deck.Queen, deck.Clubs), deck.NewCard(deck.King, deck.Diamonds),
		deck.NewCard(deck.Ace, deck.Spades),
	}}
	g.Board = board
	a.Hand = deck.Hand{Cards: []deck.Card{
		deck.NewCard(deck.Two, deck.Spades), deck.NewCard(deck.Three, deck.Hearts),
	}}
	b.Hand = deck.Hand{Cards: []deck.Card{
		deck.NewCard(deck.Two, deck.Clubs), deck.NewCard(deck.Three, deck.Diamonds),
	}}

	g.Payout()

	if a.Chips+b.Chips != 101 {
		t.Fatalf("chips leaked: %d + %d != 101", a.Chips, b.Chips)
	}
	if b.Chips != 51 {
		t.Errorf("odd chip should go to seat 1, left of the button: got %d and %d", a.Chips, b.Chips)
	}
}

func TestFirstToActHeadsUpInverts(t *testing.T) {
	gv := NewGame(5, 10)
	g := &gv
	seat(g, "Alice", 100)
	seat(g, "Bob", 100)
	g.ButtonIndex = 0

	if got := g.firstToAct(Preflop); got != 0 {
		t.Errorf("heads-up the button acts first preflop, expected 0, got %d", got)
	}
	if got := g.firstToAct(Flop); got != 1 {
		t.Errorf("heads-up the big blind opens postflop, expected 1, got %d", got)
	}
}

func TestFirstToActThreeHanded(t *testing.T) {
	gv := NewGame(5, 10)
	g := &gv
	seat(g, "Alice", 100)
	seat(g, "Bob", 100)
	seat(g, "Charlie", 100)
	g.ButtonIndex = 0 // SB is 1, BB is 2

	if got := g.firstToAct(Preflop); got != 0 {
		t.Errorf("expected the button to act first preflop three-handed, got %d", got)
	}
	if got := g.firstToAct(Flop); got != 1 {
		t.Errorf("expected the small blind to open postflop, got %d", got)
	}
}

// The big blind gets a final option to raise when everyone limps. An
// action counter cannot express this; a needsToAct set can.
func TestBigBlindKeepsOptionAfterLimps(t *testing.T) {
	gv := NewGame(10, 20)
	g := &gv
	seat(g, "Button", 1000)
	seat(g, "SB", 1000)
	bb := seat(g, "BB", 1000)

	asked := 0
	g.Sources[2] = sourceFunc(func(v PlayerView) Decision {
		asked++
		if v.ToCall != 0 {
			t.Errorf("the big blind should face no bet after limps, got %d to call", v.ToCall)
		}
		return Decision{Action: Raise, Amount: 60}
	})

	if err := g.StartNewHand(); err != nil {
		t.Fatalf("StartNewHand: %v", err)
	}
	if err := g.ExecuteBettingRound(g.firstToAct(Preflop)); err != nil {
		t.Fatalf("ExecuteBettingRound: %v", err)
	}

	if asked == 0 {
		t.Fatal("the big blind was never given the option to act")
	}
	if g.CurrentBet != 60 {
		t.Errorf("expected the current bet to be the BB's raise to 60, got %d", g.CurrentBet)
	}
	if bb.CurrentBet != 60 {
		t.Errorf("expected the big blind to have 60 in, got %d", bb.CurrentBet)
	}
	// Both limpers must get another turn and call the raise.
	if g.Pot != 180 {
		t.Errorf("expected a pot of 180 after the raise is called, got %d", g.Pot)
	}
}

func TestShortStackPostsBlindAllIn(t *testing.T) {
	gv := NewGame(10, 20)
	g := &gv
	seat(g, "Deep", 1000)
	short := seat(g, "Short", 5)

	g.ButtonIndex = 0 // heads-up: seat 0 is the small blind

	if err := g.StartNewHand(); err != nil {
		t.Fatalf("StartNewHand: %v", err)
	}

	if short.Chips != 0 || !short.AllIn {
		t.Errorf("a big blind who cannot cover it should be all-in for 5, got chips %d allIn %v",
			short.Chips, short.AllIn)
	}
	if short.TotalBet != 5 {
		t.Errorf("expected the short blind to have posted 5, got %d", short.TotalBet)
	}
	if g.CurrentBet != 20 {
		t.Errorf("others still owe the full big blind, expected 20, got %d", g.CurrentBet)
	}
	if g.Pot != 15 {
		t.Errorf("expected a pot of 15 (10 + 5), got %d", g.Pot)
	}
}

// The gate for phase 1: play a lot of hands with bots that shove, fold and
// raise at random, and assert that chips are neither created nor destroyed
// and that the pot layering accounts for exactly what was wagered.
func TestRandomTablePreservesChips(t *testing.T) {
	r := rand.New(rand.NewSource(20260902))

	handsPlayed := 0

	for trial := 0; trial < 300; trial++ {
		size := 2 + r.Intn(5)

		gv := NewGame(5, 10)
		g := &gv
		for i := 0; i < size; i++ {
			// Uneven stacks are what produce side pots, so vary them.
			p := player.NewPlayer(string(rune('A'+i)), 200+r.Intn(1200))
			g.AddPlayerWithSource(&p, RandomBot{Rand: r})
		}

		bank := totalChips(g)

		// Play the table out until only one player has chips left.
		for hand := 0; ; hand++ {
			g.RemoveBustedPlayers()
			if len(g.Players) < 2 {
				break
			}

			result, err := g.PlayHand()
			if err != nil {
				t.Fatalf("trial %d, hand %d: %v", trial, hand, err)
			}
			handsPlayed++

			layered := 0
			for _, pot := range result.Pots {
				layered += pot.Amount
			}
			if layered != result.PotTotal {
				t.Fatalf("trial %d, hand %d: pot layers sum to %d but the pot held %d",
					trial, hand, layered, result.PotTotal)
			}

			if got := totalChips(g); got != bank {
				t.Fatalf("trial %d, hand %d: chips went from %d to %d",
					trial, hand, bank, got)
			}

			for _, p := range g.Players {
				if p.Chips < 0 {
					t.Fatalf("trial %d, hand %d: %s has %d chips", trial, hand, p.Name, p.Chips)
				}
				if p.TotalBet > 0 && p.Chips == 0 && !p.AllIn && !p.Folded {
					t.Fatalf("trial %d, hand %d: %s is broke but not marked all-in",
						trial, hand, p.Name)
				}
			}
		}
	}

	if handsPlayed < 1000 {
		t.Fatalf("only %d hands played; the fuzz gate is not exercising much", handsPlayed)
	}
}

type sourceFunc func(PlayerView) Decision

func (f sourceFunc) RequestAction(_ context.Context, v PlayerView) (Decision, error) {
	return f(v), nil
}

// Conservation invariants hold even when the wrong seat wins, so this is
// the test that actually checks eligibility: the short stack takes the
// main pot with the best hand and is locked out of the side pot above it.
func TestSidePotEligibilityAwardsTheRightSeats(t *testing.T) {
	gv := NewGame(5, 10)
	g := &gv
	short := seat(g, "Short", 0)
	mid := seat(g, "Mid", 0)
	deep := seat(g, "Deep", 0)
	g.ButtonIndex = 2

	short.TotalBet, short.AllIn = 100, true
	mid.TotalBet = 400
	deep.TotalBet = 400
	g.Pot = 900

	g.Board = deck.Board{Cards: []deck.Card{
		deck.NewCard(deck.Nine, deck.Spades), deck.NewCard(deck.Seven, deck.Hearts),
		deck.NewCard(deck.Two, deck.Clubs), deck.NewCard(deck.Five, deck.Diamonds),
		deck.NewCard(deck.King, deck.Hearts),
	}}

	// The short stack has the nuts: a set of nines.
	short.Hand = deck.Hand{Cards: []deck.Card{
		deck.NewCard(deck.Nine, deck.Hearts), deck.NewCard(deck.Nine, deck.Clubs),
	}}
	// Mid has two pair, Deep has only a pair of kings.
	mid.Hand = deck.Hand{Cards: []deck.Card{
		deck.NewCard(deck.King, deck.Spades), deck.NewCard(deck.Seven, deck.Clubs),
	}}
	deep.Hand = deck.Hand{Cards: []deck.Card{
		deck.NewCard(deck.King, deck.Clubs), deck.NewCard(deck.Three, deck.Spades),
	}}

	result := g.Payout()

	if len(result.Pots) != 2 {
		t.Fatalf("expected a main pot and a side pot, got %+v", result.Pots)
	}

	main, side := result.Pots[0], result.Pots[1]

	// 100 from each of the three players.
	if main.Amount != 300 {
		t.Errorf("main pot should be 300, got %d", main.Amount)
	}
	if len(main.Winners) != 1 || main.Winners[0] != 0 {
		t.Errorf("the short stack's set of nines should take the main pot, got %v", main.Winners)
	}

	// 300 more from each of the two deep stacks.
	if side.Amount != 600 {
		t.Errorf("side pot should be 600, got %d", side.Amount)
	}
	if len(side.Winners) != 1 || side.Winners[0] != 1 {
		t.Errorf("Mid's two pair should take the side pot the short stack cannot win, got %v",
			side.Winners)
	}

	if short.Chips != 300 {
		t.Errorf("Short should win exactly the main pot (300), got %d", short.Chips)
	}
	if mid.Chips != 600 {
		t.Errorf("Mid should win exactly the side pot (600), got %d", mid.Chips)
	}
	if deep.Chips != 0 {
		t.Errorf("Deep should win nothing, got %d", deep.Chips)
	}
}

// Redaction is a correctness requirement, not hardening: anything in the
// view reaches the player's terminal.
func TestViewForLeaksNoHoleCards(t *testing.T) {
	gv := NewGame(10, 20)
	g := &gv
	seat(g, "Alice", 1000)
	seat(g, "Bob", 1000)
	seat(g, "Charlie", 1000)

	if err := g.StartNewHand(); err != nil {
		t.Fatalf("StartNewHand: %v", err)
	}

	view := g.ViewFor(1)

	if len(view.Hole) != 2 {
		t.Fatalf("expected the viewing seat to see its own two cards, got %d", len(view.Hole))
	}
	for i, card := range view.Hole {
		if card != g.Players[1].Hand.Cards[i] {
			t.Errorf("seat 1's view shows the wrong hole cards")
		}
	}
	if len(view.Seats) != 3 {
		t.Fatalf("expected 3 seats in the view, got %d", len(view.Seats))
	}

	spectator := g.ViewFor(SpectatorSeat)
	if len(spectator.Hole) != 0 {
		t.Errorf("a spectator must see no hole cards, got %v", spectator.Hole)
	}
	if spectator.Seat != SpectatorSeat {
		t.Errorf("expected the spectator seat marker, got %d", spectator.Seat)
	}
	if len(spectator.Seats) != 3 {
		t.Errorf("a spectator should still see the public seat state")
	}
}

// A player who closes their laptop mid-hand must not hold the table.
func TestTurnTimeoutFoldsUnresponsiveSeat(t *testing.T) {
	gv := NewGame(10, 20)
	g := &gv
	g.TurnTimeout = 20 * time.Millisecond

	seat(g, "Button", 1000)
	seat(g, "SB", 1000)
	absent := seat(g, "Absent", 1000)

	g.Sources[0] = blockingSource{}

	if err := g.StartNewHand(); err != nil {
		t.Fatalf("StartNewHand: %v", err)
	}
	// Put the absent player in the big blind's shoes by raising into them.
	g.Sources[1] = sourceFunc(func(v PlayerView) Decision {
		return Decision{Action: Raise, Amount: 60}
	})
	g.Sources[2] = blockingSource{}

	done := make(chan error, 1)
	started := time.Now()
	go func() { done <- g.ExecuteBettingRound(g.firstToAct(Preflop)) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ExecuteBettingRound: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the betting round hung on an unresponsive player")
	}

	// Two absent players means two timeouts. Retries must share one
	// deadline per turn rather than each getting a fresh one, so anything
	// near four timeouts means the clock is being reset per attempt.
	if elapsed := time.Since(started); elapsed > 3*g.TurnTimeout {
		t.Errorf("two timed-out turns took %v, more than three timeouts; "+
			"retries are probably getting their own deadline", elapsed)
	}

	if !g.Players[0].Folded {
		t.Errorf("an unresponsive player facing a bet should fold")
	}
	if !absent.Folded {
		t.Errorf("the unresponsive big blind should fold to the raise")
	}
}

// The deadline is what a UI counts down, so it has to be populated.
func TestViewCarriesDeadlineWhenTimeoutSet(t *testing.T) {
	gv := NewGame(10, 20)
	g := &gv
	g.TurnTimeout = time.Minute
	seat(g, "Alice", 1000)
	seat(g, "Bob", 1000)

	var seen time.Time
	g.Sources[0] = sourceFunc(func(v PlayerView) Decision {
		seen = v.Deadline
		return Decision{Action: Fold}
	})

	if err := g.StartNewHand(); err != nil {
		t.Fatalf("StartNewHand: %v", err)
	}
	if err := g.ExecuteBettingRound(g.firstToAct(Preflop)); err != nil {
		t.Fatalf("ExecuteBettingRound: %v", err)
	}

	if seen.IsZero() {
		t.Fatal("expected the view to carry a turn deadline")
	}
	if time.Until(seen) <= 0 {
		t.Errorf("the deadline should be in the future, got %v", seen)
	}
}

// A source that burns the clock and then answers illegally is the only
// thing that distinguishes one deadline per turn from one per attempt:
// under a per-attempt clock its four retries cost four full timeouts.
func TestTurnDeadlineCoversRetries(t *testing.T) {
	gv := NewGame(10, 20)
	g := &gv
	g.TurnTimeout = 50 * time.Millisecond

	seat(g, "Alice", 1000)
	seat(g, "Bob", 1000)
	g.ButtonIndex = 0

	// Checking is illegal for the small blind facing the big blind.
	g.Sources[0] = stallingIllegalSource{}

	if err := g.StartNewHand(); err != nil {
		t.Fatalf("StartNewHand: %v", err)
	}

	started := time.Now()
	if err := g.ExecuteBettingRound(g.firstToAct(Preflop)); err != nil {
		t.Fatalf("ExecuteBettingRound: %v", err)
	}
	elapsed := time.Since(started)

	if !g.Players[0].Folded {
		t.Errorf("a seat that never answers legally should be folded")
	}
	if elapsed > 2*g.TurnTimeout {
		t.Errorf("one turn took %v, over two timeouts; retries are getting "+
			"their own deadline instead of sharing the turn's", elapsed)
	}
}

type stallingIllegalSource struct{}

func (stallingIllegalSource) RequestAction(ctx context.Context, _ PlayerView) (Decision, error) {
	<-ctx.Done()
	// Deliberately answers with nil error so the engine takes the retry
	// path rather than the give-up path.
	return Decision{Action: Check}, nil
}

type blockingSource struct{}

func (blockingSource) RequestAction(ctx context.Context, _ PlayerView) (Decision, error) {
	<-ctx.Done()
	return Decision{}, ctx.Err()
}
