package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ssh_holdem/game"
)

// Seats are laid out around an oval the way a poker client does it, so
// whose turn it is reads at a glance rather than by scanning a list.
//
// The positions are numbered clockwise from the bottom of the table. You
// always sit at the bottom, and everyone else falls where they are
// relative to you, so the table looks the same to each player.
//
// There are ten positions for a nine-handed maximum, so a full table
// still shows one empty chair rather than squeezing the layout.
const (
	slotBottomCentre = iota
	slotBottomRight
	slotRightLower
	slotRightUpper
	slotTopRight
	slotTopCentre
	slotTopLeft
	slotLeftUpper
	slotLeftLower
	slotBottomLeft

	slotCount
)

// Dimensions of the felt layout. A seat is a fixed block so the rows
// line up whatever the names are.
const (
	seatContentWidth = 14
	seatBlockWidth   = seatContentWidth + 2 // the border, hidden or not
	seatBlockHeight  = 6

	minFeltWidth  = 20
	minFeltHeight = 22
)

// feltInner is the width the oval may use. It is wider than a text panel
// allows: three seat blocks and the board have to fit side by side, and
// a line of poker table is not a line of prose to read.
func (m Model) feltInner() int {
	width := m.width - appPadding*2
	if width < 0 {
		return 0
	}
	if width > 96 {
		return 96
	}
	return width
}

// feltFits reports whether the terminal has room for the oval. Below
// this the seat list is the honest layout.
func (m Model) feltFits() bool {
	return m.feltInner() >= seatBlockWidth*3+minFeltWidth && m.height >= minFeltHeight
}

// slotsFor spreads n players evenly around the table, so a three-handed
// game is a triangle rather than three seats bunched at the bottom.
func slotsFor(n int) []int {
	if n <= 0 {
		return nil
	}
	if n >= slotCount {
		slots := make([]int, slotCount)
		for i := range slots {
			slots[i] = i
		}
		return slots
	}

	slots := make([]int, 0, n)
	taken := make(map[int]bool, n)

	for i := 0; i < n; i++ {
		slot := int(math.Round(float64(i*slotCount)/float64(n))) % slotCount
		for taken[slot] {
			slot = (slot + 1) % slotCount
		}
		taken[slot] = true
		slots = append(slots, slot)
	}

	return slots
}

// placeSeats maps each seat to a position on the oval, counting
// clockwise from the viewer. A spectator has no seat of their own, so
// the table is drawn from seat zero.
func (m Model) placeSeats() map[int]game.SeatInfo {
	seats := m.view.Seats
	if len(seats) == 0 {
		return nil
	}

	hero := m.view.Seat
	if hero == game.SpectatorSeat || hero >= len(seats) {
		hero = 0
	}

	placed := make(map[int]game.SeatInfo, len(seats))
	for offset, slot := range slotsFor(len(seats)) {
		placed[slot] = seats[(hero+offset)%len(seats)]
	}

	return placed
}

// renderFelt draws the oval: seats around the outside, board and pot in
// the middle.
func (m Model) renderFelt() string {
	placed := m.placeSeats()
	if len(placed) == 0 {
		return m.panel(m.styles.Felt, m.styles.Dim.Render("Waiting for players..."))
	}

	feltWidth := m.feltInner() - seatBlockWidth*2
	if feltWidth < minFeltWidth {
		feltWidth = minFeltWidth
	}

	rows := make([]string, 0, 3)

	if row := m.seatRow(placed, slotTopLeft, slotTopCentre, slotTopRight); row != "" {
		rows = append(rows, row)
	}

	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center,
		m.seatColumn(placed, slotLeftUpper, slotLeftLower),
		m.centrePiece(feltWidth),
		m.seatColumn(placed, slotRightUpper, slotRightLower),
	))

	if row := m.seatRow(placed, slotBottomLeft, slotBottomCentre, slotBottomRight); row != "" {
		rows = append(rows, row)
	}

	return lipgloss.JoinVertical(lipgloss.Center, rows...)
}

// seatRow joins up to three seats side by side, or returns empty when
// none of those positions is occupied.
func (m Model) seatRow(placed map[int]game.SeatInfo, slots ...int) string {
	any := false
	blocks := make([]string, 0, len(slots))

	for _, slot := range slots {
		seat, ok := placed[slot]
		if ok {
			any = true
		}
		blocks = append(blocks, m.seatBlock(seat, ok))
	}

	if !any {
		return ""
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, blocks...)
}

// seatColumn stacks the two seats on one side of the table.
func (m Model) seatColumn(placed map[int]game.SeatInfo, slots ...int) string {
	blocks := make([]string, 0, len(slots))
	for _, slot := range slots {
		seat, ok := placed[slot]
		blocks = append(blocks, m.seatBlock(seat, ok))
	}
	return lipgloss.JoinVertical(lipgloss.Left, blocks...)
}

// seatBlock draws one player. An empty position is drawn at the same
// size so nothing shifts as players come and go.
func (m Model) seatBlock(seat game.SeatInfo, occupied bool) string {
	border := lipgloss.HiddenBorder()
	style := m.styles.Chips

	if occupied && seat.Index == m.view.Acting {
		border = lipgloss.RoundedBorder()
		style = m.styles.SeatTurn
	}

	box := m.styles.Chips.
		Border(border).
		BorderForeground(m.styles.SeatTurn.GetForeground()).
		Width(seatContentWidth).
		Height(seatBlockHeight - 2)

	if !occupied {
		return box.Render("")
	}

	var b strings.Builder

	name := truncate(seat.Name, seatContentWidth-4)
	if seat.Index == m.view.Seat {
		style = m.styles.SeatYou
	}
	if seat.Folded {
		style = m.styles.SeatOut
	}

	badge := "  "
	if seat.IsButton {
		badge = m.styles.Button.Render(" D ")
	}

	b.WriteString(style.Render(name) + " " + badge + "\n")
	b.WriteString(m.styles.Chips.Render(fmt.Sprintf("%d", seat.Chips)) + "\n")

	switch {
	case seat.Folded:
		b.WriteString(m.styles.SeatOut.Render("folded"))
	case seat.AllIn:
		b.WriteString(m.styles.Urgent.Render(fmt.Sprintf("all in %d", seat.CurrentBet)))
	case seat.CurrentBet > 0:
		b.WriteString(m.styles.Pot.Render(fmt.Sprintf("bets %d", seat.CurrentBet)))
	}
	b.WriteString("\n")

	if seat.Index == m.view.Acting {
		b.WriteString(m.clockBar(seatContentWidth - 4))
	}

	return box.Render(b.String())
}

// centrePiece is the middle of the table: the board and the pot.
func (m Model) centrePiece(width int) string {
	var b strings.Builder

	b.WriteString(m.styles.Dim.Render(m.view.Street.String()) + "\n")
	b.WriteString(m.renderCards(m.view.Board) + "\n")
	b.WriteString(m.styles.Pot.Render(fmt.Sprintf("pot %d", m.view.Pot)))

	// The board spans both seats on either side, so the oval reads as one
	// table rather than a box with chairs floating beside it.
	return m.styles.Felt.
		Width(width-2).
		Height(seatBlockHeight*2-4).
		Align(lipgloss.Center, lipgloss.Center).
		Render(b.String())
}
