package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const homeMenuWidth = 44

func intervalTypeStyle(t string) string {
	if t == "Focus" {
		return focusStyle.Render(t)
	}
	return breakStyle.Render(t)
}

func homeView(m model) string {
	header := m.HomeHeader
	if header == "" {
		header = headerStyle.Render("POMO TIME!") + "\n"
	}
	// center the figlet header
	centeredHeader := lipgloss.NewStyle().Width(m.Width).Align(lipgloss.Center).Render(header)

	stats := ""
	if m.TodayMinutes > 0 || m.Streak > 0 {
		parts := []string{}
		if m.TodayMinutes > 0 {
			parts = append(parts, fmt.Sprintf("Today: %d min", m.TodayMinutes))
		}
		if m.Streak > 0 {
			parts = append(parts, fmt.Sprintf("🔥 %d day streak", m.Streak))
		}
		stats = accentStyle.Render(strings.Join(parts, "   ")) + "\n\n"
	}

	choices := []string{
		"▶  Quick session",
		"📋  From template",
		"↩  Continue session",
		"📊  Report",
		"🗂  Sessions",
		"⚙  Manage templates",
		"🔧  Config",
	}
	if m.BackgroundTimer {
		mins := m.Ticks / 60
		secs := m.Ticks % 60
		resumeLabel := fmt.Sprintf("⏱  Resume — %02d:%02d remaining", mins, secs)
		choices = append([]string{resumeLabel}, choices...)
	}

	list := ""
	for i, c := range choices {
		if i == 0 && m.BackgroundTimer {
			if m.Choice == 0 {
				list += accentStyle.Render("[x] "+c) + "\n"
			} else {
				list += accentStyle.Render("[ ] "+c) + "\n"
			}
			continue
		}
		list += fmt.Sprintf("%s\n", checkbox(c, m.Choice == i))
	}

	nameHint := ""
	if m.SessionName != "" {
		nameHint = "\n" + accentStyle.Render("session: "+m.SessionName)
	}

	menuContent := panelStyle.Width(homeMenuWidth).Render(stats + strings.TrimRight(list, "\n") + nameHint)
	centeredMenu := lipgloss.Place(m.Width, 0, lipgloss.Center, lipgloss.Top, menuContent)

	hints := lipgloss.NewStyle().Width(m.Width).Align(lipgloss.Center).Render(
		subtle("j/k") + dimStyle.Render(": navigate") + dot +
			subtle("enter") + dimStyle.Render(": select") + dot +
			subtle("n") + dimStyle.Render(": name") + dot +
			subtle("?") + dimStyle.Render(": help") + dot +
			subtle("q") + dimStyle.Render(": quit"),
	)

	return centeredHeader + "\n" + centeredMenu + "\n" + hints
}

func continueSessionView(m model) string {
	title := headerStyle.Render("Continue session") + "\n\n"
	if len(m.RecentNames) == 0 {
		return title + mutedStyle.Render("No named sessions found.") +
			"\n\n" + dimStyle.Render("esc: back")
	}
	var rows []string
	for i, name := range m.RecentNames {
		if i == m.RecentNameCursor {
			rows = append(rows, accentStyle.Render("[x] "+name))
		} else {
			rows = append(rows, mutedStyle.Render("[ ] "+name))
		}
	}
	hints := "\n" + subtle("j/k") + dimStyle.Render(": navigate") + dot +
		subtle("enter") + dimStyle.Render(": select") + dot +
		subtle("backspace") + dimStyle.Render(": back")
	return title + strings.Join(rows, "\n") + hints
}

func sessionManagerView(m model) string {
	title := headerStyle.Render("Sessions") + "\n\n"
	if len(m.AllSessions) == 0 {
		return title + mutedStyle.Render("No sessions yet.") +
			"\n\n" + dimStyle.Render("backspace: back")
	}

	viewH := m.Height - 10
	if viewH < 5 {
		viewH = 5
	}
	start := m.SessionMgrOffset
	end := start + viewH
	if end > len(m.AllSessions) {
		end = len(m.AllSessions)
	}
	visible := m.AllSessions[start:end]

	var rows []string
	for ii, s := range visible {
		i := start + ii
		day := s.StartedAt.Format("2006-01-02")
		name := s.Name
		if name == "" {
			name = s.Type
		}
		noteMarker := ""
		if s.Notes != "" {
			noteMarker = " ✎"
		}
		row := fmt.Sprintf("#%-4d %s  %-22s %-7s %2d min%s",
			s.ID, day, truncate(name, 22), s.Type, s.DurationMin, noteMarker)
		if i == m.SessionMgrCursor {
			rows = append(rows, accentStyle.Render("[x] "+row))
		} else {
			rows = append(rows, mutedStyle.Render("[ ] "+row))
		}
	}

	scrollInfo := ""
	if len(m.AllSessions) > viewH {
		scrollInfo = dimStyle.Render(fmt.Sprintf(" (%d-%d of %d)", start+1, end, len(m.AllSessions)))
	}

	notePanel := ""
	if m.ViewingNote && m.SessionMgrCursor < len(m.AllSessions) {
		note := m.AllSessions[m.SessionMgrCursor].Notes
		if note != "" {
			notePanel = "\n\n" + panelStyle.Render(accentStyle.Render("Note")+"\n"+mutedStyle.Render(note))
		}
	}

	extra := ""
	if m.DeleteSessionConfirm {
		extra = "\n\n" + warnStyle.Render("Delete this session? Press y to confirm, esc to cancel")
	} else if m.CleanOldCount == -1 {
		extra = "\n\n" + mutedStyle.Render("Cleanup is disabled (cleanup_enabled: false in config).")
	} else if m.CleanNoResults {
		extra = "\n\n" + mutedStyle.Render(fmt.Sprintf("No sessions older than %d days.", m.Config.OldSessionDays))
	} else if m.CleanOldConfirm {
		extra = "\n\n" + warnStyle.Render(fmt.Sprintf("Delete %d session(s) older than %d days? Press y to confirm, esc to cancel", m.CleanOldCount, m.Config.OldSessionDays))
	}

	vHint := subtle("v") + dimStyle.Render(": note")
	if m.SessionMgrCursor < len(m.AllSessions) && m.AllSessions[m.SessionMgrCursor].Notes == "" {
		vHint = dimStyle.Render("v: note")
	}
	hints := "\n" + subtle("j/k") + dimStyle.Render(": navigate") + dot +
		subtle("d") + dimStyle.Render(": delete") + dot +
		subtle("c") + dimStyle.Render(": clean old") + dot +
		vHint + dot +
		subtle("backspace") + dimStyle.Render(": back") + scrollInfo
	return title + strings.Join(rows, "\n") + notePanel + extra + hints
}

func templatePickerView(m model) string {
	title := headerStyle.Render("From template") + "\n\n"
	if len(m.Templates) == 0 {
		return title + mutedStyle.Render("No templates yet. Go to Manage templates to create one.") +
			"\n\n" + dimStyle.Render("esc: back")
	}
	var rows []string
	for i, t := range m.Templates {
		summary := formatIntervalSummary(t)
		row := fmt.Sprintf("%-20s  %s", truncate(t.Name, 20), summary)
		if i == m.TemplateCursor {
			rows = append(rows, accentStyle.Render("[x] "+row))
		} else {
			rows = append(rows, mutedStyle.Render("[ ] "+row))
		}
	}
	hints := "\n" + subtle("j/k") + dimStyle.Render(": navigate") + dot +
		subtle("enter") + dimStyle.Render(": start") + dot +
		subtle("e") + dimStyle.Render(": edit") + dot +
		subtle("backspace") + dimStyle.Render(": back")
	return title + strings.Join(rows, "\n") + hints
}

func templateManagerView(m model) string {
	title := headerStyle.Render("Manage templates") + "\n\n"
	if len(m.Templates) == 0 {
		return title + mutedStyle.Render("No templates yet.") +
			"\n\n" + subtle("n") + dimStyle.Render(": new") + dot + subtle("esc") + dimStyle.Render(": back")
	}
	var rows []string
	for i, t := range m.Templates {
		summary := formatIntervalSummary(t)
		row := fmt.Sprintf("%-20s  %s", truncate(t.Name, 20), summary)
		if i == m.TemplateCursor {
			rows = append(rows, accentStyle.Render("[x] "+row))
		} else {
			rows = append(rows, mutedStyle.Render("[ ] "+row))
		}
	}
	extra := ""
	if m.DeleteConfirm {
		extra = "\n\n" + warnStyle.Render("Delete this template? Press y to confirm, esc to cancel")
	}
	hints := "\n" + subtle("j/k") + dimStyle.Render(": navigate") + dot +
		subtle("enter/e") + dimStyle.Render(": edit") + dot +
		subtle("n") + dimStyle.Render(": new") + dot +
		subtle("d") + dimStyle.Render(": delete") + dot +
		subtle("backspace") + dimStyle.Render(": back")
	return title + strings.Join(rows, "\n") + extra + hints
}

func templateEditorView(m model) string {
	name := m.EditorNameInput
	if name == "" {
		name = "(unnamed)"
	}
	title := headerStyle.Render("Template editor") + "\n"
	nameRow := ""
	if m.EditorNaming {
		nameRow = accentStyle.Render("Name: "+m.EditorNameInput+"▌") + "\n\n"
	} else {
		nameRow = accentStyle.Render("Name: "+name) + "\n\n"
	}

	var rows []string
	for i, iv := range m.EditorIntervals {
		typeLabel := intervalTypeStyle(iv.Type)
		row := fmt.Sprintf("%d. %s %d min", i+1, typeLabel, iv.DurationMin)
		if i == m.EditorCursor {
			rows = append(rows, accentStyle.Render("[x] ")+row)
		} else {
			rows = append(rows, mutedStyle.Render("[ ] ")+row)
		}
	}
	if len(rows) == 0 {
		rows = append(rows, dimStyle.Render("(no intervals — press a to add)"))
	}

	hints := "\n" + subtle("j/k") + dimStyle.Render(": navigate") + dot +
		subtle("a") + dimStyle.Render(": add interval") + dot +
		subtle("d") + dimStyle.Render(": delete interval") + dot +
		subtle("enter") + dimStyle.Render(": edit interval") + dot +
		subtle("n") + dimStyle.Render(": rename") + dot +
		subtle("s") + dimStyle.Render(": save") + dot +
		subtle("backspace") + dimStyle.Render(": discard")
	return title + nameRow + strings.Join(rows, "\n") + hints
}

func intervalEditorView(m model) string {
	title := headerStyle.Render("Edit interval") + "\n\n"
	if m.EditorCursor < 0 || m.EditorCursor >= len(m.EditorIntervals) {
		return title + mutedStyle.Render("No interval selected.") + "\n" + dimStyle.Render("esc: back")
	}
	iv := m.EditorIntervals[m.EditorCursor]
	currentType := iv.Type
	durDisplay := m.EditorDurBuf
	if durDisplay == "" {
		durDisplay = fmt.Sprintf("%d", iv.DurationMin)
	}

	body := fmt.Sprintf("Type: %s\nDuration: %s min\n",
		intervalTypeStyle(currentType),
		accentStyle.Render(durDisplay+"▌"),
	)
	hints := "\n" + subtle("t") + dimStyle.Render(": toggle Focus/Break") + dot +
		subtle("0-9") + dimStyle.Render(": set duration") + dot +
		subtle("enter") + dimStyle.Render(": confirm") + dot +
		subtle("backspace") + dimStyle.Render(": cancel")
	return title + body + hints
}

func totalTemplateDuration(intervals []TemplateInterval) int {
	total := 0
	for _, iv := range intervals {
		total += iv.DurationMin
	}
	return total
}

func formatIntervalSummary(t Template) string {
	n := len(t.Intervals)
	total := totalTemplateDuration(t.Intervals)
	hours := total / 60
	mins := total % 60
	durStr := ""
	if hours > 0 {
		durStr = fmt.Sprintf("%dh %dmin", hours, mins)
	} else {
		durStr = fmt.Sprintf("%dmin", mins)
	}
	return fmt.Sprintf("%d intervals · %s", n, durStr)
}

func configView(m model) string {
	title := headerStyle.Render("Config") + "\n\n"

	type field struct {
		label string
		value string
	}
	fields := []field{
		{"old_session_days", fmt.Sprintf("%d", m.Config.OldSessionDays)},
		{"cleanup_enabled", fmt.Sprintf("%v", m.Config.CleanupEnabled)},
		{"csv_export_path", m.Config.CSVExportPath},
	}

	var rows []string
	for i, f := range fields {
		val := f.value
		if i == m.ConfigCursor && m.ConfigEditing {
			switch i {
			case 0:
				val = m.ConfigIntBuf + "▌"
			case 2:
				val = m.ConfigStrBuf + "▌"
			}
		}
		row := fmt.Sprintf("%-22s %s", f.label, val)
		if i == m.ConfigCursor {
			rows = append(rows, accentStyle.Render("[x] "+row))
		} else {
			rows = append(rows, mutedStyle.Render("[ ] "+row))
		}
	}

	editHint := "enter: edit"
	if m.ConfigCursor == 1 {
		editHint = "enter/space: toggle"
	}
	var hints string
	if m.ConfigDiscardPrompt {
		hints = "\n" + warnStyle.Render("Unsaved changes — s: save  •  backspace/esc: discard")
	} else {
		hints = "\n" + subtle("j/k") + dimStyle.Render(": navigate") + dot +
			subtle(editHint) + dimStyle.Render("") + dot +
			subtle("s") + dimStyle.Render(": save") + dot +
			subtle("backspace") + dimStyle.Render(": discard")
	}
	return title + strings.Join(rows, "\n") + hints
}

func helpOverlayView(width int) string {
	rows := [][]string{
		{"j/k", "navigate"},
		{"enter", "select / confirm"},
		{"backspace", "back / cancel"},
		{"n", "name session"},
		{"p", "pause (countdown)"},
		{"tab", "cycle report mode"},
		{"x", "export CSV (report)"},
		{"?", "this help"},
		{"q", "quit"},
	}
	var lines []string
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("  %-14s %s", accentStyle.Render(r[0]), mutedStyle.Render(r[1])))
	}
	body := headerStyle.Render("Keys") + "\n\n" +
		strings.Join(lines, "\n") + "\n\n" +
		dimStyle.Render("? to close")
	panel := panelStyle.Width(36).Render(body)
	return lipgloss.Place(width, 0, lipgloss.Center, lipgloss.Top, panel)
}
