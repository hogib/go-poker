package table

import (
	"context"
	"testing"
	"time"

	"ssh_holdem/game"
)

// Connecting puts you in the lobby, not in a hand. A new arrival should
// never be dealt into whatever is already going on.
func TestJoinDoesNotTakeASeat(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	rec := &recorder{}
	s := tbl.Join("key-watcher", "Watcher", rec.notify)

	var seated int
	if !tbl.do(func() { seated = tbl.game.SeatOf(s.Player) }, 5*time.Second) {
		t.Fatal("the table never processed the read")
	}

	if seated != game.SpectatorSeat {
		t.Errorf("joining should leave you watching, got seat %d", seated)
	}
}

// The lobby needs the house rules and the seat counts to draw its menu.
func TestJoinSendsLobbyStateImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := testConfig()
	tbl := New(cfg)
	go tbl.Run(ctx)

	rec := &recorder{}
	tbl.Join("key-watcher", "Watcher", rec.notify)

	waitUntil(t, "the lobby state", 2*time.Second, func() bool {
		_, ok := rec.lastLobby()
		return ok
	})

	lobby, _ := rec.lastLobby()
	if lobby.Rules.BigBlind != cfg.BigBlind || lobby.Rules.BuyIn != cfg.BuyIn {
		t.Errorf("the lobby should carry the house rules, got %+v", lobby.Rules)
	}
	if lobby.YouAreSeated {
		t.Error("a fresh arrival is not seated")
	}
	if lobby.Watching < 1 {
		t.Errorf("expected at least one watcher, got %d", lobby.Watching)
	}
}

func TestSitTakesASeatAndStandGivesItBack(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	rec := &recorder{}
	s := tbl.Join("key-alice", "Alice", rec.notify)

	if !tbl.Sit("key-alice") {
		t.Fatal("Sit should be accepted for a connected session")
	}
	waitUntil(t, "the seat", 5*time.Second, func() bool {
		var seat int
		return tbl.do(func() { seat = tbl.game.SeatOf(s.Player) }, time.Second) &&
			seat != game.SpectatorSeat
	})

	var chips int
	tbl.do(func() { chips = s.Player.Chips }, 5*time.Second)
	if chips != tbl.Config().BuyIn {
		t.Errorf("expected the buy-in of %d, got %d", tbl.Config().BuyIn, chips)
	}

	if !tbl.Stand("key-alice") {
		t.Fatal("Stand should be accepted for a seated session")
	}
	waitUntil(t, "the seat to be given up", 5*time.Second, func() bool {
		var seat int
		return tbl.do(func() { seat = tbl.game.SeatOf(s.Player) }, time.Second) &&
			seat == game.SpectatorSeat
	})

	// Standing keeps you connected: you are back to watching, not gone.
	if tbl.session("key-alice") == nil {
		t.Error("standing up should not disconnect the session")
	}
}

// Standing up banks the stack, so sitting back down returns it rather
// than handing out a fresh buy-in.
func TestStandThenSitKeepsTheStack(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	rec := &recorder{}
	s := tbl.Join("key-alice", "Alice", rec.notify)
	tbl.Sit("key-alice")

	waitUntil(t, "the seat", 5*time.Second, func() bool {
		var seat int
		return tbl.do(func() { seat = tbl.game.SeatOf(s.Player) }, time.Second) &&
			seat != game.SpectatorSeat
	})

	// Spend some chips so the stack is distinguishable from the buy-in.
	tbl.do(func() { s.Player.Chips = 137 }, 5*time.Second)

	tbl.Stand("key-alice")
	waitUntil(t, "the stack to be banked", 5*time.Second, func() bool {
		var banked int
		return tbl.do(func() { banked = tbl.banked["key-alice"] }, time.Second) &&
			banked == 137
	})

	tbl.Sit("key-alice")
	waitUntil(t, "the seat back", 5*time.Second, func() bool {
		var seat int
		return tbl.do(func() { seat = tbl.game.SeatOf(s.Player) }, time.Second) &&
			seat != game.SpectatorSeat
	})

	var chips int
	tbl.do(func() { chips = s.Player.Chips }, 5*time.Second)
	if chips != 137 {
		t.Errorf("sitting back down should return the banked 137, got %d", chips)
	}
}

func TestSitAndStandRejectStrangers(t *testing.T) {
	tbl := New(testConfig())

	if tbl.Sit("nobody") {
		t.Error("Sit should reject an unknown session")
	}
	if tbl.Stand("nobody") {
		t.Error("Stand should reject an unknown session")
	}
}

// Sitting twice must not produce two seats for one player.
func TestSitTwiceIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	rec := &recorder{}
	s := tbl.Join("key-alice", "Alice", rec.notify)

	tbl.Sit("key-alice")
	tbl.Sit("key-alice")
	tbl.Sit("key-alice")

	waitUntil(t, "the seat", 5*time.Second, func() bool {
		var seat int
		return tbl.do(func() { seat = tbl.game.SeatOf(s.Player) }, time.Second) &&
			seat != game.SpectatorSeat
	})

	var seats int
	tbl.do(func() { seats = len(tbl.game.Players) }, 5*time.Second)
	if seats != 1 {
		t.Errorf("one player asking three times should still be one seat, got %d", seats)
	}
}

// The seat and watcher counts must be right the moment someone sits,
// not only once a hand has been dealt.
func TestLobbyCountsUpdateOnSeating(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	alice, bob := &recorder{}, &recorder{}
	tbl.Join("key-alice", "Alice", alice.notify)
	tbl.Join("key-bob", "Bob", bob.notify)

	waitUntil(t, "both watchers in the lobby count", 5*time.Second, func() bool {
		lobby, ok := bob.lastLobby()
		return ok && lobby.Watching == 2 && lobby.Seated == 0
	})

	tbl.Sit("key-alice")

	waitUntil(t, "the seated count to move", 5*time.Second, func() bool {
		lobby, ok := bob.lastLobby()
		return ok && lobby.Seated == 1 && lobby.Watching == 1
	})

	waitUntil(t, "Alice to be told she is seated", 5*time.Second, func() bool {
		lobby, ok := alice.lastLobby()
		return ok && lobby.YouAreSeated && lobby.YourChips == tbl.Config().BuyIn
	})
}
