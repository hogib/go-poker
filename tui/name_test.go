package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"ssh_holdem/table"
)

func typeName(m Model, text string) Model {
	for _, r := range text {
		m = press(m, string(r))
	}
	return m
}

func TestTypingAName(t *testing.T) {
	m, c := newModel()

	m = press(m, "ctrl+u") // clear the prefill
	if m.naming.typed != "" {
		t.Fatalf("ctrl+u should clear the field, got %q", m.naming.typed)
	}

	m = typeName(m, "Ace")
	if m.naming.typed != "Ace" {
		t.Fatalf("expected the typed name, got %q", m.naming.typed)
	}
	if !strings.Contains(m.View(), "Ace") {
		t.Errorf("the field should show what was typed:\n%s", m.View())
	}

	m = press(m, "backspace")
	if m.naming.typed != "Ac" {
		t.Errorf("backspace should remove a character, got %q", m.naming.typed)
	}

	m = press(m, "enter")
	if len(c.renames) == 0 || c.renames[len(c.renames)-1] != "Ac" {
		t.Errorf("expected the typed name to be sent, got %v", c.renames)
	}
	if m.name != "Ac" {
		t.Errorf("the model should adopt the confirmed name, got %q", m.name)
	}
}

// q is a letter, not a command. People are called Quinn.
func TestQIsALetterOnTheNameScreen(t *testing.T) {
	m, _ := newModel()
	m = press(m, "ctrl+u")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = next.(Model)

	if cmd != nil {
		t.Error("q should not quit while typing a name")
	}
	if m.naming.typed != "q" {
		t.Errorf("q should be typed into the field, got %q", m.naming.typed)
	}
}

func TestSpaceIsAllowedInAName(t *testing.T) {
	m, _ := newModel()
	m = press(m, "ctrl+u")
	m = typeName(m, "Big")
	m = press(m, " ")
	m = typeName(m, "Al")

	if m.naming.typed != "Big Al" {
		t.Errorf("expected a space to be typed, got %q", m.naming.typed)
	}
}

// The field stops at the table's limit rather than letting a player type
// something that will be silently cut.
func TestNameFieldStopsAtTheLimit(t *testing.T) {
	m, _ := newModel()
	m = press(m, "ctrl+u")
	m = typeName(m, strings.Repeat("x", table.MaxNameLength+8))

	if got := len([]rune(m.naming.typed)); got != table.MaxNameLength {
		t.Errorf("expected the field to stop at %d, got %d", table.MaxNameLength, got)
	}
}

// The table has the final say, and its reason is shown rather than
// swallowed.
func TestARefusedNameIsReportedAndKeepsYouOnTheScreen(t *testing.T) {
	m, c := newModel()
	c.renameErr = errors.New(`"Alice" is already taken`)

	m = press(m, "enter")

	if m.screen != screenName {
		t.Errorf("a refused name should leave you on the name screen, got %v", m.screen)
	}
	if !strings.Contains(m.View(), "already taken") {
		t.Errorf("the reason should be shown:\n%s", m.View())
	}
	if m.name != "Alice" {
		t.Errorf("a refused rename should not change the model's name, got %q", m.name)
	}
}

// Editing after a refusal clears the complaint, so it does not sit there
// contradicting what is on screen.
func TestEditingClearsTheError(t *testing.T) {
	m, c := newModel()
	c.renameErr = errors.New("taken")

	m = press(m, "enter")
	if m.naming.err == "" {
		t.Fatal("expected an error to be shown")
	}

	m = press(m, "backspace")
	if m.naming.err != "" {
		t.Errorf("editing should clear the error, got %q", m.naming.err)
	}
}

// The table may clean up what was typed; the model must take back what
// it actually got, not what it sent.
func TestTheCleanedNameIsWhatSticks(t *testing.T) {
	m, c := newModel()
	c.renameFrom = func(string) string { return "Trimmed" }

	m = press(m, "ctrl+u")
	m = typeName(m, "  messy  ")
	m = press(m, "enter")

	if m.name != "Trimmed" {
		t.Errorf("expected the name the table returned, got %q", m.name)
	}
	if !strings.Contains(m.View(), "Trimmed") {
		t.Errorf("the menu should show the cleaned name:\n%s", m.View())
	}
}

func TestEscKeepsTheCurrentName(t *testing.T) {
	m, c := newModel()

	m = press(m, "ctrl+u")
	m = typeName(m, "Discarded")
	m = press(m, "esc")

	if m.screen != screenMenu {
		t.Errorf("esc should back out to the menu, got %v", m.screen)
	}
	if len(c.renames) != 0 {
		t.Errorf("backing out should send nothing, got %v", c.renames)
	}
	if m.name != "Alice" {
		t.Errorf("the existing name should be kept, got %q", m.name)
	}
}

func TestChangeYourNameFromTheMenu(t *testing.T) {
	m, _ := atMenu()

	for i, item := range m.menu() {
		if item.id == "name" {
			m.cursor = i
		}
	}
	m = press(m, "enter")

	if m.screen != screenName {
		t.Errorf("the menu should be able to reopen the name screen, got %v", m.screen)
	}
	if m.naming.typed != "Alice" {
		t.Errorf("the field should be prefilled with the current name, got %q", m.naming.typed)
	}
}

func TestMenuShowsWhoYouArePlayingAs(t *testing.T) {
	m, _ := atMenu()
	m = send(m, lobby(1, 1, false))

	if !strings.Contains(m.View(), "playing as Alice") {
		t.Errorf("the menu should say who you are:\n%s", m.View())
	}
}
