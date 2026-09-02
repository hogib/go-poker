package game

import (
	"fmt"
	"testing"

	"ssh_holdem/player"
)

// ringSeats renders the seating ring for failure messages.
func ringSeats(g *Game) string {
	out := ""
	for pos, p := range g.order {
		name := "--"
		if p != nil {
			name = p.Name
		}
		if pos > 0 {
			out += " "
		}
		out += name
	}
	return out
}

func nameAt(g *Game, seat int) string {
	if seat == SpectatorSeat || seat >= len(g.Players) {
		return "dead"
	}
	return g.Players[seat].Name
}

func ringTable(names ...string) *Game {
	gv := NewGame(5, 10)
	g := &gv
	for _, name := range names {
		p := player.NewPlayer(name, 1000)
		g.AddPlayer(&p)
	}
	g.SetButton(0)
	return g
}

// The dead button rule exists to keep one promise: the big blind advances
// exactly one seat per hand. Nobody posts it twice running, and nobody
// gets skipped, however players come and go.
func TestBigBlindAdvancesExactlyOneSeatPerHand(t *testing.T) {
	g := ringTable("A", "B", "C", "D")

	var posted []string
	for hand := 0; hand < 12; hand++ {
		_, bb := g.getBlindIndices()
		posted = append(posted, nameAt(g, bb))

		// D busts partway through the first orbit, which is exactly when
		// a naive button starts double-dealing positions.
		if hand == 1 {
			g.RemovePlayer(g.Players[g.SeatOf(findPlayer(g, "D"))])
		}

		g.MoveButton()
	}

	for i := 1; i < len(posted); i++ {
		if posted[i] == posted[i-1] {
			t.Errorf("%s posted the big blind twice running (hand %d): %v",
				posted[i], i, posted)
		}
	}

	// Once D is gone, each orbit of three hands must cover all three
	// remaining players exactly once.
	for start := 3; start+3 <= len(posted); start++ {
		seen := map[string]int{}
		for _, name := range posted[start : start+3] {
			seen[name]++
		}
		if len(seen) != 3 {
			t.Errorf("hands %d-%d did not give every player the big blind once: %v",
				start, start+2, posted[start:start+3])
		}
	}
}

func findPlayer(g *Game, name string) *player.Player {
	for _, p := range g.Players {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// When the seat that owes the small blind has been vacated, nobody posts
// it. The blind is dead, not passed to a neighbour.
func TestSmallBlindGoesDeadWhenItsSeatIsVacated(t *testing.T) {
	g := ringTable("A", "B", "C", "D")

	// Button A, small blind B, big blind C.
	if sb, bb := g.getBlindIndices(); nameAt(g, sb) != "B" || nameAt(g, bb) != "C" {
		t.Fatalf("expected B and C on the blinds, got %s and %s",
			nameAt(g, sb), nameAt(g, bb))
	}

	// C is about to take the big blind again next orbit; remove D so the
	// big blind has to walk over an empty seat.
	g.RemovePlayer(findPlayer(g, "D"))

	sawDeadSmallBlind := false
	for hand := 0; hand < 4; hand++ {
		g.MoveButton()
		sb, bb := g.getBlindIndices()

		if bb == SpectatorSeat {
			t.Fatalf("the big blind must always be live, ring %q", ringSeats(g))
		}
		if sb == SpectatorSeat {
			sawDeadSmallBlind = true
		}
	}

	if !sawDeadSmallBlind {
		t.Error("removing a player between the button and the big blind should " +
			"leave the small blind dead for one hand")
	}
}

// A dead small blind is not posted at all: the pot is short by exactly
// that much, and no neighbour is charged for it.
func TestDeadSmallBlindIsNotPosted(t *testing.T) {
	g := ringTable("A", "B", "C", "D")

	// Walk the ring until the small blind is dead, then deal.
	for hand := 0; hand < 8; hand++ {
		g.RemovePlayer(findPlayer(g, "D"))
		g.MoveButton()

		sb, _ := g.getBlindIndices()
		if sb != SpectatorSeat {
			continue
		}

		before := map[string]int{}
		for _, p := range g.Players {
			before[p.Name] = p.Chips
		}

		if err := g.StartNewHand(); err != nil {
			t.Fatalf("StartNewHand: %v", err)
		}

		if g.Pot != g.BigBlind {
			t.Errorf("with a dead small blind the pot should hold just the big "+
				"blind (%d), got %d", g.BigBlind, g.Pot)
		}

		posted := 0
		for _, p := range g.Players {
			if diff := before[p.Name] - p.Chips; diff > 0 {
				posted++
				if diff != g.BigBlind {
					t.Errorf("%s put in %d; only the big blind should be posted",
						p.Name, diff)
				}
			}
		}
		if posted != 1 {
			t.Errorf("expected exactly one player to post, got %d", posted)
		}
		return
	}

	t.Skip("the ring never produced a dead small blind in this arrangement")
}

// The button itself can land on a vacated seat. Nothing may index Players
// with it while it is there.
func TestDeadButtonNeverIndexesPlayers(t *testing.T) {
	g := ringTable("A", "B", "C", "D")
	g.RemovePlayer(findPlayer(g, "C"))

	sawDeadButton := false
	for hand := 0; hand < 8; hand++ {
		g.MoveButton()

		if g.ButtonIndex == SpectatorSeat {
			sawDeadButton = true
		}

		// Every derived seat must still be a real, indexable player.
		for _, street := range []Street{Preflop, Flop, Turn, River} {
			seat := g.firstToAct(street)
			if seat < 0 || seat >= len(g.Players) {
				t.Fatalf("firstToAct(%v) returned seat %d with %d players, ring %q",
					street, seat, len(g.Players), ringSeats(g))
			}
		}

		for _, seat := range g.clockwiseFromButton() {
			if seat < 0 || seat >= len(g.Players) {
				t.Fatalf("clockwiseFromButton returned seat %d, ring %q",
					seat, ringSeats(g))
			}
		}

		if err := g.StartNewHand(); err != nil {
			t.Fatalf("StartNewHand: %v", err)
		}
	}

	if !sawDeadButton {
		t.Error("removing the seat between the button and the small blind should " +
			"park the button on an empty seat for one hand")
	}
}

// A vacated seat is not kept forever: it leaves the ring once the button
// has passed over it.
func TestVacatedSeatLeavesTheRingAfterAnOrbit(t *testing.T) {
	g := ringTable("A", "B", "C", "D")
	g.RemovePlayer(findPlayer(g, "D"))

	if len(g.order) != 4 {
		t.Fatalf("the vacated seat should be held open, ring %q", ringSeats(g))
	}

	for hand := 0; hand < 6; hand++ {
		g.MoveButton()
	}

	if len(g.order) != 3 {
		t.Errorf("after a full orbit the ring should be back to three seats, got %q",
			ringSeats(g))
	}
	for _, p := range g.order {
		if p == nil {
			t.Errorf("no empty seat should survive an orbit, ring %q", ringSeats(g))
		}
	}
}

// Heads-up has no dead seats: the button is the small blind and both
// positions alternate every hand.
func TestHeadsUpAlternatesWithNoDeadSeats(t *testing.T) {
	g := ringTable("A", "B", "C")
	g.RemovePlayer(findPlayer(g, "C"))

	var buttons, bigBlinds []string
	for hand := 0; hand < 6; hand++ {
		g.MoveButton()

		sb, bb := g.getBlindIndices()
		if sb == SpectatorSeat {
			t.Fatalf("heads-up has no dead small blind, ring %q", ringSeats(g))
		}
		if g.ButtonIndex == SpectatorSeat {
			t.Fatalf("heads-up has no dead button, ring %q", ringSeats(g))
		}
		if sb != g.ButtonIndex {
			t.Errorf("heads-up the button posts the small blind: button %s, sb %s",
				nameAt(g, g.ButtonIndex), nameAt(g, sb))
		}

		buttons = append(buttons, nameAt(g, g.ButtonIndex))
		bigBlinds = append(bigBlinds, nameAt(g, bb))
	}

	for i := 1; i < len(buttons); i++ {
		if buttons[i] == buttons[i-1] {
			t.Errorf("heads-up the button must alternate every hand: %v", buttons)
			break
		}
		if bigBlinds[i] == bigBlinds[i-1] {
			t.Errorf("heads-up the big blind must alternate every hand: %v", bigBlinds)
			break
		}
	}
}

// The old implementation moved the button backwards when its occupant
// busted, so the same seat could hold it twice. This pins the fix at the
// level a player would notice.
func TestButtonDoesNotRepeatWhenItsOccupantBusts(t *testing.T) {
	g := ringTable("A", "B", "C", "D")

	var buttons []string
	for hand := 0; hand < 8; hand++ {
		buttons = append(buttons, nameAt(g, g.ButtonIndex))

		if hand == 0 {
			// The player on the button busts out.
			g.Players[g.ButtonIndex].Chips = 0
			g.RemoveBustedPlayers()
		}

		g.MoveButton()
	}

	for i := 1; i < len(buttons); i++ {
		if buttons[i] != "dead" && buttons[i] == buttons[i-1] {
			t.Errorf("%s held the button two hands running: %v", buttons[i], buttons)
		}
	}
}

func TestSetButtonIgnoresSeatsThatDoNotExist(t *testing.T) {
	g := ringTable("A", "B", "C")
	before := g.ButtonIndex

	g.SetButton(-1)
	g.SetButton(99)

	if g.ButtonIndex != before {
		t.Errorf("an out-of-range seat should leave the button alone, moved to %d",
			g.ButtonIndex)
	}
}

func TestRingStaysConsistentThroughChurn(t *testing.T) {
	g := ringTable("A", "B", "C", "D", "E")

	for hand := 0; hand < 30; hand++ {
		g.MoveButton()

		if occupied := g.occupied(); occupied != len(g.Players) {
			t.Fatalf("hand %d: ring holds %d players but Players has %d (%q)",
				hand, occupied, len(g.Players), ringSeats(g))
		}

		if g.ButtonIndex != SpectatorSeat {
			if got := g.order[g.buttonPos]; got != g.Players[g.ButtonIndex] {
				t.Fatalf("hand %d: ButtonIndex %d disagrees with the ring (%q)",
					hand, g.ButtonIndex, ringSeats(g))
			}
		}

		if hand == 4 {
			g.RemovePlayer(findPlayer(g, "B"))
		}
		if hand == 9 {
			g.RemovePlayer(findPlayer(g, "D"))
		}
		if hand == 14 {
			p := player.NewPlayer("F", 1000)
			if err := g.AddPlayer(&p); err != nil {
				t.Fatalf("AddPlayer: %v", err)
			}
		}
	}

	if len(g.Players) != 4 {
		t.Errorf("expected A, C, E and F at the table, got %d: %s",
			len(g.Players), fmt.Sprint(g.Players))
	}
}
