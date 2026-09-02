package tui

import (
	"strings"
	"testing"
	"time"

	"ssh_holdem/deck"
	"ssh_holdem/game"
	"ssh_holdem/table"
)

// tableOf builds a view with n seats, the viewer at seat 0.
func tableOf(n, acting int) game.PlayerView {
	names := []string{"Hero", "Bob", "Carol", "Dave", "Erin", "Frank", "Gina", "Hank", "Iris"}

	seats := make([]game.SeatInfo, 0, n)
	for i := 0; i < n; i++ {
		seats = append(seats, game.SeatInfo{
			Index: i, Name: names[i], Chips: 1000 + i*100, IsButton: i == 1,
		})
	}

	return game.PlayerView{
		Seat: 0,
		Hole: []deck.Card{
			deck.NewCard(deck.Ace, deck.Spades), deck.NewCard(deck.King, deck.Hearts),
		},
		Board: []deck.Card{
			deck.NewCard(deck.Two, deck.Clubs), deck.NewCard(deck.Seven, deck.Diamonds),
			deck.NewCard(deck.Ten, deck.Spades),
		},
		Seats:      seats,
		Street:     game.Flop,
		Acting:     acting,
		Pot:        320,
		ToCall:     60,
		MinRaiseTo: 120,
		MaxRaiseTo: 1000,
		Deadline:   time.Now().Add(21 * time.Second),
		TurnLength: 30 * time.Second,
	}
}

// atFelt builds a model showing the oval on a roomy terminal.
func atFelt(n, acting int) Model {
	m, _ := atMenu()
	m = resize(m, 100, 40)
	m = send(m, table.StateMsg{View: tableOf(n, acting)})
	m.screen = screenTable
	return m
}

// rowOf reports which rendered line a piece of text lands on.
func rowOf(out, text string) int {
	for i, line := range strings.Split(out, "\n") {
		if strings.Contains(stripStyles(line), text) {
			return i
		}
	}
	return -1
}

func TestFeltIsUsedWhenThereIsRoom(t *testing.T) {
	m := atFelt(6, 1)

	if !m.useFelt() {
		t.Fatal("a 100x40 terminal has room for the table view")
	}
	if !strings.Contains(m.View(), "pot 320") {
		t.Errorf("the pot belongs in the middle of the table:\n%s", m.View())
	}
}

// Everyone at the table has to be on screen, wherever they sit.
func TestEverySeatIsDrawn(t *testing.T) {
	for n := 2; n <= 9; n++ {
		m := atFelt(n, 1)
		out := stripStyles(m.View())

		for _, seat := range m.view.Seats {
			if !strings.Contains(out, seat.Name) {
				t.Errorf("%d-handed: %s is missing from the table:\n%s", n, seat.Name, out)
			}
		}
	}
}

// You always sit at the bottom, the way every poker client does it, so
// the table reads the same however the seats are numbered.
func TestYouAlwaysSitAtTheBottom(t *testing.T) {
	for n := 2; n <= 9; n++ {
		m := atFelt(n, 1)
		out := m.View()

		hero := rowOf(out, "Hero")
		if hero < 0 {
			t.Fatalf("%d-handed: the viewer is not on screen", n)
		}

		for _, seat := range m.view.Seats[1:] {
			if row := rowOf(out, seat.Name); row > hero {
				t.Errorf("%d-handed: %s is below you (row %d against %d):\n%s",
					n, seat.Name, row, hero, out)
			}
		}
	}
}

// Whoever the table is waiting on gets a box drawn round them, which is
// the whole point of the layout.
func TestTheActingSeatIsBoxed(t *testing.T) {
	m := atFelt(6, 3)
	out := stripStyles(m.View())

	acting := m.view.Seats[3].Name
	row := rowOf(out, acting)
	if row < 0 {
		t.Fatalf("the acting player is not on screen:\n%s", out)
	}

	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[row], "│") {
		t.Errorf("the acting seat should be boxed, its row is:\n%s", lines[row])
	}

	// Nobody else is.
	for _, seat := range m.view.Seats {
		if seat.Index == 3 {
			continue
		}
		if r := rowOf(out, seat.Name); r >= 0 && strings.Contains(lines[r], "│"+seat.Name) {
			t.Errorf("%s should not be boxed", seat.Name)
		}
	}
}

// The clock belongs to the acting seat's box, so a player watching can
// see how long the person they are waiting on has left.
func TestTheClockSitsWithTheActingSeat(t *testing.T) {
	m := atFelt(6, 3)
	out := stripStyles(m.View())

	bar := rowOf(out, "█")
	acting := rowOf(out, m.view.Seats[3].Name)

	if bar < 0 {
		t.Fatalf("no clock bar was drawn:\n%s", out)
	}
	if bar <= acting || bar > acting+4 {
		t.Errorf("the clock should sit inside the acting seat's box: bar at row %d, seat at %d:\n%s",
			bar, acting, out)
	}
}

func TestSeatStatusIsShownOnTheFelt(t *testing.T) {
	m, _ := atMenu()
	m = resize(m, 100, 40)

	view := tableOf(4, 1)
	view.Seats[1].CurrentBet = 60
	view.Seats[2].Folded = true
	view.Seats[3].AllIn, view.Seats[3].CurrentBet = true, 940

	m = send(m, table.StateMsg{View: view})
	m.screen = screenTable

	out := stripStyles(m.View())
	for _, want := range []string{"bets 60", "folded", "all in 940"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q on the table:\n%s", want, out)
		}
	}
}

func TestDealerButtonIsShownOnTheFelt(t *testing.T) {
	m := atFelt(4, 0)
	out := stripStyles(m.View())

	row := rowOf(out, "Bob") // seat 1 holds the button
	if row < 0 {
		t.Fatalf("the button holder is not on screen:\n%s", out)
	}
	if !strings.Contains(strings.Split(out, "\n")[row], "D") {
		t.Errorf("the dealer button should sit with its seat:\n%s",
			strings.Split(out, "\n")[row])
	}
}

// A railbird has no seat of their own; the table still has to draw.
func TestSpectatorSeesTheWholeTable(t *testing.T) {
	m, _ := atMenu()
	m = resize(m, 100, 40)

	view := tableOf(5, 2)
	view.Seat = game.SpectatorSeat
	view.Hole = nil
	m = send(m, table.StateMsg{View: view})
	m.screen = screenTable

	out := stripStyles(m.View())
	for _, seat := range view.Seats {
		if !strings.Contains(out, seat.Name) {
			t.Errorf("a spectator should see %s:\n%s", seat.Name, out)
		}
	}
	if strings.Contains(out, "A♠") {
		t.Errorf("a spectator must not be shown hole cards:\n%s", out)
	}
}

// ---- falling back ----------------------------------------------------

func TestFeltGivesWayOnASmallTerminal(t *testing.T) {
	for _, size := range [][2]int{{70, 40}, {100, 18}, {40, 12}} {
		m := atFelt(6, 1)
		m = resize(m, size[0], size[1])

		if m.useFelt() {
			t.Errorf("%dx%d has no room for the table view", size[0], size[1])
		}
		// The compact list still shows everyone.
		out := stripStyles(m.View())
		if !strings.Contains(out, "Hero") {
			t.Errorf("%dx%d: the compact view lost the seats:\n%s", size[0], size[1], out)
		}
	}
}

func TestVTogglesBetweenTableAndCompact(t *testing.T) {
	m := atFelt(6, 1)
	if !m.useFelt() {
		t.Fatal("expected the table view to start")
	}

	m = press(m, "v")
	if m.useFelt() {
		t.Error("v should switch to the compact view")
	}
	if !strings.Contains(m.View(), "Compact view") {
		t.Errorf("the switch should be acknowledged:\n%s", m.View())
	}

	m = press(m, "v")
	if !m.useFelt() {
		t.Error("v should switch back to the table view")
	}
}

// Whichever layout is showing, nothing may spill past the edge.
func TestFeltFitsEveryTerminalAndTableSize(t *testing.T) {
	for n := 2; n <= 9; n++ {
		for _, size := range [][2]int{{40, 12}, {72, 24}, {80, 30}, {100, 40}, {200, 60}} {
			m, _ := atMenu()
			m = resize(m, size[0], size[1])
			m = send(m, table.StateMsg{View: tableOf(n, 1)})
			m.screen = screenTable

			if got := widest(m.View()); got > size[0] {
				t.Errorf("%d-handed at %dx%d rendered %d columns:\n%s",
					n, size[0], size[1], got, m.View())
			}
		}
	}
}

// ---- seat placement --------------------------------------------------

func TestSlotsForSpreadsPlayersAround(t *testing.T) {
	for n := 1; n <= slotCount; n++ {
		slots := slotsFor(n)

		if len(slots) != n {
			t.Errorf("%d players got %d positions", n, len(slots))
		}

		seen := map[int]bool{}
		for _, slot := range slots {
			if slot < 0 || slot >= slotCount {
				t.Errorf("%d players: position %d is off the table", n, slot)
			}
			if seen[slot] {
				t.Errorf("%d players: two players were put in position %d", n, slot)
			}
			seen[slot] = true
		}

		if len(slots) > 0 && slots[0] != slotBottomCentre {
			t.Errorf("%d players: the viewer should be at the bottom, got position %d",
				n, slots[0])
		}
	}
}

func TestSlotsForHandlesEmptyTables(t *testing.T) {
	if got := slotsFor(0); len(got) != 0 {
		t.Errorf("no players should need no positions, got %v", got)
	}
	if got := slotsFor(-1); len(got) != 0 {
		t.Errorf("a negative count should be ignored, got %v", got)
	}
}

// Seats are placed clockwise from the viewer, so the player to your left
// is drawn to your left whatever seat numbers the engine handed out.
func TestSeatsArePlacedClockwiseFromYou(t *testing.T) {
	m, _ := atMenu()
	m = resize(m, 100, 40)

	view := tableOf(4, 0)
	view.Seat = 2 // the viewer is seat 2, not seat 0
	m = send(m, table.StateMsg{View: view})
	m.screen = screenTable

	placed := m.placeSeats()

	if placed[slotBottomCentre].Index != 2 {
		t.Errorf("the viewer should be at the bottom, found seat %d",
			placed[slotBottomCentre].Index)
	}

	// Walking the ring from the bottom must give the seats in order.
	want := []int{2, 3, 0, 1}
	got := make([]int, 0, len(want))
	for _, slot := range slotsFor(4) {
		got = append(got, placed[slot].Index)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("expected seats %v clockwise from the viewer, got %v", want, got)
			break
		}
	}
}

func TestFeltHandlesAnEmptyTable(t *testing.T) {
	m, _ := atMenu()
	m = resize(m, 100, 40)
	m = send(m, table.StateMsg{View: game.PlayerView{Seat: game.SpectatorSeat}})
	m.screen = screenTable

	if out := m.View(); out == "" {
		t.Error("an empty table should still render something")
	}
}
