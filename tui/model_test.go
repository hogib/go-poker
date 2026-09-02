package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"ssh_holdem/deck"
	"ssh_holdem/game"
	"ssh_holdem/table"
)

// fakeController records what the model asked the table to do, which is
// the whole observable effect of a keypress.
type fakeController struct {
	acts   []game.Decision
	sits   int
	stands int

	acceptAct bool
}

func newController() *fakeController { return &fakeController{acceptAct: true} }

func (c *fakeController) Act(_ string, d game.Decision) bool {
	c.acts = append(c.acts, d)
	return c.acceptAct
}
func (c *fakeController) Sit(string) bool   { c.sits++; return true }
func (c *fakeController) Stand(string) bool { c.stands++; return true }

func (c *fakeController) lastAct(t *testing.T) game.Decision {
	t.Helper()
	if len(c.acts) == 0 {
		t.Fatal("no action was sent to the table")
	}
	return c.acts[len(c.acts)-1]
}

// newModel builds a model with a fake table behind it.
func newModel() (Model, *fakeController) {
	c := newController()
	return New(c, "session-1", "Alice", NewStyles(nil)), c
}

// press feeds one keypress and returns the model that came back.
func press(m Model, key string) Model {
	var msg tea.KeyMsg
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "backspace":
		msg = tea.KeyMsg{Type: tea.KeyBackspace}
	case " ":
		msg = tea.KeyMsg{Type: tea.KeySpace}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}

	next, _ := m.Update(msg)
	return next.(Model)
}

func send(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func lobby(seated, watching int, youAreSeated bool) table.LobbyMsg {
	return table.LobbyMsg{
		Rules: table.Config{
			SmallBlind:  10,
			BigBlind:    20,
			BuyIn:       2000,
			TurnTimeout: 30 * time.Second,
			HandDelay:   4 * time.Second,
		},
		Seated:       seated,
		Watching:     watching,
		YouAreSeated: youAreSeated,
		YourChips:    1850,
	}
}

// seatedView builds a snapshot with this player at seat 0 and on the
// clock facing a bet.
func seatedView(toCall int) game.PlayerView {
	return game.PlayerView{
		Seat: 0,
		Hole: []deck.Card{
			deck.NewCard(deck.Ace, deck.Spades),
			deck.NewCard(deck.King, deck.Hearts),
		},
		Board: []deck.Card{
			deck.NewCard(deck.Two, deck.Clubs),
			deck.NewCard(deck.Seven, deck.Diamonds),
			deck.NewCard(deck.Ten, deck.Spades),
		},
		Seats: []game.SeatInfo{
			{Index: 0, Name: "Alice", Chips: 980, CurrentBet: 20, IsButton: true},
			{Index: 1, Name: "Bob", Chips: 940, CurrentBet: 60},
		},
		Street:     game.Flop,
		Acting:     0,
		Pot:        120,
		CurrentBet: 60,
		ToCall:     toCall,
		MinRaiseTo: 100,
		MaxRaiseTo: 1000,
	}
}

// onClock puts the model in the state a player is in when it is their
// turn: seated, at the table, with a live view.
func onClock(t *testing.T, toCall int) (Model, *fakeController) {
	t.Helper()

	m, c := newModel()
	m = send(m, lobby(2, 0, true))
	m = send(m, table.TurnMsg{View: seatedView(toCall)})

	if m.screen != screenTable {
		t.Fatalf("being put on the clock should show the table, got screen %v", m.screen)
	}
	return m, c
}

// ---- lobby ----------------------------------------------------------

func TestStartsOnTheMenu(t *testing.T) {
	m, _ := newModel()

	if m.screen != screenMenu {
		t.Errorf("a new session should land on the menu, got %v", m.screen)
	}
	if !strings.Contains(m.View(), "Take a seat") {
		t.Errorf("the menu should offer a seat:\n%s", m.View())
	}
}

func TestMenuShowsTheHouseRules(t *testing.T) {
	m, _ := newModel()
	m = send(m, lobby(3, 2, false))

	out := m.View()
	for _, want := range []string{"10/20", "2000 buy-in", "3 seated", "2 players watching"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected the menu to show %q:\n%s", want, out)
		}
	}
}

func TestTakeASeatAsksTheTableAndOpensTheFelt(t *testing.T) {
	m, c := newModel()
	m = send(m, lobby(1, 1, false))

	m = press(m, "enter") // "Take a seat" is the first item

	if c.sits != 1 {
		t.Errorf("expected one seat request, got %d", c.sits)
	}
	if m.screen != screenTable {
		t.Errorf("taking a seat should open the table, got screen %v", m.screen)
	}
}

func TestAlreadySeatedGoesStraightToTheFelt(t *testing.T) {
	m, c := newModel()
	m = send(m, lobby(2, 0, true))

	if got := m.menu()[0].label(m); got != "Back to the table" {
		t.Errorf("a seated player should be offered the table, got %q", got)
	}

	m = press(m, "enter")

	if c.sits != 0 {
		t.Error("a seated player should not ask for another seat")
	}
	if m.screen != screenTable {
		t.Errorf("expected the table, got screen %v", m.screen)
	}
}

func TestLeaveYourSeatIsOnlyOfferedWhenSeated(t *testing.T) {
	m, c := newModel()
	m = send(m, lobby(1, 1, false))

	stand := m.menu()[1]
	if stand.id != "stand" {
		t.Fatalf("expected the stand item second, got %q", stand.id)
	}
	if stand.enabled(m) {
		t.Error("a player with no seat should not be offered to leave one")
	}

	// The cursor must skip it rather than resting somewhere enter is a
	// no-op.
	m = press(m, "down")
	if m.menu()[m.cursor].id == "stand" {
		t.Error("the cursor should skip the disabled item")
	}

	m = send(m, lobby(2, 0, true))
	m.cursor = 1
	m = press(m, "enter")

	if c.stands != 1 {
		t.Errorf("expected one request to stand, got %d", c.stands)
	}
}

func TestFullTableDisablesTakingASeat(t *testing.T) {
	m, _ := newModel()
	m = send(m, lobby(maxSeats, 3, false))

	seat := m.menu()[0]
	if seat.enabled(m) {
		t.Error("a full table should not offer a seat")
	}
	if got := seat.label(m); got != "Table full" {
		t.Errorf("expected the item to say the table is full, got %q", got)
	}
}

func TestMenuNavigationWraps(t *testing.T) {
	m, _ := newModel()
	m = send(m, lobby(2, 0, true)) // everything enabled

	items := m.menu()
	if m.cursor != 0 {
		t.Fatalf("expected to start at the top, got %d", m.cursor)
	}

	m = press(m, "up")
	if m.cursor != len(items)-1 {
		t.Errorf("up from the top should wrap to the bottom, got %d", m.cursor)
	}

	m = press(m, "down")
	if m.cursor != 0 {
		t.Errorf("down from the bottom should wrap to the top, got %d", m.cursor)
	}
}

func TestRulesAndHelpScreensReturnToTheMenu(t *testing.T) {
	for _, tc := range []struct {
		item   string
		screen screen
		want   string
	}{
		{"rules", screenRules, "Shot clock"},
		{"help", screenHelp, "How to play"},
	} {
		m, _ := newModel()
		m = send(m, lobby(2, 0, true))

		for i, item := range m.menu() {
			if item.id == tc.item {
				m.cursor = i
			}
		}
		m = press(m, "enter")

		if m.screen != tc.screen {
			t.Fatalf("%s: expected its own screen, got %v", tc.item, m.screen)
		}

		m = press(m, "esc")
		if m.screen != screenMenu {
			t.Errorf("%s: esc should return to the menu, got %v", tc.item, m.screen)
		}
	}
}

func TestRulesScreenShowsTheConfiguredValues(t *testing.T) {
	m, _ := newModel()
	m = send(m, lobby(2, 0, true))
	m.screen = screenRules

	out := m.View()
	for _, want := range []string{"10 / 20", "2000", "30 seconds", "4 seconds"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected the rules screen to show %q:\n%s", want, out)
		}
	}
}

// ---- the felt -------------------------------------------------------

func TestFoldSendsAFold(t *testing.T) {
	m, c := onClock(t, 40)
	m = press(m, "f")

	if got := c.lastAct(t); got.Action != game.Fold {
		t.Errorf("expected a fold, got %+v", got)
	}
	if m.onClock {
		t.Error("acting should take the player off the clock")
	}
}

func TestCallAndCheckFollowTheAmountOwed(t *testing.T) {
	m, c := onClock(t, 40)
	press(m, "c")
	if got := c.lastAct(t); got.Action != game.Call {
		t.Errorf("with 40 to call, c should call, got %+v", got)
	}

	m, c = onClock(t, 0)
	press(m, "c")
	if got := c.lastAct(t); got.Action != game.Check {
		t.Errorf("with nothing to call, c should check, got %+v", got)
	}
}

func TestAllInShovesTheMaximum(t *testing.T) {
	m, c := onClock(t, 40)
	press(m, "a")

	got := c.lastAct(t)
	if got.Action != game.Raise || got.Amount != 1000 {
		t.Errorf("expected a shove to 1000, got %+v", got)
	}
}

func TestRaisePromptAcceptsATypedAmount(t *testing.T) {
	m, c := onClock(t, 40)

	m = press(m, "r")
	if m.raising == nil {
		t.Fatal("r should open the raise prompt")
	}
	if !strings.Contains(m.View(), "Raise to") {
		t.Errorf("the prompt should be visible:\n%s", m.View())
	}

	for _, key := range []string{"2", "5", "0"} {
		m = press(m, key)
	}
	if m.raising.digits != "250" {
		t.Fatalf("expected the typed digits, got %q", m.raising.digits)
	}

	m = press(m, "backspace")
	if m.raising.digits != "25" {
		t.Errorf("backspace should remove a digit, got %q", m.raising.digits)
	}

	m = press(m, "5")
	m = press(m, "enter")

	got := c.lastAct(t)
	if got.Action != game.Raise || got.Amount != 255 {
		t.Errorf("expected a raise to 255, got %+v", got)
	}
	if m.raising != nil {
		t.Error("confirming should close the prompt")
	}
}

func TestRaiseAmountIsClampedToWhatIsLegal(t *testing.T) {
	for _, tc := range []struct {
		typed string
		want  int
	}{
		{"1", 100},       // below the minimum raise
		{"999999", 1000}, // beyond the stack
		{"", 1000},       // no amount typed means shove
	} {
		m, c := onClock(t, 40)
		m = press(m, "r")
		for _, r := range tc.typed {
			m = press(m, string(r))
		}
		press(m, "enter")

		if got := c.lastAct(t); got.Amount != tc.want {
			t.Errorf("typing %q should raise to %d, got %d", tc.typed, tc.want, got.Amount)
		}
	}
}

func TestRaiseCanBeCancelled(t *testing.T) {
	m, c := onClock(t, 40)

	m = press(m, "r")
	m = press(m, "5")
	m = press(m, "esc")

	if m.raising != nil {
		t.Error("esc should close the raise prompt")
	}
	if len(c.acts) != 0 {
		t.Errorf("cancelling should send nothing, got %v", c.acts)
	}
	if m.screen != screenTable {
		t.Error("cancelling a raise should not leave the table")
	}
}

// A keypress that arrives after the table has moved on must not act.
func TestKeysDoNothingWhenNotOnTheClock(t *testing.T) {
	m, c := onClock(t, 40)

	// The table moves on to another seat.
	view := seatedView(40)
	view.Acting = 1
	m = send(m, table.StateMsg{View: view})

	if m.onClock {
		t.Fatal("the snapshot says another seat is acting")
	}

	m = press(m, "f")
	m = press(m, "c")
	m = press(m, "a")

	if len(c.acts) != 0 {
		t.Errorf("expected no actions off the clock, got %v", c.acts)
	}
	if !strings.Contains(m.status, "not your turn") {
		t.Errorf("the player should be told why nothing happened, got %q", m.status)
	}
}

// Acting twice on one turn would be a double-fold. The second press must
// not reach the table.
func TestASecondPressOnTheSameTurnIsIgnored(t *testing.T) {
	m, c := onClock(t, 40)

	m = press(m, "f")
	m = press(m, "f")

	if len(c.acts) != 1 {
		t.Errorf("expected exactly one action for one turn, got %v", c.acts)
	}
}

func TestRejectedActionIsReportedAndDoesNotClearTheClock(t *testing.T) {
	m, c := onClock(t, 40)
	c.acceptAct = false

	m = press(m, "f")

	if !m.onClock {
		t.Error("a rejected action should leave the player on the clock")
	}
	if !strings.Contains(m.status, "not accepted") {
		t.Errorf("the player should be told the action bounced, got %q", m.status)
	}
}

func TestEscReturnsToTheMenuFromTheFelt(t *testing.T) {
	m, _ := onClock(t, 40)
	m = press(m, "esc")

	if m.screen != screenMenu {
		t.Errorf("esc should open the menu, got %v", m.screen)
	}
}

// Being put on the clock while reading the rules pulls you back, so
// nobody times out on a help screen.
func TestBeingPutOnTheClockInterruptsAnyScreen(t *testing.T) {
	for _, from := range []screen{screenMenu, screenRules, screenHelp} {
		m, _ := newModel()
		m.screen = from

		m = send(m, table.TurnMsg{View: seatedView(20)})

		if m.screen != screenTable {
			t.Errorf("from screen %v, a turn should open the table, got %v", from, m.screen)
		}
		if !m.onClock {
			t.Errorf("from screen %v, the player should be on the clock", from)
		}
	}
}

func TestRebuyIsOfferedOnlyToABustedPlayer(t *testing.T) {
	// Still has chips: r must not buy in.
	m, c := onClock(t, 40)
	view := seatedView(40)
	view.Acting = 1
	m = send(m, table.StateMsg{View: view})

	m = press(m, "r")
	if c.sits != 0 {
		t.Error("a player with chips should not be able to buy in again")
	}
	if strings.Contains(m.View(), "buy in") {
		t.Errorf("the buy-in hint should be hidden:\n%s", m.View())
	}

	// Busted: r buys in.
	view.Seats[0].Chips = 0
	m = send(m, table.StateMsg{View: view})

	m = press(m, "r")
	if c.sits != 1 {
		t.Errorf("a busted player should be able to buy in, got %d requests", c.sits)
	}
	if !strings.Contains(m.View(), "buy in") {
		t.Errorf("the buy-in hint should be offered:\n%s", m.View())
	}
}

// ---- rendering ------------------------------------------------------

func TestFeltShowsOnlyYourOwnHoleCards(t *testing.T) {
	m, _ := onClock(t, 40)
	out := m.View()

	if !strings.Contains(out, "A♠") || !strings.Contains(out, "K♥") {
		t.Errorf("your own cards should be shown:\n%s", out)
	}
	if !strings.Contains(out, "Bob") {
		t.Errorf("the other seat should be listed:\n%s", out)
	}
	if !strings.Contains(out, "pot 120") {
		t.Errorf("the pot should be shown:\n%s", out)
	}
}

func TestSpectatorSeesNoHoleCards(t *testing.T) {
	m, _ := newModel()
	m = send(m, lobby(2, 1, false))

	view := seatedView(0)
	view.Seat = game.SpectatorSeat
	view.Hole = nil
	view.Acting = 1
	m = send(m, table.StateMsg{View: view})
	m.screen = screenTable

	out := m.View()
	if strings.Contains(out, "A♠") {
		t.Errorf("a spectator must not be shown hole cards:\n%s", out)
	}
	if !strings.Contains(out, "Bob") {
		t.Errorf("a spectator should still see the table:\n%s", out)
	}
}

func TestClockCountsDownAndTurnsUrgent(t *testing.T) {
	m, _ := newModel()
	m = send(m, lobby(2, 0, true))

	view := seatedView(20)
	view.Deadline = time.Now().Add(20 * time.Second)
	m = send(m, table.TurnMsg{View: view})

	if !strings.Contains(m.View(), "20s") {
		t.Errorf("expected the clock to show twenty seconds:\n%s", m.View())
	}

	view.Deadline = time.Now().Add(3 * time.Second)
	m = send(m, table.TurnMsg{View: view})

	if !strings.Contains(m.View(), "3s") {
		t.Errorf("expected the clock to show three seconds:\n%s", m.View())
	}
}

func TestResultIsWrittenToTheLog(t *testing.T) {
	m, _ := onClock(t, 40)

	m = send(m, table.ResultMsg{
		Result: game.HandResult{
			Pots: []game.PotResult{{
				Amount:  240,
				Winners: []int{1},
				Best: deck.Hand{Cards: []deck.Card{
					deck.NewCard(deck.Ace, deck.Spades),
					deck.NewCard(deck.Ace, deck.Hearts),
					deck.NewCard(deck.King, deck.Clubs),
					deck.NewCard(deck.Seven, deck.Diamonds),
					deck.NewCard(deck.Two, deck.Spades),
				}},
			}},
		},
		Seats: seatedView(0).Seats,
	})

	out := m.View()
	if !strings.Contains(out, "Bob wins 240") {
		t.Errorf("the result should be logged:\n%s", out)
	}
	if m.onClock {
		t.Error("a finished hand takes everyone off the clock")
	}
}

func TestUncontestedResultDoesNotClaimAWinningHand(t *testing.T) {
	m, _ := onClock(t, 40)

	m = send(m, table.ResultMsg{
		Result: game.HandResult{
			Uncontested: true,
			Pots:        []game.PotResult{{Amount: 30, Winners: []int{0}}},
		},
		Seats: seatedView(0).Seats,
	})

	out := m.View()
	if !strings.Contains(out, "Alice takes 30") {
		t.Errorf("an uncontested pot should just be taken:\n%s", out)
	}
	if strings.Contains(out, "wins 30 with") {
		t.Errorf("no hand was shown, so none should be named:\n%s", out)
	}
}

func TestLogKeepsOnlyTheRecentLines(t *testing.T) {
	m, _ := newModel()

	for i := 0; i < logLines+5; i++ {
		m = send(m, table.InfoMsg{Text: strings.Repeat("x", i+1)})
	}

	if len(m.log) != logLines {
		t.Errorf("expected the log to hold %d lines, got %d", logLines, len(m.log))
	}
	if m.log[len(m.log)-1] != strings.Repeat("x", logLines+5) {
		t.Error("the newest line should survive")
	}
}

func TestViewNeverPanicsBeforeAnyStateArrives(t *testing.T) {
	m, _ := newModel()

	for _, s := range []screen{screenMenu, screenTable, screenRules, screenHelp} {
		m.screen = s
		if out := m.View(); out == "" {
			t.Errorf("screen %v rendered nothing", s)
		}
	}
}

func TestQuitFromEitherScreen(t *testing.T) {
	m, _ := newModel()
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd == nil {
		t.Error("q should quit from the menu")
	}

	m, _ = onClock(t, 40)
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd == nil {
		t.Error("ctrl+c should quit from the table")
	}
}

func TestTruncateKeepsLongNamesInTheirColumn(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Bob", "Bob"},
		{"AVeryLongPlayerName", "AVeryLongPl…"},
	} {
		if got := truncate(tc.in, 12); got != tc.want {
			t.Errorf("truncate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	for _, tc := range []struct {
		in   time.Duration
		want string
	}{
		{0, "none"},
		{30 * time.Second, "30 seconds"},
		{90 * time.Second, "1m30s"},
	} {
		if got := humanDuration(tc.in); got != tc.want {
			t.Errorf("humanDuration(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The model must satisfy bubbletea's interface, which the server relies
// on but nothing else here would catch.
var _ tea.Model = Model{}

// And *table.Table must satisfy the Controller the model drives.
var _ Controller = (*table.Table)(nil)
