package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styling for the human-readable report. Lipgloss detects the terminal's color
// support through termenv, so this output degrades to plain text automatically
// when stdout is piped or NO_COLOR is set, which keeps the verifier usable in
// scripts and CI.
var (
	colGreen = lipgloss.Color("42")
	colRed   = lipgloss.Color("203")
	colBlue  = lipgloss.Color("39")
	colMuted = lipgloss.Color("245")

	styleTitle = lipgloss.NewStyle().Bold(true).Foreground(colBlue)
	styleMuted = lipgloss.NewStyle().Foreground(colMuted)
	stylePass  = lipgloss.NewStyle().Bold(true).Foreground(colGreen)
	styleFail  = lipgloss.NewStyle().Bold(true).Foreground(colRed)

	stylePassBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colGreen).
			Foreground(colGreen).
			Bold(true).
			Padding(0, 2)
	styleFailBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colRed).
			Foreground(colRed).
			Bold(true).
			Padding(0, 2)
)

const (
	markPass = "✓"
	markFail = "✗"
)

// renderReport returns the full styled report for a verification run.
func renderReport(rep report) string {
	var b strings.Builder

	b.WriteString(styleTitle.Render("Stonewrit Verify"))
	b.WriteByte('\n')
	b.WriteString(styleMuted.Render("Independent hash-chain verification"))
	b.WriteString("\n\n")

	if len(rep.Chains) == 0 {
		b.WriteString(styleMuted.Render("No chains found in the bundle."))
		b.WriteString("\n\n")
	}

	for _, c := range rep.Chains {
		mark, markStyle := markPass, stylePass
		if !c.Valid {
			mark, markStyle = markFail, styleFail
		}
		line := fmt.Sprintf("%s  %s  %s",
			markStyle.Render(mark),
			shortID(c.ChainID),
			styleMuted.Render(fmt.Sprintf("%d/%d events verified", c.EventsVerified, c.EventCount)),
		)
		b.WriteString(line)
		b.WriteByte('\n')

		if c.Valid {
			continue
		}
		if !c.TipMatches && c.Break == nil {
			b.WriteString("   " + styleFail.Render("tip does not match the chain's published head"))
			b.WriteByte('\n')
		}
		if c.Break != nil {
			b.WriteString("   " + styleFail.Render(fmt.Sprintf("break at position %d: %s", c.Break.Position, c.Break.Reason)))
			b.WriteByte('\n')
			if c.Break.Expected != "" || c.Break.Stored != "" {
				b.WriteString("   " + styleMuted.Render(fmt.Sprintf("expected %s", c.Break.Expected)))
				b.WriteByte('\n')
				b.WriteString("   " + styleMuted.Render(fmt.Sprintf("stored   %s", c.Break.Stored)))
				b.WriteByte('\n')
			}
		}
	}

	b.WriteByte('\n')
	summary := fmt.Sprintf("%d chain(s), %d event(s)", rep.ChainCount, rep.EventCount)
	if rep.Valid {
		b.WriteString(stylePassBox.Render(fmt.Sprintf("%s  VALID   %s", markPass, summary)))
	} else {
		b.WriteString(styleFailBox.Render(fmt.Sprintf("%s  INVALID   at least one chain failed", markFail)))
	}
	b.WriteByte('\n')
	return b.String()
}

// shortID abbreviates a UUID-like id for compact display while keeping it
// recognizable, for example "11111111...111111111111".
func shortID(id string) string {
	if len(id) <= 13 {
		return id
	}
	return id[:8] + "…" + id[len(id)-4:]
}
