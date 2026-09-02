package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"go_poker/game"
)

// Styles are built per session, because the colour profile depends on the
// terminal that connected.
type Styles struct {
	Title    lipgloss.Style
	Board    lipgloss.Style
	Hole     lipgloss.Style
	Seat     lipgloss.Style
	SeatYou  lipgloss.Style
	SeatTurn lipgloss.Style
	Folded   lipgloss.Style
	Pot      lipgloss.Style
	Status   lipgloss.Style
	Log      lipgloss.Style
	Keys     lipgloss.Style
	Clock    lipgloss.Style
}

// NewStyles builds the palette from a renderer bound to one session. Use
// bubbletea.MakeRenderer(sess) so the colours match what that terminal
// can actually display.
func NewStyles(r *lipgloss.Renderer) Styles {
	base := r.NewStyle()

	return Styles{
		Title:    base.Bold(true).Foreground(lipgloss.Color("15")),
		Board:    base.Bold(true).Foreground(lipgloss.Color("10")),
		Hole:     base.Bold(true).Foreground(lipgloss.Color("14")),
		Seat:     base.Foreground(lipgloss.Color("7")),
		SeatYou:  base.Bold(true).Foreground(lipgloss.Color("11")),
		SeatTurn: base.Bold(true).Foreground(lipgloss.Color("13")),
		Folded:   base.Faint(true),
		Pot:      base.Bold(true).Foreground(lipgloss.Color("11")),
		Status:   base.Foreground(lipgloss.Color("12")),
		Log:      base.Faint(true),
		Keys:     base.Faint(true),
		Clock:    base.Foreground(lipgloss.Color("9")),
	}
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(m.styles.Title.Render("go-poker") + "\n\n")

	if !m.hasView {
		b.WriteString(m.styles.Status.Render(m.status) + "\n")
		return b.String()
	}

	b.WriteString(m.renderSeats())
	b.WriteString("\n")
	b.WriteString(m.renderBoard())
	b.WriteString("\n")
	b.WriteString(m.renderHole())
	b.WriteString("\n\n")
	b.WriteString(m.renderStatus())
	b.WriteString("\n")
	b.WriteString(m.renderLog())
	b.WriteString("\n")
	b.WriteString(m.styles.Keys.Render(m.keyHints()))
	b.WriteString("\n")

	return b.String()
}

func (m Model) renderSeats() string {
	var b strings.Builder

	for _, seat := range m.view.Seats {
		marker := "  "
		if seat.IsButton {
			marker = "D "
		}

		line := fmt.Sprintf("%s%-12s %6d", marker, seat.Name, seat.Chips)

		switch {
		case seat.Folded:
			line += "  folded"
		case seat.AllIn:
			line += fmt.Sprintf("  all-in %d", seat.CurrentBet)
		case seat.CurrentBet > 0:
			line += fmt.Sprintf("  bet %d", seat.CurrentBet)
		}

		style := m.styles.Seat
		switch {
		case seat.Folded:
			style = m.styles.Folded
		case seat.Index == m.view.Acting:
			style = m.styles.SeatTurn
			line += "  <-"
		case seat.Index == m.view.Seat:
			style = m.styles.SeatYou
		}

		b.WriteString(style.Render(line) + "\n")
	}

	return b.String()
}

func (m Model) renderBoard() string {
	label := fmt.Sprintf("%-8s", m.view.Street.String())
	board := m.styles.Board.Render(cardsString(m.view.Board))
	pot := m.styles.Pot.Render(fmt.Sprintf("pot %d", m.view.Pot))

	return fmt.Sprintf("%s %s   %s\n", label, board, pot)
}

func (m Model) renderHole() string {
	if m.view.Seat == game.SpectatorSeat {
		return m.styles.Log.Render("watching from the rail\n")
	}
	return fmt.Sprintf("%-8s %s\n", "you", m.styles.Hole.Render(cardsString(m.view.Hole)))
}

func (m Model) renderStatus() string {
	if m.raising != nil {
		typed := m.raising.digits
		if typed == "" {
			typed = "_"
		}
		return m.styles.Status.Render(fmt.Sprintf(
			"Raise to %s   (%d-%d, enter to confirm, esc to cancel)",
			typed, m.raising.min, m.raising.max))
	}

	status := m.status
	if m.onClock {
		status = fmt.Sprintf("Your move. %d to call.", m.view.ToCall)
		if remaining := time.Until(m.view.Deadline); !m.view.Deadline.IsZero() && remaining > 0 {
			status += m.styles.Clock.Render(fmt.Sprintf("  %ds", int(remaining.Seconds())))
		}
	}

	return m.styles.Status.Render(status)
}

func (m Model) renderLog() string {
	if len(m.log) == 0 {
		return ""
	}
	return m.styles.Log.Render(strings.Join(m.log, "\n")) + "\n"
}

func (m Model) keyHints() string {
	if m.raising != nil {
		return "digits: amount   enter: confirm   esc: cancel"
	}
	if !m.onClock {
		// Rebuying is only on the table for someone who has been knocked
		// out, so do not advertise it to a player who still has chips.
		if m.view.Seat == game.SpectatorSeat {
			return "r: buy in   q: quit"
		}
		return "waiting for the table   q: quit"
	}

	hints := []string{"f: fold"}
	if m.view.ToCall > 0 {
		hints = append(hints, fmt.Sprintf("c: call %d", m.view.ToCall))
	} else {
		hints = append(hints, "c: check")
	}
	if m.view.Legal(game.Raise) {
		hints = append(hints, "r: raise", "a: all-in")
	}
	hints = append(hints, "q: quit")

	return strings.Join(hints, "   ")
}
