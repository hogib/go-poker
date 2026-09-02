package table

import (
	"context"
	"sync"
	"testing"
	"time"

	"go_poker/game"
)

// recorder stands in for a terminal: it captures everything pushed to a
// session and reports whether that session is on the clock.
type recorder struct {
	mu     sync.Mutex
	states []game.PlayerView
	turns  []game.PlayerView
	infos  []string
	result *game.HandResult
}

func (r *recorder) notify(msg any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch m := msg.(type) {
	case StateMsg:
		r.states = append(r.states, m.View)
	case TurnMsg:
		r.turns = append(r.turns, m.View)
	case InfoMsg:
		r.infos = append(r.infos, m.Text)
	case ResultMsg:
		res := m.Result
		r.result = &res
	}
}

func (r *recorder) pendingTurn() (game.PlayerView, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.turns) == 0 {
		return game.PlayerView{}, false
	}
	return r.turns[len(r.turns)-1], true
}

func (r *recorder) turnCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.turns)
}

func (r *recorder) sawResult() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.result != nil
}

func (r *recorder) lastState() (game.PlayerView, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.states) == 0 {
		return game.PlayerView{}, false
	}
	return r.states[len(r.states)-1], true
}

func testConfig() Config {
	return Config{
		SmallBlind:  10,
		BigBlind:    20,
		BuyIn:       1000,
		TurnTimeout: 2 * time.Second,
		HandDelay:   time.Millisecond,
	}
}

// autoPlay answers every turn for a session until the context ends. It is
// the test's stand-in for a player mashing check/call.
func autoPlay(ctx context.Context, t *Table, id string, r *recorder) {
	seen := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Millisecond):
		}

		if r.turnCount() == seen {
			continue
		}
		view, ok := r.pendingTurn()
		if !ok {
			continue
		}

		d := game.Decision{Action: game.Check}
		if view.ToCall > 0 {
			d = game.Decision{Action: game.Call}
		}

		if t.Act(id, d) {
			seen = r.turnCount()
		}
	}
}

func TestTablePlaysAHandForTwoSessions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	alice, bob := &recorder{}, &recorder{}
	tbl.Join("key-alice", "Alice", alice.notify)
	tbl.Join("key-bob", "Bob", bob.notify)

	go autoPlay(ctx, tbl, "key-alice", alice)
	go autoPlay(ctx, tbl, "key-bob", bob)

	deadline := time.After(10 * time.Second)
	for !alice.sawResult() || !bob.sawResult() {
		select {
		case <-deadline:
			t.Fatalf("no hand completed: alice turns=%d bob turns=%d",
				alice.turnCount(), bob.turnCount())
		case <-time.After(5 * time.Millisecond):
		}
	}

	for name, r := range map[string]*recorder{"Alice": alice, "Bob": bob} {
		view, ok := r.lastState()
		if !ok {
			t.Fatalf("%s never received a state snapshot", name)
		}
		if len(view.Seats) != 2 {
			t.Errorf("%s should see both seats, saw %d", name, len(view.Seats))
		}
		if r.turnCount() == 0 {
			t.Errorf("%s was never put on the clock", name)
		}
	}
}

// Every snapshot a session receives must carry that session's hole cards
// and nobody else's. SeatInfo has no card field, so this checks the other
// half: that the seat marker and hole cards line up.
func TestTableSnapshotsStayRedacted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	alice, bob := &recorder{}, &recorder{}
	sa := tbl.Join("key-alice", "Alice", alice.notify)
	sb := tbl.Join("key-bob", "Bob", bob.notify)

	go autoPlay(ctx, tbl, "key-alice", alice)
	go autoPlay(ctx, tbl, "key-bob", bob)

	deadline := time.After(10 * time.Second)
	for !alice.sawResult() {
		select {
		case <-deadline:
			t.Fatal("no hand completed")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()

	check := func(name string, r *recorder, want *Session, other *Session) {
		r.mu.Lock()
		defer r.mu.Unlock()

		for _, view := range r.states {
			if view.Seat == game.SpectatorSeat {
				if len(view.Hole) != 0 {
					t.Errorf("%s got hole cards in a spectator view", name)
				}
				continue
			}
			if len(view.Hole) == 0 {
				continue
			}
			if view.Seats[view.Seat].Name != want.Name {
				t.Errorf("%s's view is anchored to seat %q", name, view.Seats[view.Seat].Name)
			}
			for _, seat := range view.Seats {
				if seat.Name == other.Name {
					// The only thing carried about another seat is public
					// state; there is no field that could hold their cards.
					if seat.Name == want.Name {
						t.Errorf("%s appears twice in the seat list", name)
					}
				}
			}
		}
	}

	check("Alice", alice, sa, sb)
	check("Bob", bob, sb, sa)
}

// A player who drops mid-hand must not hold the table for the whole shot
// clock: their turn should give up as soon as the session closes.
func TestDisconnectDoesNotStallTheTable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := testConfig()
	cfg.TurnTimeout = 30 * time.Second // long enough that waiting it out fails
	tbl := New(cfg)
	go tbl.Run(ctx)

	alice, ghost := &recorder{}, &recorder{}
	tbl.Join("key-alice", "Alice", alice.notify)
	ghostSession := tbl.Join("key-ghost", "Ghost", ghost.notify)

	go autoPlay(ctx, tbl, "key-alice", alice)

	// Wait until the ghost is actually on the clock, then pull the plug.
	deadline := time.After(5 * time.Second)
	for ghost.turnCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("the ghost was never put on the clock")
		case <-time.After(2 * time.Millisecond):
		}
	}

	dropped := time.Now()
	tbl.Leave(ghostSession)

	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("the table stalled for %v after a disconnect", time.Since(dropped))
	case <-waitFor(func() bool { return alice.sawResult() }):
	}

	if elapsed := time.Since(dropped); elapsed > 5*time.Second {
		t.Errorf("the hand took %v to finish after the disconnect", elapsed)
	}
}

// Reconnecting with the same key keeps the seat and the chips.
func TestReconnectKeepsTheSeat(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	first := &recorder{}
	original := tbl.Join("key-alice", "Alice", first.notify)
	tbl.Join("key-bob", "Bob", (&recorder{}).notify)

	second := &recorder{}
	again := tbl.Join("key-alice", "Alice", second.notify)

	if again != original {
		t.Fatal("reconnecting with the same key should reattach to the same session")
	}
	if again.Player != original.Player {
		t.Error("a reconnect must keep the same chip stack")
	}

	deadline := time.After(2 * time.Second)
	for {
		if _, ok := second.lastState(); ok {
			break
		}
		select {
		case <-deadline:
			t.Fatal("the reconnected session received no state")
		case <-time.After(2 * time.Millisecond):
		}
	}
}

func TestActIgnoresPlayersNotOnTheClock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	alice, bob := &recorder{}, &recorder{}
	tbl.Join("key-alice", "Alice", alice.notify)
	tbl.Join("key-bob", "Bob", bob.notify)

	if tbl.Act("key-nobody", game.Decision{Action: game.Fold}) {
		t.Error("a stranger's decision should be rejected")
	}

	deadline := time.After(5 * time.Second)
	for alice.turnCount() == 0 && bob.turnCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("nobody was put on the clock")
		case <-time.After(2 * time.Millisecond):
		}
	}

	// Whoever is not on the clock must not be able to act.
	offClock := "key-alice"
	if alice.turnCount() > 0 {
		offClock = "key-bob"
	}
	if tbl.Act(offClock, game.Decision{Action: game.Fold}) {
		t.Errorf("%s acted out of turn", offClock)
	}
}

func waitFor(cond func() bool) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for !cond() {
			time.Sleep(2 * time.Millisecond)
		}
	}()
	return done
}

// Reconnecting after a full disconnect must return the player's own
// stack. Without banking, a losing player could drop and rejoin for a
// fresh buy-in, which is free money.
func TestReconnectAfterLeaveKeepsChips(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := testConfig()
	tbl := New(cfg)
	go tbl.Run(ctx)

	alice, bob := &recorder{}, &recorder{}
	aliceSession := tbl.Join("key-alice", "Alice", alice.notify)
	bobSession := tbl.Join("key-bob", "Bob", bob.notify)

	playCtx, stopPlay := context.WithCancel(ctx)
	go autoPlay(playCtx, tbl, "key-alice", alice)
	go autoPlay(playCtx, tbl, "key-bob", bob)

	// Play a hand so the stacks are no longer the buy-in.
	deadline := time.After(10 * time.Second)
	for !bob.sawResult() {
		select {
		case <-deadline:
			t.Fatal("no hand completed")
		case <-time.After(5 * time.Millisecond):
		}
	}
	stopPlay()

	// Empty the table before checking anything. With fewer than two
	// players no hand can start, so nothing moves chips underneath the
	// assertions -- which is what made an earlier version of this test
	// flake when a blind happened to land Bob on the buy-in exactly.
	tbl.Leave(aliceSession)
	tbl.Leave(bobSession)

	var banked int
	if !tbl.do(func() { banked = tbl.banked["key-bob"] }, 10*time.Second) {
		t.Fatal("the table never processed the read")
	}
	if banked <= 0 {
		t.Fatalf("Bob's stack was not banked when he stood: %d", banked)
	}

	returning := tbl.Join("key-bob", "Bob", (&recorder{}).notify)
	if returning == bobSession {
		t.Fatal("a full disconnect should produce a new session")
	}

	var seated int
	if !tbl.do(func() { seated = returning.Player.Chips }, 10*time.Second) {
		t.Fatal("the table never seated the returning player")
	}

	if seated != banked {
		t.Errorf("Bob banked %d and sat back down with %d (buy-in is %d)",
			banked, seated, cfg.BuyIn)
	}
}

// fundSeat decides what stack a player sits down with. It runs only on
// the Run goroutine, so it can be checked directly.
func TestFundSeat(t *testing.T) {
	tbl := New(testConfig())
	buyIn := tbl.Config().BuyIn

	t.Run("a new player gets the buy-in", func(t *testing.T) {
		s := newSession("new", "New", 0, nil)
		tbl.fundSeat(s)
		if s.Player.Chips != buyIn {
			t.Errorf("expected %d, got %d", buyIn, s.Player.Chips)
		}
	})

	t.Run("a returning player gets their own stack back", func(t *testing.T) {
		s := newSession("back", "Back", 0, nil)
		tbl.banked["back"] = 137
		tbl.fundSeat(s)
		if s.Player.Chips != 137 {
			t.Errorf("expected the banked 137, got %d", s.Player.Chips)
		}
		if _, still := tbl.banked["back"]; still {
			t.Error("the banked stack should be consumed when it is paid out")
		}
	})

	t.Run("a busted player gets a fresh buy-in", func(t *testing.T) {
		s := newSession("bust", "Bust", 0, nil)
		tbl.banked["bust"] = 0
		tbl.fundSeat(s)
		if s.Player.Chips != buyIn {
			t.Errorf("expected a fresh %d, got %d", buyIn, s.Player.Chips)
		}
	})

	t.Run("a seated player is not topped up", func(t *testing.T) {
		s := newSession("rich", "Rich", 0, nil)
		s.Player.Chips = 42
		tbl.fundSeat(s)
		if s.Player.Chips != 42 {
			t.Errorf("a player with chips should keep exactly them, got %d", s.Player.Chips)
		}
	})
}
