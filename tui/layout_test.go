package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"ssh_holdem/game"
	"ssh_holdem/table"
)

func resize(m Model, w, h int) Model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

// widest reports the longest rendered line, which is what has to fit.
func widest(out string) int {
	longest := 0
	for _, line := range strings.Split(out, "\n") {
		if w := lineWidth(line); w > longest {
			longest = w
		}
	}
	return longest
}

// Every screen has to fit the terminal it is drawn on. A line wider than
// the window wraps, and a wrapped bordered panel looks broken.
func TestEveryScreenFitsTheTerminal(t *testing.T) {
	sizes := []struct{ w, h int }{
		{40, 12},  // a phone in an ssh client
		{60, 20},  // a narrow split
		{80, 24},  // the classic
		{120, 40}, // roomy
		{200, 60}, // very wide
	}

	for _, size := range sizes {
		for _, sc := range []screen{screenName, screenMenu, screenTable, screenRules, screenHelp} {
			m, _ := atMenu()
			m = send(m, lobby(2, 1, true))
			m = send(m, table.StateMsg{View: seatedView(40)})
			m = resize(m, size.w, size.h)
			m.screen = sc

			if got := widest(m.View()); got > size.w {
				t.Errorf("screen %v at %dx%d rendered %d columns wide:\n%s",
					sc, size.w, size.h, got, m.View())
			}
		}
	}
}

// The panel should use the space it is given rather than sitting at a
// fixed width in the corner of a wide terminal.
func TestPanelsGrowWithTheTerminal(t *testing.T) {
	m, _ := atMenu()
	m = send(m, lobby(2, 1, true))

	narrow := widest(resize(m, 50, 24).View())
	wide := widest(resize(m, 100, 24).View())

	if wide <= narrow {
		t.Errorf("a wider terminal should get a wider panel: %d at 50 columns, %d at 100",
			narrow, wide)
	}
}

// Past a point more width is just a longer line to read, so the panel
// stops growing.
func TestPanelsStopGrowingOnAVeryWideTerminal(t *testing.T) {
	m, _ := atMenu()
	m = send(m, lobby(2, 1, true))

	wide := widest(resize(m, 120, 40).View())
	huge := widest(resize(m, 400, 40).View())

	if huge != wide {
		t.Errorf("the layout should cap: %d columns at 120, %d at 400", wide, huge)
	}
}

// On a narrow terminal the tagline is the first thing to go.
func TestTaglineDropsOnANarrowTerminal(t *testing.T) {
	m, _ := atMenu()

	if !strings.Contains(resize(m, 100, 24).View(), "no-limit texas") {
		t.Error("a roomy terminal should show the tagline")
	}
	if strings.Contains(resize(m, 44, 24).View(), "no-limit texas") {
		t.Error("a narrow terminal should drop the tagline rather than wrap it")
	}
}

// A short terminal trims the hand log instead of pushing the felt off
// the top of the screen.
func TestShortTerminalTrimsTheLog(t *testing.T) {
	m, _ := atMenu()
	m = send(m, table.StateMsg{View: seatedView(0)})
	m.screen = screenTable

	for i := 0; i < logLines; i++ {
		m = send(m, table.InfoMsg{Text: strings.Repeat("line ", 1) + string(rune('a'+i))})
	}

	tall := strings.Count(resize(m, 100, 60).View(), "\n")
	short := strings.Count(resize(m, 100, 14).View(), "\n")

	if short >= tall {
		t.Errorf("a short terminal should render fewer lines: %d at 60 rows, %d at 14",
			tall, short)
	}
}

// A resize must never lose state or crash, whatever order it arrives in.
func TestResizingIsSafeAtAnySize(t *testing.T) {
	m, _ := atMenu()
	m = send(m, lobby(2, 1, true))
	m = send(m, table.StateMsg{View: seatedView(40)})

	for _, size := range [][2]int{{0, 0}, {1, 1}, {10, 3}, {80, 24}, {500, 200}, {20, 5}} {
		m = resize(m, size[0], size[1])
		for _, sc := range []screen{screenName, screenMenu, screenTable, screenRules, screenHelp} {
			m.screen = sc
			if out := m.View(); out == "" {
				t.Errorf("screen %v rendered nothing at %dx%d", sc, size[0], size[1])
			}
		}
	}

	if !m.hasView {
		t.Error("resizing should not lose the table state")
	}
}

// ---- the action clock ------------------------------------------------

func clockView(remaining, length time.Duration) game.PlayerView {
	view := seatedView(40)
	view.Acting = 1 // the other seat is on the clock
	view.Deadline = time.Now().Add(remaining)
	view.TurnLength = length
	return view
}

// The bar is drawn against whoever is on the clock, so every player can
// see how long the one they are waiting on has left.
func TestClockBarIsShownForTheActingSeat(t *testing.T) {
	m, _ := atMenu()
	m = send(m, table.StateMsg{View: clockView(30*time.Second, 30*time.Second)})
	m.screen = screenTable
	m = resize(m, 100, 30)

	out := m.View()
	if !strings.Contains(out, "█") {
		t.Errorf("expected a clock bar for the acting seat:\n%s", out)
	}
	if !strings.Contains(out, "30s") {
		t.Errorf("expected the seconds remaining:\n%s", out)
	}

	// It belongs to that seat's row, not to the table at large.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "█") && !strings.Contains(line, "Bob") {
			t.Errorf("the bar should sit on the acting seat's row, found it on:\n%s", line)
		}
	}
}

func TestClockBarDrainsAsTimeRuns(t *testing.T) {
	full, _ := atMenu()
	full = resize(full, 100, 30)
	full = send(full, table.StateMsg{View: clockView(30*time.Second, 30*time.Second)})
	full.screen = screenTable

	nearlyOut, _ := atMenu()
	nearlyOut = resize(nearlyOut, 100, 30)
	nearlyOut = send(nearlyOut, table.StateMsg{View: clockView(3*time.Second, 30*time.Second)})
	nearlyOut.screen = screenTable

	filled := func(m Model) int { return strings.Count(m.View(), "█") }

	if filled(nearlyOut) >= filled(full) {
		t.Errorf("the bar should drain: %d blocks at 30s, %d at 3s",
			filled(full), filled(nearlyOut))
	}
	if filled(nearlyOut) == 0 {
		t.Error("a clock with time left should never render as empty")
	}
}

func TestNoClockBarWhenNobodyIsOnTheClock(t *testing.T) {
	m, _ := atMenu()
	m = resize(m, 100, 30)

	view := seatedView(0)
	view.Acting = game.SpectatorSeat
	view.Deadline = time.Time{}
	m = send(m, table.StateMsg{View: view})
	m.screen = screenTable

	if strings.Contains(m.View(), "█") {
		t.Errorf("no clock is running, so no bar should be drawn:\n%s", m.View())
	}
}

// On a narrow terminal the bar gives way to the seconds alone rather
// than pushing the seat row off the edge.
func TestClockFallsBackToSecondsWhenNarrow(t *testing.T) {
	m, _ := atMenu()
	m = send(m, table.StateMsg{View: clockView(30*time.Second, 30*time.Second)})
	m.screen = screenTable
	m = resize(m, 44, 24)

	out := m.View()
	if strings.Contains(out, "█") {
		t.Errorf("a narrow terminal has no room for the bar:\n%s", out)
	}
	if !strings.Contains(out, "30s") {
		t.Errorf("the seconds should still be shown:\n%s", out)
	}
}

// Redrawing only matters while something is moving.
func TestTickSpeedsUpOnlyWhileTheClockRuns(t *testing.T) {
	idle, _ := atMenu()
	if got := idle.tickRate(); got != idleTick {
		t.Errorf("an idle table should tick slowly, got %v", got)
	}

	running := send(idle, table.StateMsg{View: clockView(30*time.Second, 30*time.Second)})
	if got := running.tickRate(); got != runningTick {
		t.Errorf("a running clock should tick faster, got %v", got)
	}
}

// ---- names in the seat column ----------------------------------------

func TestSeatRowsStayAlignedWithAwkwardNames(t *testing.T) {
	m, _ := atMenu()
	m = resize(m, 100, 30)

	view := seatedView(40)
	view.Seats[0].Name = "Renée"
	view.Seats[1].Name = "♠♠♠♠♠♠♠♠♠♠♠♠"
	m = send(m, table.StateMsg{View: view})
	m.screen = screenTable

	out := m.View()

	// Both stacks are printed in the same column, so the digits should
	// start at the same offset on both rows.
	var offsets []int
	for _, line := range strings.Split(out, "\n") {
		plain := stripStyles(line)
		for _, stack := range []string{"980", "940"} {
			if i := strings.Index(plain, stack); i >= 0 {
				// Measure in terminal cells, not bytes: the two names
				// have very different byte lengths and identical widths,
				// which is the whole point.
				offsets = append(offsets, lineWidth(plain[:i]))
			}
		}
	}

	if len(offsets) != 2 {
		t.Fatalf("expected both stacks on screen, found %d:\n%s", len(offsets), out)
	}
	if offsets[0] != offsets[1] {
		t.Errorf("seat rows fell out of line: stacks at columns %d and %d:\n%s",
			offsets[0], offsets[1], out)
	}
}

func TestTruncateCountsDisplayWidth(t *testing.T) {
	for _, tc := range []struct {
		in    string
		limit int
		want  string
	}{
		{"Bob", 12, "Bob"},
		{"AVeryLongPlayerName", 12, "AVeryLongPl…"},
		{"Renée", 12, "Renée"},
		{"ééééééééééééééé", 12, "ééééééééééé…"},
		{"anything", 0, ""},
	} {
		if got := truncate(tc.in, tc.limit); got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.limit, got, tc.want)
		}
		if w := lineWidth(truncate(tc.in, tc.limit)); w > tc.limit {
			t.Errorf("truncate(%q, %d) is %d cells wide", tc.in, tc.limit, w)
		}
	}
}

func TestPadCountsDisplayWidth(t *testing.T) {
	for _, in := range []string{"Bob", "Renée", "♠♠", ""} {
		if got := lineWidth(pad(in, 12)); got != 12 {
			t.Errorf("pad(%q) came out %d cells wide, want 12", in, got)
		}
	}
}
