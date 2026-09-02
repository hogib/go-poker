package table

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"ssh_holdem/game"
)

func TestCleanName(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"plain", "Alice", "Alice", false},
		{"trims", "  Alice  ", "Alice", false},
		{"empty", "", "", true},
		{"only spaces", "   ", "", true},
		{"only control characters", "\x1b\x07", "", true},
		{"caps the length", "AbsurdlyLongPlayerName", "AbsurdlyLong", false},
		{"keeps accents", "Renée", "Renée", false},
		{"keeps emoji", "♠ace", "♠ace", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CleanName(tc.in)

			if tc.wantErr {
				if err == nil {
					t.Errorf("expected %q to be rejected, got %q", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("CleanName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A name is written into every other player's terminal, so an escape
// sequence in one would let a player scribble on everyone's screen.
func TestCleanNameStripsEscapeSequences(t *testing.T) {
	got, err := CleanName("Bob\x1b[2J\x1b[Hgotcha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.ContainsRune(got, '\x1b') {
		t.Errorf("an escape character survived: %q", got)
	}
	for _, r := range got {
		if r < 0x20 || r == 0x7f {
			t.Errorf("a control character survived in %q", got)
		}
	}
}

// The cap is in runes, not bytes: slicing a multi-byte name by bytes
// produces mojibake and throws the seat column out of line.
func TestCleanNameCapsByRunes(t *testing.T) {
	got, err := CleanName(strings.Repeat("é", 20))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runes := []rune(got); len(runes) != MaxNameLength {
		t.Errorf("expected %d runes, got %d (%q)", MaxNameLength, len(runes), got)
	}
	if !strings.HasSuffix(got, "é") {
		t.Errorf("the name was sliced mid-rune: %q", got)
	}
}

func TestRenameChangesWhatTheTableCallsYou(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	s := tbl.Join("key-alice", "alice", (&recorder{}).notify)

	got, err := tbl.Rename("key-alice", "  Alice  ")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if got != "Alice" {
		t.Errorf("expected the cleaned name back, got %q", got)
	}
	if s.Name() != "Alice" {
		t.Errorf("the session should carry the new name, got %q", s.Name())
	}

	waitUntil(t, "the player to be renamed", 5*time.Second, func() bool {
		var name string
		return tbl.do(func() { name = s.Player.Name }, time.Second) && name == "Alice"
	})
}

// The renamer sees their own new name whatever happens, because their UI
// knows what they typed. What matters is that everyone else is told.
func TestRenameIsRepublishedToOtherPlayers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	alice, bob := &recorder{}, &recorder{}
	joinAndSit(tbl, "key-alice", "alice", alice)
	joinAndSit(tbl, "key-bob", "bob", bob)

	waitUntil(t, "both seats", 5*time.Second, func() bool {
		view, ok := bob.lastState()
		return ok && len(view.Seats) == 2
	})

	if _, err := tbl.Rename("key-alice", "Ace"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	waitUntil(t, "Bob's seat list to show the new name", 5*time.Second, func() bool {
		view, ok := bob.lastState()
		if !ok {
			return false
		}
		for _, seat := range view.Seats {
			if seat.Name == "Ace" {
				return true
			}
		}
		return false
	})
}

func TestRenameRejectsATakenName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	tbl.Join("key-alice", "Alice", (&recorder{}).notify)
	bob := tbl.Join("key-bob", "Bob", (&recorder{}).notify)

	if _, err := tbl.Rename("key-bob", "Alice"); err == nil {
		t.Error("a name already in use should be refused")
	}
	// Case is not a way around it.
	if _, err := tbl.Rename("key-bob", "ALICE"); err == nil {
		t.Error("a name should be taken regardless of case")
	}
	if bob.Name() != "Bob" {
		t.Errorf("a refused rename should leave the name alone, got %q", bob.Name())
	}

	// Renaming to what you are already called is not a clash with
	// yourself.
	if _, err := tbl.Rename("key-bob", "Bob"); err != nil {
		t.Errorf("keeping your own name should be allowed: %v", err)
	}
}

func TestRenameRejectsStrangersAndEmptyNames(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	tbl.Join("key-alice", "Alice", (&recorder{}).notify)

	if _, err := tbl.Rename("nobody", "Alice"); err == nil {
		t.Error("an unknown session should be refused")
	}
	if _, err := tbl.Rename("key-alice", "   "); err == nil {
		t.Error("an empty name should be refused")
	}
}

// Renaming while a hand is in progress must not deadlock: the queued
// change waits for the Run goroutine and the caller does not.
func TestRenameDuringAHandDoesNotBlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := testConfig()
	cfg.TurnTimeout = 5 * time.Second
	tbl := New(cfg)
	go tbl.Run(ctx)

	alice, bob := &recorder{}, &recorder{}
	joinAndSit(tbl, "key-alice", "alice", alice)
	joinAndSit(tbl, "key-bob", "bob", bob)

	// Nobody acts, so the table is stuck inside a hand.
	waitUntil(t, "a hand to be in progress", 10*time.Second, func() bool {
		return alice.turnCount() > 0 || bob.turnCount() > 0
	})

	done := make(chan error, 1)
	go func() {
		_, err := tbl.Rename("key-alice", "Ace")
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Rename: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Rename blocked while a hand was in progress")
	}
}

// The name a player chose is what a reconnect gets back, since Join
// reattaches rather than taking the name argument again.
func TestReconnectKeepsTheChosenName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	tbl.Join("key-alice", "alice", (&recorder{}).notify)
	if _, err := tbl.Rename("key-alice", "Ace"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	// A reconnect passes the ssh username again; the chosen name wins.
	again := tbl.Join("key-alice", "alice", (&recorder{}).notify)

	if again.Name() != "Ace" {
		t.Errorf("a reconnect should keep the chosen name, got %q", again.Name())
	}
}

// Renaming a seated player must not disturb their seat or their chips.
func TestRenameKeepsTheSeatAndStack(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	rec := &recorder{}
	s := tbl.Join("key-alice", "alice", rec.notify)
	tbl.Sit("key-alice")

	waitUntil(t, "the seat", 5*time.Second, func() bool {
		var seat int
		return tbl.do(func() { seat = tbl.game.SeatOf(s.Player) }, time.Second) &&
			seat != game.SpectatorSeat
	})

	var before int
	tbl.do(func() { before = s.Player.Chips }, 5*time.Second)

	if _, err := tbl.Rename("key-alice", "Ace"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	waitUntil(t, "the rename to land", 5*time.Second, func() bool {
		var name string
		return tbl.do(func() { name = s.Player.Name }, time.Second) && name == "Ace"
	})

	var seat, after int
	tbl.do(func() {
		seat = tbl.game.SeatOf(s.Player)
		after = s.Player.Chips
	}, 5*time.Second)

	if seat == game.SpectatorSeat {
		t.Error("renaming should not cost a player their seat")
	}
	if after != before {
		t.Errorf("renaming should not touch the stack: %d became %d", before, after)
	}
}

// Two people who ssh in under the same username must not both arrive as
// that name: each would then be refused their own, because it clashes
// with the other's.
func TestJoiningTwiceUnderOneUsernameGivesDistinctNames(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	first := tbl.Join("key-one", "alice", (&recorder{}).notify)
	second := tbl.Join("key-two", "alice", (&recorder{}).notify)

	if first.Name() == second.Name() {
		t.Fatalf("both players arrived as %q", first.Name())
	}
	if first.Name() != "alice" {
		t.Errorf("the first should keep the plain name, got %q", first.Name())
	}
	if second.Name() != "alice 2" {
		t.Errorf("the second should be numbered, got %q", second.Name())
	}

	// And each can now confirm the name they were given.
	for _, id := range []string{"key-one", "key-two"} {
		name := tbl.Session(id).Name()
		if _, err := tbl.Rename(id, name); err != nil {
			t.Errorf("%s could not confirm its own name %q: %v", id, name, err)
		}
	}
}

func TestNumberingKeepsNamesWithinTheLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	long := strings.Repeat("x", MaxNameLength)
	for i := 0; i < 4; i++ {
		s := tbl.Join(fmt.Sprintf("key-%d", i), long, (&recorder{}).notify)

		if runes := []rune(s.Name()); len(runes) > MaxNameLength {
			t.Errorf("session %d got a %d-rune name: %q", i, len(runes), s.Name())
		}
	}

	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		name := tbl.Session(fmt.Sprintf("key-%d", i)).Name()
		if seen[name] {
			t.Errorf("%q was handed out twice", name)
		}
		seen[name] = true
	}
}

// An ssh username is attacker-controlled text, so it gets the same
// cleaning as a name a player types.
func TestJoinCleansTheIncomingName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	s := tbl.Join("key-one", "bob\x1b[2Jgotcha", (&recorder{}).notify)

	if strings.ContainsRune(s.Name(), '\x1b') {
		t.Errorf("an escape sequence survived the ssh username: %q", s.Name())
	}

	blank := tbl.Join("key-two", "\x00\x01", (&recorder{}).notify)
	if blank.Name() == "" {
		t.Error("a username with nothing printable in it still needs a name")
	}
}

// The handle a player is given is derived from their session, so a
// reconnect before they have chosen a name finds the same one rather
// than a different one every time.
func TestDefaultNameIsStableForASession(t *testing.T) {
	if a, b := DefaultName("key:abc"), DefaultName("key:abc"); a != b {
		t.Errorf("the same session got %q and then %q", a, b)
	}

	seen := map[string]int{}
	for i := 0; i < 200; i++ {
		seen[DefaultName(fmt.Sprintf("key:%d", i))]++
	}

	if len(seen) < 5 {
		t.Errorf("handles should vary between players, only saw %d: %v", len(seen), seen)
	}
	for name := range seen {
		if _, err := CleanName(name); err != nil {
			t.Errorf("handle %q is not a usable name: %v", name, err)
		}
	}
}

// A session with no name given is handed a handle, not left blank and
// not given the caller's own text.
func TestJoinWithNoNameHandsOutAHandle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tbl := New(testConfig())
	go tbl.Run(ctx)

	s := tbl.Join("key-one", "", (&recorder{}).notify)

	if s.Name() == "" {
		t.Fatal("a nameless join should still get a handle")
	}
	if s.Name() != DefaultName("key-one") {
		t.Errorf("expected the handle for this session, got %q", s.Name())
	}
}
