package tui

import (
	"regexp"

	"github.com/charmbracelet/lipgloss"
)

var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

// stripStyles removes colour codes so a test can measure or search the
// text a player actually sees.
func stripStyles(s string) string { return ansiPattern.ReplaceAllString(s, "") }

// lineWidth is the number of terminal cells a rendered line occupies.
func lineWidth(s string) int { return lipgloss.Width(stripStyles(s)) }
