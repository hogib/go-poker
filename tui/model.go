// Package tui renders a poker table for one player over their SSH
// session. It holds no game state of its own: every frame is drawn from
// the last redacted snapshot the table pushed.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go_poker/deck"
	"go_poker/game"
	"go_poker/table"
)

// tickMsg drives the shot-clock countdown.
type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Model is one player's view of the table.
type Model struct {
	table   *table.Table
	session *table.Session
	styles  Styles

	width  int
	height int

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

// New builds the model for a session. The table is not touched here;
// everything arrives as a message.
func New(t *table.Table, s *table.Session, styles Styles) Model {
	return Model{
		table:   t,
		session: s,
		styles:  styles,
		width:   80,
		height:  24,
		status:  "Waiting for the table...",
	}
}

func (m Model) Init() tea.Cmd {
	return tick()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		return m, tick()

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
		m.status = "Your move."
		return m, nil

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
	if m.raising != nil {
		return m.handleRaiseKey(msg)
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "r":
		// Doubles as buy-in when this player has been knocked out.
		if !m.onClock {
			if m.table.Rebuy(m.session.ID) {
				m.status = "Sitting in for the next hand."
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

	case "f":
		return m.act(game.Decision{Action: game.Fold})

	case "c":
		if m.view.ToCall > 0 {
			return m.act(game.Decision{Action: game.Call})
		}
		return m.act(game.Decision{Action: game.Check})

	case "k", " ", "enter":
		if m.view.ToCall == 0 {
			return m.act(game.Decision{Action: game.Check})
		}
		return m.act(game.Decision{Action: game.Call})

	case "a":
		if m.onClock && m.view.Legal(game.Raise) {
			return m.act(game.Decision{Action: game.Raise, Amount: m.view.MaxRaiseTo})
		}
	}

	return m, nil
}

func (m Model) handleRaiseKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.raising = nil
		m.status = "Raise cancelled."
		return m, nil

	case "backspace":
		if n := len(m.raising.digits); n > 0 {
			m.raising.digits = m.raising.digits[:n-1]
		}
		return m, nil

	case "enter":
		amount := m.raising.max
		if m.raising.digits != "" {
			fmt.Sscanf(m.raising.digits, "%d", &amount)
		}
		if amount < m.raising.min {
			amount = m.raising.min
		}
		if amount > m.raising.max {
			amount = m.raising.max
		}
		m.raising = nil
		return m.act(game.Decision{Action: game.Raise, Amount: amount})

	case "ctrl+c":
		return m, tea.Quit
	}

	if len(msg.String()) == 1 && msg.String()[0] >= '0' && msg.String()[0] <= '9' {
		if len(m.raising.digits) < 9 {
			m.raising.digits += msg.String()
		}
	}
	return m, nil
}

func (m Model) act(d game.Decision) (tea.Model, tea.Cmd) {
	if !m.onClock {
		m.status = "It is not your turn."
		return m, nil
	}

	if !m.table.Act(m.session.ID, d) {
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
	if len(m.log) > 6 {
		m.log = m.log[len(m.log)-6:]
	}
}

func describeResult(r game.HandResult, seats []game.SeatInfo) string {
	name := func(seat int) string {
		if seat >= 0 && seat < len(seats) {
			return seats[seat].Name
		}
		return fmt.Sprintf("seat %d", seat)
	}

	parts := make([]string, 0, len(r.Pots))
	for _, pot := range r.Pots {
		winners := make([]string, 0, len(pot.Winners))
		for _, seat := range pot.Winners {
			winners = append(winners, name(seat))
		}

		who := strings.Join(winners, " and ")
		if r.Uncontested {
			parts = append(parts, fmt.Sprintf("%s takes %d", who, pot.Amount))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s wins %d with %s", who, pot.Amount, pot.Best))
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ")
}

func cardsString(cards []deck.Card) string {
	if len(cards) == 0 {
		return "--"
	}
	parts := make([]string, 0, len(cards))
	for _, c := range cards {
		parts = append(parts, c.String())
	}
	return strings.Join(parts, " ")
}
