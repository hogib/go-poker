package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"ssh_holdem/game"
	"ssh_holdem/table"
)

// Controller is the slice of the table this model drives. Keeping it an
// interface is what lets the whole UI be exercised without a terminal or
// a running table.
type Controller interface {
	Act(sessionID string, d game.Decision) bool
	Sit(sessionID string) bool
	Stand(sessionID string) bool
	Rename(sessionID, name string) (string, error)
}

type screen int

const (
	screenName screen = iota
	screenMenu
	screenTable
	screenRules
	screenHelp
)

// menuItem is one line on the lobby menu. Items that do not apply right
// now are shown greyed rather than hidden, so the menu never reflows
// under the cursor.
type menuItem struct {
	id      string
	label   func(m Model) string
	enabled func(m Model) bool
	action  func(m Model) (Model, tea.Cmd)
}

// tickMsg drives the shot-clock countdown.
type tickMsg time.Time

// The clock bar needs redrawing several times a second to move at all,
// but there is nothing to animate when no one is on the clock.
const (
	idleTick    = time.Second
	runningTick = 200 * time.Millisecond
)

func tick(every time.Duration) tea.Cmd {
	return tea.Tick(every, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Model is one player's view of the lobby and the table.
type Model struct {
	ctrl      Controller
	sessionID string
	name      string
	styles    Styles

	width  int
	height int

	screen screen
	cursor int

	// listView forces the compact seat list even where the oval would
	// fit. It is off by default: the oval is what makes whose turn it is
	// obvious at a glance.
	listView bool

	// naming holds the name being typed. The screen is shown first, and
	// again whenever the player picks "Change name".
	naming nameInput

	lobby    table.LobbyMsg
	hasLobby bool

	view    game.PlayerView
	hasView bool

	// onClock is set when this player is the one being waited on, and
	// cleared as soon as they act, so a stale keypress cannot act twice.
	onClock bool

	// raising holds a raise amount being typed. Nil when not raising.
	raising *raiseInput

	result *game.HandResult
	log    []string
	status string
}

type raiseInput struct {
	digits string
	min    int
	max    int
}

type nameInput struct {
	typed string
	err   string
}

// New builds the model for a session. Nothing is read from the table
// here; every piece of state arrives as a message.
func New(ctrl Controller, sessionID, name string, styles Styles) Model {
	return Model{
		ctrl:      ctrl,
		sessionID: sessionID,
		name:      name,
		styles:    styles,
		width:     80,
		height:    24,
		screen:    screenName,
		naming:    nameInput{typed: name},
		status:    "",
	}
}

func (m Model) Init() tea.Cmd { return tick(idleTick) }

// tickRate slows right down when there is no clock to animate.
func (m Model) tickRate() time.Duration {
	if m.hasView && m.view.Acting != game.SpectatorSeat && !m.view.Deadline.IsZero() {
		return runningTick
	}
	return idleTick
}

// menu is the lobby's items in order.
func (m Model) menu() []menuItem {
	return []menuItem{
		{
			id: "seat",
			label: func(m Model) string {
				if m.lobby.YouAreSeated {
					return "Back to the table"
				}
				if m.hasLobby && m.lobby.Seated >= maxSeats {
					return "Table full"
				}
				return "Take a seat"
			},
			enabled: func(m Model) bool {
				return m.lobby.YouAreSeated || m.lobby.Seated < maxSeats
			},
			action: func(m Model) (Model, tea.Cmd) {
				if !m.lobby.YouAreSeated {
					m.ctrl.Sit(m.sessionID)
					m.status = "Sitting in for the next hand."
				}
				m.screen = screenTable
				return m, nil
			},
		},
		{
			id:      "stand",
			label:   func(m Model) string { return "Leave your seat" },
			enabled: func(m Model) bool { return m.lobby.YouAreSeated },
			action: func(m Model) (Model, tea.Cmd) {
				m.ctrl.Stand(m.sessionID)
				m.status = "You will be up after this hand."
				return m, nil
			},
		},
		{
			id:      "watch",
			label:   func(m Model) string { return "Watch the table" },
			enabled: func(m Model) bool { return true },
			action: func(m Model) (Model, tea.Cmd) {
				m.screen = screenTable
				return m, nil
			},
		},
		{
			id:      "name",
			label:   func(m Model) string { return "Change your name" },
			enabled: func(m Model) bool { return true },
			action: func(m Model) (Model, tea.Cmd) {
				m.naming = nameInput{typed: m.name}
				m.screen = screenName
				return m, nil
			},
		},
		{
			id:      "rules",
			label:   func(m Model) string { return "Table rules" },
			enabled: func(m Model) bool { return true },
			action: func(m Model) (Model, tea.Cmd) {
				m.screen = screenRules
				return m, nil
			},
		},
		{
			id:      "help",
			label:   func(m Model) string { return "How to play" },
			enabled: func(m Model) bool { return true },
			action: func(m Model) (Model, tea.Cmd) {
				m.screen = screenHelp
				return m, nil
			},
		},
		{
			id:      "quit",
			label:   func(m Model) string { return "Quit" },
			enabled: func(m Model) bool { return true },
			action:  func(m Model) (Model, tea.Cmd) { return m, tea.Quit },
		},
	}
}

// maxSeats mirrors the engine's table limit.
const maxSeats = 9

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		return m, tick(m.tickRate())

	case table.LobbyMsg:
		m.lobby = msg
		m.hasLobby = true
		return m, nil

	case table.StateMsg:
		m.view = msg.View
		m.hasView = true
		// The table has moved on: this player is only on the clock while
		// the snapshot says so.
		if m.view.Acting != m.view.Seat || m.view.Seat == game.SpectatorSeat {
			m.onClock = false
			m.raising = nil
		}
		return m, nil

	case table.TurnMsg:
		m.view = msg.View
		m.hasView = true
		m.onClock = true
		// Being put on the clock pulls you back to the felt: reading the
		// rules, or picking a new name, is not worth losing a hand over.
		m.screen = screenTable
		return m, tick(runningTick)

	case table.ResultMsg:
		result := msg.Result
		m.result = &result
		m.onClock = false
		m.raising = nil
		m.pushLog(describeResult(result, msg.Seats))
		return m, nil

	case table.InfoMsg:
		m.pushLog(msg.Text)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	switch m.screen {
	case screenName:
		return m.handleNameKey(msg)
	case screenMenu:
		return m.handleMenuKey(msg)
	case screenRules, screenHelp:
		switch msg.String() {
		case "esc", "enter", "q", "backspace", "left":
			m.screen = screenMenu
		}
		return m, nil
	default:
		return m.handleTableKey(msg)
	}
}

// handleNameKey drives the name field. It is the first thing a player
// sees, prefilled with their ssh username, so enter alone is enough.
func (m Model) handleNameKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		// q is a letter here, not a command: people are called Quinn.
		if len(m.naming.typed) < table.MaxNameLength {
			m.naming.typed += "q"
		}
		return m, nil

	case "esc":
		// Backing out keeps the name you already have.
		m.screen = screenMenu
		return m, nil

	case "backspace":
		if runes := []rune(m.naming.typed); len(runes) > 0 {
			m.naming.typed = string(runes[:len(runes)-1])
			m.naming.err = ""
		}
		return m, nil

	case "ctrl+u":
		m.naming.typed = ""
		m.naming.err = ""
		return m, nil

	case "enter":
		name, err := m.ctrl.Rename(m.sessionID, m.naming.typed)
		if err != nil {
			m.naming.err = err.Error()
			return m, nil
		}
		m.name = name
		m.naming = nameInput{typed: name}
		m.screen = screenMenu
		m.status = fmt.Sprintf("Sitting in as %s.", name)
		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		for _, r := range msg.Runes {
			if len([]rune(m.naming.typed)) >= table.MaxNameLength {
				break
			}
			m.naming.typed += string(r)
		}
		m.naming.err = ""
		return m, nil
	}
	if msg.Type == tea.KeySpace && len([]rune(m.naming.typed)) < table.MaxNameLength {
		m.naming.typed += " "
	}

	return m, nil
}

func (m Model) handleMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.menu()

	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "up", "k":
		m.cursor = m.moveCursor(items, -1)
		return m, nil

	case "down", "j", "tab":
		m.cursor = m.moveCursor(items, 1)
		return m, nil

	case "enter", " ", "right", "l":
		if m.cursor < 0 || m.cursor >= len(items) {
			return m, nil
		}
		item := items[m.cursor]
		if !item.enabled(m) {
			m.status = "Not available right now."
			return m, nil
		}
		return item.action(m)
	}

	return m, nil
}

// moveCursor steps to the next selectable item, skipping any that are
// greyed out so the cursor never rests somewhere enter does nothing.
func (m Model) moveCursor(items []menuItem, step int) int {
	n := len(items)
	if n == 0 {
		return 0
	}

	for i := 1; i <= n; i++ {
		next := ((m.cursor+step*i)%n + n) % n
		if items[next].enabled(m) {
			return next
		}
	}
	return m.cursor
}

func (m Model) handleTableKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.raising != nil {
		return m.handleRaiseKey(msg)
	}

	switch msg.String() {
	case "esc", "m":
		m.screen = screenMenu
		m.cursor = 0
		return m, nil

	case "v":
		m.listView = !m.listView
		if m.listView {
			m.status = "Compact view."
		} else {
			m.status = "Table view."
		}
		return m, nil

	case "q":
		return m, tea.Quit

	case "f":
		return m.act(game.Decision{Action: game.Fold})

	case "c", "k", " ", "enter":
		if m.view.ToCall > 0 {
			return m.act(game.Decision{Action: game.Call})
		}
		return m.act(game.Decision{Action: game.Check})

	case "a":
		if m.onClock && m.view.Legal(game.Raise) {
			return m.act(game.Decision{Action: game.Raise, Amount: m.view.MaxRaiseTo})
		}
		return m, nil

	case "r":
		if !m.onClock {
			// Doubles as buy-in for someone who has been knocked out.
			if m.canRebuy() {
				m.ctrl.Sit(m.sessionID)
				m.status = "Buying in for the next hand."
			}
			return m, nil
		}
		if !m.view.Legal(game.Raise) {
			m.status = "You have nothing left to raise with."
			return m, nil
		}
		m.raising = &raiseInput{
			min: min(m.view.MinRaiseTo, m.view.MaxRaiseTo),
			max: m.view.MaxRaiseTo,
		}
		return m, nil
	}

	return m, nil
}

// canRebuy reports whether this player is out of chips and needs to buy
// in again. A busted player keeps their seat until the next hand starts,
// so an empty stack counts as well as being off the table.
func (m Model) canRebuy() bool {
	if !m.hasView {
		return false
	}
	if m.view.Seat == game.SpectatorSeat {
		return true
	}
	return m.view.Seats[m.view.Seat].Chips == 0
}

func (m Model) handleRaiseKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.raising = nil
		m.status = "Raise cancelled."
		return m, nil

	case "backspace":
		if n := len(m.raising.digits); n > 0 {
			trimmed := *m.raising
			trimmed.digits = trimmed.digits[:n-1]
			m.raising = &trimmed
		}
		return m, nil

	case "enter":
		amount := m.raising.max
		if m.raising.digits != "" {
			fmt.Sscanf(m.raising.digits, "%d", &amount)
		}
		amount = max(m.raising.min, min(amount, m.raising.max))
		m.raising = nil
		return m.act(game.Decision{Action: game.Raise, Amount: amount})
	}

	if key := msg.String(); len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		if len(m.raising.digits) < 9 {
			typed := *m.raising
			typed.digits += key
			m.raising = &typed
		}
	}
	return m, nil
}

func (m Model) act(d game.Decision) (tea.Model, tea.Cmd) {
	if !m.onClock {
		m.status = "It is not your turn."
		return m, nil
	}

	if !m.ctrl.Act(m.sessionID, d) {
		m.status = "That action was not accepted."
		return m, nil
	}

	m.onClock = false
	m.status = fmt.Sprintf("You %s.", strings.ToLower(string(d.Action)))
	return m, nil
}

func (m *Model) pushLog(line string) {
	if line == "" {
		return
	}
	m.log = append(m.log, line)
	if len(m.log) > logLines {
		m.log = m.log[len(m.log)-logLines:]
	}
}

const logLines = 5
