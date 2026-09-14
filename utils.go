package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	colorful "github.com/lucasb-eyer/go-colorful"
	"github.com/muesli/termenv"
)

// — existing helpers (unchanged) —

func checkbox(label string, checked bool) string {
	if checked {
		return colorFg("[x] "+label, "212")
	}
	return fmt.Sprintf("[ ] %s", label)
}

func progressbar(percent float64) string {
	w := float64(progressBarWidth)

	fullSize := min(int(math.Round(w*percent)), len(ramp))
	var fullCells string
	for i := 0; i < fullSize; i++ {
		fullCells += termenv.String(progressFullChar).Foreground(term.Color(ramp[i])).String()
	}

	emptySize := int(w) - fullSize
	emptyCells := strings.Repeat(progressEmpty, emptySize)

	return fmt.Sprintf("%s%s %3.0f", fullCells, emptyCells, math.Round(percent*100))
}

func colorFg(val, color string) string {
	return termenv.String(val).Foreground(term.Color(color)).String()
}

func makeFgStyle(color string) func(string) string {
	return termenv.Style{}.Foreground(term.Color(color)).Styled
}

func makeRamp(colorA, colorB string, steps float64) (s []string) {
	cA, _ := colorful.Hex(colorA)
	cB, _ := colorful.Hex(colorB)
	for i := 0.0; i < steps; i++ {
		c := cA.BlendLuv(cB, i/steps)
		s = append(s, colorToHex(c))
	}
	return
}

func colorToHex(c colorful.Color) string {
	return fmt.Sprintf("#%s%s%s", colorFloatToHex(c.R), colorFloatToHex(c.G), colorFloatToHex(c.B))
}

func colorFloatToHex(f float64) (s string) {
	s = strconv.FormatInt(int64(f*255), 16)
	if len(s) == 1 {
		s = "0" + s
	}
	return
}

// — new helpers —

func dashboardPanel(sessions []Session, current options, currentName string, minutes int, streak int, width int) string {
	innerWidth := width - 6
	if innerWidth < 50 {
		innerWidth = 50
	}

	var rows []string
	for _, s := range sessions {
		name := s.Name
		if name == "" {
			name = s.Type
		}
		row := fmt.Sprintf("%-22s %-6s %2d min  ✓", truncate(name, 22), s.Type, s.DurationMin)
		rows = append(rows, mutedStyle.Render(row))
	}

	runningName := currentName
	if runningName == "" {
		runningName = current.Type
	}
	runningRow := fmt.Sprintf("%-22s %-6s %2d min  …", truncate(runningName, 22), current.Type, current.Time)
	rows = append(rows, accentStyle.Render("▶ "+runningRow))

	body := strings.Join(rows, "\n")
	divider := dimStyle.Render(strings.Repeat("─", innerWidth-2))

	footerParts := []string{accentStyle.Render(fmt.Sprintf("Total focus: %d min", minutes))}
	if streak > 0 {
		footerParts = append(footerParts, accentStyle.Render(fmt.Sprintf("🔥 %d day streak", streak)))
	}
	footer := strings.Join(footerParts, dimStyle.Render("   •   "))

	title := dimStyle.Render("Today")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("238")).
		Padding(0, 1).
		Width(innerWidth).
		Render(title + "\n" + body + "\n" + divider + "\n" + footer)
}

func completionMenuView(m model) string {
	label := m.Options[m.ActiveChoiceIdx].Type
	if m.RunID > 0 && m.CurrentInterval < len(m.TemplateIntervals) {
		label = fmt.Sprintf("%s (Interval %d of %d)",
			m.TemplateIntervals[m.CurrentInterval].Type,
			m.CurrentInterval+1, len(m.TemplateIntervals))
	}
	if m.SessionName != "" {
		label = m.SessionName
	}

	var completedDur int
	if m.LastSessionDuration > 0 {
		completedDur = m.LastSessionDuration
	} else if m.RunID > 0 && m.CurrentInterval < len(m.TemplateIntervals) {
		completedDur = m.TemplateIntervals[m.CurrentInterval].DurationMin
	} else {
		completedDur = m.Options[m.ActiveChoiceIdx].Time
	}
	title := accentStyle.Render(fmt.Sprintf("✓  %s — %d min", label, completedDur))

	var choices []string
	if m.RunID > 0 {
		next := m.CurrentInterval + 1
		if next < len(m.TemplateIntervals) {
			iv := m.TemplateIntervals[next]
			choices = []string{
				fmt.Sprintf("▶  Next: %s — %d min", intervalTypeStyle(iv.Type), iv.DurationMin),
				"Skip to next",
				"End run early",
				"Quit",
			}
		} else {
			choices = []string{"Back to home", "Quit"}
		}
	} else {
		choices = []string{
			"Focus again",
			fmt.Sprintf("Break — %d min", m.Options[1].Time),
			fmt.Sprintf("Long break — %d min", m.Options[2].Time),
			"Back to home",
			"Quit",
		}
	}

	maxChoice := len(choices) - 1
	dc := m.DoneChoice
	if dc > maxChoice {
		dc = maxChoice
	}

	var rows []string
	for i, c := range choices {
		if i == dc {
			rows = append(rows, accentStyle.Render(fmt.Sprintf("[x] %s", c)))
		} else {
			rows = append(rows, mutedStyle.Render(fmt.Sprintf("[ ] %s", c)))
		}
	}

	summaryLine := ""
	if m.SessionSummaryLoading {
		summaryLine = "\n" + dimStyle.Render("✦ summarising...")
	} else if m.SessionSummary != "" {
		summaryLine = "\n" + mutedStyle.Render("✦ "+m.SessionSummary)
	}

	achievementLines := ""
	for _, a := range m.NewAchievements {
		achievementLines += "\n" + accentStyle.Render("✨ Achievement unlocked: "+a)
	}

	hints := dimStyle.Render("m: add note  •  j/k: navigate  •  enter: select")
	return panelStyle.Render(title + summaryLine + achievementLines + "\n\n" + strings.Join(rows, "\n") + "\n\n" + hints)
}

func notesView(m model) string {
	title := headerStyle.Render("Session note")
	input := panelStyle.Render(m.NotesInput + "▌")
	hint := dimStyle.Render("ctrl+s: save  •  esc: skip")
	if m.NotesConfirmDiscard {
		hint = warnStyle.Render("Unsaved note — esc again to discard")
	}
	return title + "\n\n" + input + "\n" + hint
}

func reportView(data []DayReport, width int, mode int) string {
	tabs := []string{"Daily", "Weekly", "Monthly"}
	var tabParts []string
	for i, t := range tabs {
		if i == mode {
			tabParts = append(tabParts, accentStyle.Render("[ "+t+" ]"))
		} else {
			tabParts = append(tabParts, mutedStyle.Render("[ "+t+" ]"))
		}
	}
	tabBar := strings.Join(tabParts, dimStyle.Render("  ")) + dimStyle.Render("  tab: cycle")

	title := headerStyle.Render("Focus Report") + "\n" + tabBar

	if len(data) == 0 {
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1).
			Render(title + "\n\n" + mutedStyle.Render("No sessions logged yet.") + "\n\n" + dimStyle.Render("backspace or r to close"))
	}

	maxMin := 0
	for _, d := range data {
		if d.TotalMin > maxMin {
			maxMin = d.TotalMin
		}
	}

	const maxBar = 20
	var rows []string
	for _, d := range data {
		barLen := 0
		if maxMin > 0 {
			barLen = int(float64(d.TotalMin) / float64(maxMin) * maxBar)
		}
		bar := accentStyle.Render(strings.Repeat("█", barLen)) +
			dimStyle.Render(strings.Repeat("░", maxBar-barLen))
		unit := "session"
		if d.Count != 1 {
			unit = "sessions"
		}
		row := fmt.Sprintf("%-12s  %s  %3d min  %d %s", d.Date, bar, d.TotalMin, d.Count, unit)
		rows = append(rows, row)
		if d.Notes != "" {
			snippets := strings.Split(d.Notes, "||")
			for _, s := range snippets {
				if strings.TrimSpace(s) != "" {
					rows = append(rows, mutedStyle.Render("  └ "+s))
				}
			}
		}
	}

	innerWidth := width - 6
	if innerWidth < 65 {
		innerWidth = 65
	}

	body := title + "\n\n" + strings.Join(rows, "\n") + "\n\n" + dimStyle.Render("backspace or r: close  •  x: export CSV")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("238")).
		Padding(0, 1).
		Width(innerWidth).
		Render(body)
}

func namingView(m model) string {
	input := panelStyle.Render(m.NameInput + "▌")
	return headerStyle.Render("Name this session:") + "\n" + input + "\n" +
		mutedStyle.Render("enter") + dimStyle.Render(" to confirm  •  ") +
		mutedStyle.Render("esc") + dimStyle.Render(" to cancel")
}

func truncate(s string, maxLen int) string {
	if len([]rune(s)) <= maxLen {
		return s
	}
	return string([]rune(s)[:maxLen-1]) + "…"
}
