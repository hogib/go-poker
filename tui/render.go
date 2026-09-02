package tui

import (
	"fmt"
	"strings"
	"time"

	"ssh_holdem/deck"
	"ssh_holdem/game"
)

func (m Model) View() string {
	var body string

	switch m.screen {
	case screenMenu:
		body = m.renderMenu()
	case screenRules:
		body = m.renderRules()
	case screenHelp:
		body = m.renderHelp()
	default:
		body = m.renderTable()
	}

	return m.styles.App.Render(m.header() + "\n" + body)
}

func (m Model) header() string {
	logo := m.styles.Logo.Render("♠ ssh holdem ♥")
	tag := m.styles.Tag.Render("  no-limit texas hold'em, over ssh")
	return logo + tag + "\n"
}

// ---- lobby ----------------------------------------------------------

func (m Model) renderMenu() string {
	var b strings.Builder

	for i, item := range m.menu() {
		label := item.label(m)

		switch {
		case !item.enabled(m):
			b.WriteString("    " + m.styles.MenuDisabled.Render(label))
		case i == m.cursor:
			b.WriteString(m.styles.MenuCursor.Render("  ▸ ") +
				m.styles.MenuSelected.Render(label))
		default:
			b.WriteString("    " + m.styles.MenuItem.Render(label))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n" + m.styles.Dim.Render(strings.Repeat("─", 26)) + "\n")
	b.WriteString(m.styles.Dim.Render(m.tableSummary()))

	panel := m.styles.Panel.Render(b.String())

	footer := m.hint("↑↓", "move", "enter", "select", "q", "quit")
	if m.status != "" {
		footer = m.styles.Prompt.Render(m.status) + "\n" + footer
	}

	return panel + "\n" + footer + "\n"
}

func (m Model) tableSummary() string {
	if !m.hasLobby {
		return "connecting..."
	}

	r := m.lobby.Rules
	lines := []string{
		fmt.Sprintf("%d/%d blinds · %d buy-in", r.SmallBlind, r.BigBlind, r.BuyIn),
		fmt.Sprintf("%s · %s watching", plural(m.lobby.Seated, "seated", "seated"),
			plural(m.lobby.Watching, "player", "players")),
	}

	if m.lobby.YouAreSeated {
		lines = append(lines, fmt.Sprintf("you are in for %d", m.lobby.YourChips))
	}

	return strings.Join(lines, "\n")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func (m Model) renderRules() string {
	r := m.lobby.Rules

	rows := [][2]string{
		{"Game", "No-limit Texas hold'em"},
		{"Blinds", fmt.Sprintf("%d / %d", r.SmallBlind, r.BigBlind)},
		{"Buy-in", fmt.Sprintf("%d", r.BuyIn)},
		{"Seats", fmt.Sprintf("%d maximum", maxSeats)},
		{"Shot clock", humanDuration(r.TurnTimeout)},
		{"Between hands", humanDuration(r.HandDelay)},
	}

	var b strings.Builder
	for _, row := range rows {
		b.WriteString(m.styles.Dim.Render(fmt.Sprintf("%-15s", row[0])))
		b.WriteString(m.styles.Bright.Render(row[1]))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.styles.Dim.Render(
		"Run out of chips and you can buy in again for the next hand.\n" +
			"Let the shot clock expire and you check if you can, fold if you cannot."))

	return m.styles.Panel.Render(b.String()) + "\n" +
		m.hint("esc", "back") + "\n"
}

func humanDuration(d time.Duration) string {
	if d == 0 {
		return "none"
	}
	if d >= time.Minute {
		return d.Round(time.Second).String()
	}
	return fmt.Sprintf("%.0f seconds", d.Seconds())
}

func (m Model) renderHelp() string {
	rows := [][2]string{
		{"f", "fold"},
		{"c", "check, or call the outstanding bet"},
		{"r", "raise — type an amount, enter to confirm"},
		{"a", "shove all in"},
		{"esc", "back to the menu"},
		{"q", "quit"},
	}

	var b strings.Builder
	b.WriteString(m.styles.Bright.Render("At the table") + "\n\n")
	for _, row := range rows {
		b.WriteString("  " + m.styles.KeyCap.Render(fmt.Sprintf("%-5s", row[0])))
		b.WriteString(m.styles.Dim.Render(row[1]) + "\n")
	}

	b.WriteString("\n" + m.styles.Bright.Render("Good to know") + "\n\n")
	b.WriteString(m.styles.Dim.Render(
		"  The seat on the clock is marked ▸, and your own seat is highlighted.\n" +
			"  Reconnect with the same SSH key and you keep your seat and stack.\n" +
			"  Leaving your seat banks your chips; sitting back down returns them."))

	return m.styles.Panel.Render(b.String()) + "\n" +
		m.hint("esc", "back") + "\n"
}

// ---- felt -----------------------------------------------------------

func (m Model) renderTable() string {
	if !m.hasView {
		return m.styles.Felt.Render(m.styles.Dim.Render("Waiting for the table...")) +
			"\n" + m.hint("esc", "menu", "q", "quit") + "\n"
	}

	var b strings.Builder

	b.WriteString(m.renderSeats())
	b.WriteString("\n")
	b.WriteString(m.renderBoard())

	if m.view.Seat != game.SpectatorSeat {
		b.WriteString("\n" + m.renderHole())
	}

	felt := m.styles.Felt.Render(b.String())

	var out strings.Builder
	out.WriteString(felt + "\n")

	if prompt := m.renderPrompt(); prompt != "" {
		out.WriteString(prompt + "\n")
	}
	if log := m.renderLog(); log != "" {
		out.WriteString(log)
	}
	out.WriteString(m.keyHints() + "\n")

	return out.String()
}

func (m Model) renderSeats() string {
	var b strings.Builder

	for _, seat := range m.view.Seats {
		marker := "  "
		if seat.Index == m.view.Acting {
			marker = m.styles.SeatTurn.Render("▸ ")
		}

		badge := "  "
		if seat.IsButton {
			badge = m.styles.Button.Render(" D ")
		}

		name := fmt.Sprintf("%-12s", truncate(seat.Name, 12))
		chips := fmt.Sprintf("%7d", seat.Chips)

		style := m.styles.Chips
		switch {
		case seat.Folded:
			style = m.styles.SeatOut
		case seat.Index == m.view.Seat:
			style = m.styles.SeatYou
		case seat.Index == m.view.Acting:
			style = m.styles.SeatTurn
		}

		b.WriteString(marker + badge + " " + style.Render(name+chips))

		switch {
		case seat.Folded:
			b.WriteString(m.styles.SeatOut.Render("   folded"))
		case seat.AllIn:
			b.WriteString(m.styles.Urgent.Render(fmt.Sprintf("   all in %d", seat.CurrentBet)))
		case seat.CurrentBet > 0:
			b.WriteString(m.styles.Dim.Render(fmt.Sprintf("   bets %d", seat.CurrentBet)))
		}

		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) renderBoard() string {
	street := m.styles.Dim.Render(fmt.Sprintf("%-8s", m.view.Street.String()))
	board := m.renderCards(m.view.Board)
	pot := m.styles.Pot.Render(fmt.Sprintf("pot %d", m.view.Pot))

	return fmt.Sprintf("%s %s   %s\n", street, board, pot)
}

func (m Model) renderHole() string {
	return fmt.Sprintf("%s %s\n",
		m.styles.Dim.Render(fmt.Sprintf("%-8s", "you")),
		m.renderCards(m.view.Hole))
}

// renderCards colours the suits, which is most of what makes a hand
// readable at a glance.
func (m Model) renderCards(cards []deck.Card) string {
	if len(cards) == 0 {
		return m.styles.Dim.Render("--")
	}

	parts := make([]string, 0, len(cards))
	for _, c := range cards {
		style := m.styles.Black
		if c.Suit() == deck.Hearts || c.Suit() == deck.Diamonds {
			style = m.styles.Red
		}
		parts = append(parts, style.Render(c.String()))
	}

	return strings.Join(parts, " ")
}

func (m Model) renderPrompt() string {
	if m.raising != nil {
		typed := m.raising.digits
		if typed == "" {
			typed = "_"
		}
		return m.styles.Prompt.Render(fmt.Sprintf("Raise to %s", typed)) +
			m.styles.Dim.Render(fmt.Sprintf("   (%d to %d, enter to confirm, esc to cancel)",
				m.raising.min, m.raising.max))
	}

	if !m.onClock {
		if m.status != "" {
			return m.styles.Dim.Render(m.status)
		}
		return ""
	}

	prompt := m.styles.Prompt.Render(fmt.Sprintf("Your move — %d to call", m.view.ToCall))
	if m.view.ToCall == 0 {
		prompt = m.styles.Prompt.Render("Your move — checking is free")
	}

	if !m.view.Deadline.IsZero() {
		if remaining := time.Until(m.view.Deadline); remaining > 0 {
			seconds := int(remaining.Seconds()) + 1
			clock := m.styles.Clock
			if seconds <= 5 {
				clock = m.styles.Urgent
			}
			prompt += clock.Render(fmt.Sprintf("   %ds", seconds))
		}
	}

	return prompt
}

func (m Model) renderLog() string {
	if len(m.log) == 0 {
		return ""
	}

	lines := make([]string, 0, len(m.log))
	for _, line := range m.log {
		lines = append(lines, m.styles.Log.Render("  "+line))
	}

	return strings.Join(lines, "\n") + "\n"
}

func (m Model) keyHints() string {
	if m.raising != nil {
		return m.hint("0-9", "amount", "enter", "confirm", "esc", "cancel")
	}

	if !m.onClock {
		if m.canRebuy() {
			return m.hint("r", "buy in", "esc", "menu", "q", "quit")
		}
		return m.hint("esc", "menu", "q", "quit")
	}

	pairs := []string{"f", "fold"}
	if m.view.ToCall > 0 {
		pairs = append(pairs, "c", fmt.Sprintf("call %d", m.view.ToCall))
	} else {
		pairs = append(pairs, "c", "check")
	}
	if m.view.Legal(game.Raise) {
		pairs = append(pairs, "r", "raise", "a", "all in")
	}
	pairs = append(pairs, "esc", "menu")

	return m.hint(pairs...)
}

// hint renders alternating key/description pairs as a footer.
func (m Model) hint(pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts,
			m.styles.KeyCap.Render(pairs[i])+" "+m.styles.Key.Render(pairs[i+1]))
	}
	return "  " + strings.Join(parts, m.styles.Key.Render("  ·  "))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
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
		if len(pot.Winners) == 0 {
			continue
		}

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
