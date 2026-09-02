// Package tui renders the lobby and the felt for one player over their
// SSH session. It holds no game state of its own: every frame is drawn
// from the last redacted snapshot the table pushed.
package tui

import "github.com/charmbracelet/lipgloss"

// Styles is the palette. It is built per session, because the colours
// available depend on the terminal that connected.
type Styles struct {
	App    lipgloss.Style
	Logo   lipgloss.Style
	Tag    lipgloss.Style
	Panel  lipgloss.Style
	Felt   lipgloss.Style
	Dim    lipgloss.Style
	Bright lipgloss.Style

	MenuItem     lipgloss.Style
	MenuSelected lipgloss.Style
	MenuCursor   lipgloss.Style
	MenuDisabled lipgloss.Style

	Board  lipgloss.Style
	Hole   lipgloss.Style
	Red    lipgloss.Style
	Black  lipgloss.Style
	Chips  lipgloss.Style
	Pot    lipgloss.Style
	Button lipgloss.Style

	SeatYou  lipgloss.Style
	SeatTurn lipgloss.Style
	SeatOut  lipgloss.Style

	Prompt lipgloss.Style
	Clock  lipgloss.Style
	Urgent lipgloss.Style
	Key    lipgloss.Style
	KeyCap lipgloss.Style
	Log    lipgloss.Style
	Win    lipgloss.Style
}

// NewStyles builds the palette from a renderer bound to one session. Pass
// bubbletea.MakeRenderer(sess) so the colours match what that terminal
// can actually show; the zero renderer is fine for tests.
func NewStyles(r *lipgloss.Renderer) Styles {
	if r == nil {
		r = lipgloss.DefaultRenderer()
	}
	base := r.NewStyle()

	var (
		felt   = lipgloss.Color("22")
		gold   = lipgloss.Color("178")
		cream  = lipgloss.Color("223")
		muted  = lipgloss.Color("245")
		red    = lipgloss.Color("174")
		white  = lipgloss.Color("255")
		accent = lipgloss.Color("110")
		alarm  = lipgloss.Color("203")
	)

	panel := base.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(gold).
		Padding(1, 3)

	return Styles{
		App:    base.Padding(1, 2),
		Logo:   base.Bold(true).Foreground(gold),
		Tag:    base.Foreground(muted).Italic(true),
		Panel:  panel,
		Felt:   panel.BorderForeground(felt),
		Dim:    base.Foreground(muted),
		Bright: base.Foreground(white),

		MenuItem:     base.Foreground(cream),
		MenuSelected: base.Bold(true).Foreground(white),
		MenuCursor:   base.Bold(true).Foreground(gold),
		MenuDisabled: base.Foreground(muted).Faint(true),

		Board:  base.Bold(true),
		Hole:   base.Bold(true),
		Red:    base.Foreground(red),
		Black:  base.Foreground(white),
		Chips:  base.Foreground(cream),
		Pot:    base.Bold(true).Foreground(gold),
		Button: base.Bold(true).Foreground(lipgloss.Color("232")).Background(gold),

		SeatYou:  base.Bold(true).Foreground(gold),
		SeatTurn: base.Bold(true).Foreground(accent),
		SeatOut:  base.Foreground(muted).Faint(true),

		Prompt: base.Bold(true).Foreground(accent),
		Clock:  base.Foreground(muted),
		Urgent: base.Bold(true).Foreground(alarm),
		Key:    base.Foreground(muted),
		KeyCap: base.Bold(true).Foreground(cream),
		Log:    base.Foreground(muted),
		Win:    base.Bold(true).Foreground(gold),
	}
}
