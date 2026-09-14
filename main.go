package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/indent"
	"github.com/muesli/reflow/wordwrap"
)

const version = "1.0.0"

func main() {
	var sessionName string
	var showVersion bool
	flag.StringVar(&sessionName, "s", "", "name this session (e.g. pomo -s \"Deep_Work\")")
	flag.StringVar(&sessionName, "session", "", "name this session")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&showVersion, "v", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "pomo — a minimal pomodoro timer\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  pomo [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nKeys (home screen):\n")
		fmt.Fprintf(os.Stderr, "  j/k         navigate\n")
		fmt.Fprintf(os.Stderr, "  enter       select\n")
		fmt.Fprintf(os.Stderr, "  n           name the session\n")
		fmt.Fprintf(os.Stderr, "  r           open report\n")
		fmt.Fprintf(os.Stderr, "  q           quit\n")
		fmt.Fprintf(os.Stderr, "\nKeys (quick session):\n")
		fmt.Fprintf(os.Stderr, "  0-9         type a custom duration in minutes\n")
		fmt.Fprintf(os.Stderr, "  enter       start selected timer\n")
		fmt.Fprintf(os.Stderr, "\nKeys (countdown):\n")
		fmt.Fprintf(os.Stderr, "  p           pause / resume\n")
		fmt.Fprintf(os.Stderr, "  n           rename session\n")
		fmt.Fprintf(os.Stderr, "  q           quit\n")
		fmt.Fprintf(os.Stderr, "\nKeys (done screen):\n")
		fmt.Fprintf(os.Stderr, "  m           add / edit note\n")
		fmt.Fprintf(os.Stderr, "  j/k, enter  navigate and select\n\n")
		fmt.Fprintf(os.Stderr, "Sessions are saved to ~/.pomo/sessions.db\n")
		fmt.Fprintf(os.Stderr, "Config lives at ~/.pomo/config.json\n")
	}
	flag.Parse()

	if showVersion {
		fmt.Println("pomo", version)
		os.Exit(0)
	}

	cfg := loadConfig()
	db, err := initDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not open session database: %v\n", err)
	}

	// load today's stats at startup so dashboard is populated immediately
	var todaySess []Session
	var todayMins, streak int
	if db != nil {
		todaySess, _ = todaySessions(db)
		todayMins, _ = todayFocusMinutes(db)
		streak, _ = currentStreak(db)
	}

	opts := []options{
		{"Focus", 25},
		{"Break", 5},
		{"Break", 15},
	}
	startScreen := screenHome
	if sessionName != "" {
		startScreen = screenQuick
	}
	initialModel := model{
		Choice:        0,
		Ticks:         1000,
		Options:       opts,
		SessionName:   sessionName,
		DB:            db,
		Width:         80,
		Config:        cfg,
		TodaySessions: todaySess,
		TodayMinutes:  todayMins,
		Streak:        streak,
		Screen:        startScreen,
		Achievements:  loadAchievements(),
	}
	p := tea.NewProgram(initialModel, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("could not start program:", err)
	}
}

// computeHomeHeaderCmd renders the "Pomo time!" greeting figlet once at startup.
func computeHomeHeaderCmd(width int) tea.Cmd {
	return func() tea.Msg {
		w := strconv.Itoa(max(width-6, 40))
		out, err := exec.Command("/opt/local/bin/figlet", "-w", w, "-f", "small", "Pomo time!").Output()
		if err != nil || strings.TrimSpace(string(out)) == "" {
			return homeHeaderMsg{headerStyle.Render("POMO TIME!") + "\n"}
		}
		return homeHeaderMsg{headerStyle.Render(strings.TrimRight(string(out), "\n")) + "\n"}
	}
}

// computeFigletCmd runs figlet asynchronously so it never blocks View().
func computeFigletCmd(name string, width int) tea.Cmd {
	return func() tea.Msg {
		if name == "" {
			return figletMsg{""}
		}
		w := strconv.Itoa(max(width-6, 40))
		out, err := exec.Command("/opt/local/bin/figlet", "-w", w, "-f", "small", name).Output()
		if err != nil {
			return figletMsg{titleStyle.Render(strings.ToUpper(name)) + "\n\n"}
		}
		trimmed := strings.TrimRight(string(out), "\n")
		if trimmed == "" {
			return figletMsg{titleStyle.Render(strings.ToUpper(name)) + "\n\n"}
		}
		return figletMsg{headerStyle.Render(trimmed) + "\n\n"}
	}
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

var claudeSpinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
var claudeThoughts = []string{
	"thinkin bout it",
	"hmm...",
	"consulting the void",
	"big thoughts incoming",
	"one sec",
	"let me cook",
	"almost got it",
	"neurons firing",
}

func claudeTick() tea.Cmd {
	return tea.Tick(720*time.Millisecond, func(time.Time) tea.Msg {
		return claudeTickMsg{}
	})
}

func runClaudeQuery(input, sessionName, sessionType string, thread []ClaudeMessage, id int) tea.Cmd {
	return func() tea.Msg {
		// build thread context from last 3 exchanges (6 messages)
		var ctx string
		if len(thread) > 0 {
			start := 0
			if len(thread) > 6 {
				start = len(thread) - 6
			}
			ctx = "[Session conversation so far:\n"
			for _, msg := range thread[start:] {
				role := "Q"
				if msg.Role == "assistant" {
					role = "A"
				}
				ctx += role + ": " + msg.Content + "\n"
			}
			ctx += "]\n\n"
		}
		query := ctx + input
		if sessionName != "" {
			query = fmt.Sprintf("[Working on: \"%s\"] ", sessionName) + query
		}
		query += " [non-interactive query from pomodoro app, keep response under 270 chars]"
		out, err := exec.Command("claude", "-p", query).Output()
		if err != nil {
			return claudeResponseMsg{err: err, queryID: id, sessionType: sessionType, rawQuery: input}
		}
		return claudeResponseMsg{
			text:        strings.TrimSpace(string(out)),
			queryID:     id,
			sessionType: sessionType,
			rawQuery:    input,
		}
	}
}

func runSessionSummary(sessionName, sessionType string, durationMin int, notes string) tea.Cmd {
	return func() tea.Msg {
		parts := fmt.Sprintf("type: %s, duration: %dmin", sessionType, durationMin)
		if sessionName != "" {
			parts = "name: " + sessionName + ", " + parts
		}
		if notes != "" {
			parts += ", notes: " + notes
		}
		prompt := "One punchy sentence summarising this work session — " + parts +
			" [non-interactive, max 120 chars, no quotes, no leading 'You']"
		out, err := exec.Command("claude", "-p", prompt).Output()
		if err != nil {
			return claudeSummaryMsg{}
		}
		return claudeSummaryMsg{text: strings.TrimSpace(string(out))}
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick(), computeHomeHeaderCmd(m.Width)}
	if m.SessionName != "" {
		cmds = append(cmds, computeFigletCmd(m.SessionName, m.Width))
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// async figlet result
	if msg, ok := msg.(figletMsg); ok {
		m.FigletHeader = msg.header
		return m, nil
	}

	// home screen header
	if msg, ok := msg.(homeHeaderMsg); ok {
		m.HomeHeader = msg.header
		return m, nil
	}

	// claude query response arrives asynchronously
	if msg, ok := msg.(claudeResponseMsg); ok {
		if msg.queryID == m.ClaudeQueryID {
			m.ClaudeLoading = false
			if msg.err != nil {
				m.ClaudeResponse = "✗ " + msg.err.Error()
			} else {
				m.ClaudeResponse = msg.text
				// persist to DB
				saveClaudeQuery(m.DB, m.SessionName, m.ClaudeSessionType, msg.rawQuery, msg.text)
				// append to conversation thread
				m.ClaudeThread = append(m.ClaudeThread,
					ClaudeMessage{"user", msg.rawQuery},
					ClaudeMessage{"assistant", msg.text},
				)
			}
		}
		return m, nil
	}

	// end-of-session summary arrives asynchronously
	if msg, ok := msg.(claudeSummaryMsg); ok {
		m.SessionSummaryLoading = false
		m.SessionSummary = msg.text
		return m, nil
	}

	// fast tick drives the loading animation
	if _, ok := msg.(claudeTickMsg); ok {
		if m.ClaudeLoading {
			m.ClaudeFrame++
			return m, claudeTick()
		}
		return m, nil
	}

	// track terminal dimensions — recompute home header on resize
	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		m.Width = msg.Width
		m.Height = msg.Height
		return m, computeHomeHeaderCmd(m.Width)
	}

	// tick — keep alive on all screens; route to countdown when active or backgrounded
	if _, ok := msg.(tickMsg); ok {
		if m.Screen == screenCountdown || m.BackgroundTimer {
			return updateCountdown(msg, m)
		}
		return m, tick()
	}

	if msg, ok := msg.(tea.KeyMsg); ok {
		k := msg.String()

		// ctrl+c always quits regardless of overlay state
		if k == "ctrl+c" {
			m.Quitting = true
			return m, tea.Quit
		}

		// naming prompt intercepts all keys
		if m.Naming {
			switch k {
			case "enter":
				m.SessionName = m.NameInput
				m.NameInput = ""
				m.Naming = false
				return m, computeFigletCmd(m.SessionName, m.Width)
			case "esc":
				m.NameInput = ""
				m.Naming = false
			case "backspace":
				runes := []rune(m.NameInput)
				if len(runes) > 0 {
					m.NameInput = string(runes[:len(runes)-1])
				}
			case " ":
				m.NameInput += " "
			default:
				if msg.Type == tea.KeyRunes {
					m.NameInput += string(msg.Runes)
				}
			}
			return m, nil
		}

		// in-countdown notes overlay intercepts all keys
		if m.NotesOverlay {
			switch k {
			case "ctrl+s":
				m.PendingNote = m.NotesOverlayBuf
				m.NotesOverlay = false
				m.NotesOverlayDiscard = false
				return m, tea.ClearScreen
			case "esc":
				if m.NotesOverlayBuf != m.PendingNote && !m.NotesOverlayDiscard {
					m.NotesOverlayDiscard = true
				} else {
					m.NotesOverlayBuf = m.PendingNote
					m.NotesOverlay = false
					m.NotesOverlayDiscard = false
					return m, tea.ClearScreen
				}
			case "backspace":
				m.NotesOverlayDiscard = false
				runes := []rune(m.NotesOverlayBuf)
				if len(runes) > 0 {
					m.NotesOverlayBuf = string(runes[:len(runes)-1])
				}
			case "enter":
				m.NotesOverlayDiscard = false
				m.NotesOverlayBuf += "\n"
			case " ":
				m.NotesOverlayDiscard = false
				m.NotesOverlayBuf += " "
			default:
				m.NotesOverlayDiscard = false
				if msg.Type == tea.KeyRunes {
					m.NotesOverlayBuf += string(msg.Runes)
				}
			}
			return m, nil
		}

		// claude panel intercepts all keys when open
		if m.ClaudeOpen {
			if m.ClaudeLoading {
				if k == "esc" || k == "ctrl+c" {
					m.ClaudeQueryID++ // invalidate in-flight response
					m.ClaudeLoading = false
					m.ClaudeOpen = false
					m.ClaudeInput = ""
					m.ClaudeResponse = ""
					return m, tea.ClearScreen
				}
				return m, nil
			}
			if m.ClaudeResponse != "" {
				m.ClaudeOpen = false
				m.ClaudeResponse = ""
				m.ClaudeInput = ""
				return m, tea.ClearScreen
			}
			// typing mode
			switch k {
			case "t":
				m.ClaudeThread = nil
			case "enter":
				if strings.TrimSpace(m.ClaudeInput) != "" {
					m.ClaudeLoading = true
					m.ClaudeFrame = 0
					m.ClaudeQueryID++
					return m, tea.Batch(
						runClaudeQuery(m.ClaudeInput, m.SessionName, m.ClaudeSessionType, m.ClaudeThread, m.ClaudeQueryID),
						claudeTick(),
					)
				}
			case "esc":
				m.ClaudeOpen = false
				m.ClaudeInput = ""
				return m, tea.ClearScreen
			case "backspace":
				runes := []rune(m.ClaudeInput)
				if len(runes) > 0 {
					m.ClaudeInput = string(runes[:len(runes)-1])
				}
			case " ":
				m.ClaudeInput += " "
			default:
				if msg.Type == tea.KeyRunes {
					m.ClaudeInput += string(msg.Runes)
				}
			}
			return m, nil
		}

		// q quits everywhere except text-input screens
		if k == "q" && m.Screen != screenNotes && m.Screen != screenTemplateEditor && m.Screen != screenIntervalEditor {
			m.Quitting = true
			return m, tea.Quit
		}

		// n renames session everywhere except screens that use n for something else
		if k == "n" && m.Screen != screenTemplateManager && m.Screen != screenTemplateEditor && m.Screen != screenNotes && m.Screen != screenIntervalEditor {
			m.Naming = true
			m.NameInput = m.SessionName
			return m, nil
		}

		// ? toggles help overlay (excluded from text-input screens)
		if k == "?" && m.Screen != screenNotes && m.Screen != screenTemplateEditor &&
			m.Screen != screenIntervalEditor && m.Screen != screenConfig {
			m.HelpVisible = !m.HelpVisible
			return m, nil
		}
		// any key other than ? clears help overlay
		if m.HelpVisible && k != "?" {
			m.HelpVisible = false
			return m, nil
		}

		// m opens notes overlay during countdown (not when claude panel is open)
		if k == "m" && m.Screen == screenCountdown && !m.ClaudeOpen {
			m.NotesOverlay = true
			m.NotesOverlayBuf = m.PendingNote
			m.NotesOverlayDiscard = false
			return m, nil
		}

		// c opens claude query panel during countdown (not when notes overlay is open)
		if k == "c" && m.Screen == screenCountdown && !m.NotesOverlay {
			m.ClaudeOpen = !m.ClaudeOpen
			if m.ClaudeOpen {
				// freeze current session type for DB logging
				if m.RunID > 0 && m.CurrentInterval < len(m.TemplateIntervals) {
					m.ClaudeSessionType = m.TemplateIntervals[m.CurrentInterval].Type
				} else {
					m.ClaudeSessionType = m.Options[m.ActiveChoiceIdx].Type
				}
			} else {
				m.ClaudeResponse = ""
				m.ClaudeInput = ""
			}
			return m, nil
		}

		// z toggles compact mode (not in text-input screens)
		if k == "z" && m.Screen != screenNotes && m.Screen != screenTemplateEditor &&
			m.Screen != screenIntervalEditor && m.Screen != screenConfig {
			m.CompactMode = !m.CompactMode
			return m, nil
		}

		// f finishes the current interval early, logs elapsed time
		if k == "f" && m.Screen == screenCountdown {
			var totalSecs int
			var sessType string
			if m.RunID > 0 && m.CurrentInterval < len(m.TemplateIntervals) {
				iv := m.TemplateIntervals[m.CurrentInterval]
				totalSecs = iv.DurationMin * 60
				sessType = iv.Type
			} else {
				totalSecs = m.Options[m.ActiveChoiceIdx].Time * 60
				sessType = m.Options[m.ActiveChoiceIdx].Type
			}
			elapsedMin := (totalSecs - m.Ticks) / 60
			if elapsedMin < 1 {
				elapsedMin = 1
			}
			fmt.Print("\a")
			if m.DB != nil {
				s := Session{
					Name:        m.SessionName,
					Type:        sessType,
					DurationMin: elapsedMin,
					StartedAt:   m.StartedAt,
					CompletedAt: time.Now(),
					Completed:   true,
				}
				id, _ := saveSession(m.DB, s, m.RunID)
				m.LastSessionID = id
				m.LastSessionDuration = elapsedMin
				if m.PendingNote != "" {
					_ = updateSessionNotes(m.DB, id, m.PendingNote)
					m.PendingNote = ""
				}
				if sessions, err := todaySessions(m.DB); err == nil {
					m.TodaySessions = sessions
				}
				if mins, err := todayFocusMinutes(m.DB); err == nil {
					m.TodayMinutes = mins
				}
				if streak, err := currentStreak(m.DB); err == nil {
					m.Streak = streak
				}
			}
			newAch, updatedAch := checkAchievements(m.DB, m.Achievements, m.Streak, m.TodayMinutes, true)
			m.Achievements = updatedAch
			m.NewAchievements = newAch
			m.WasEarlyFinish = true
			m.Ticks = 0
			m.Progress = 1
			m.Paused = false
			m.BackgroundTimer = false
			m.NotesOverlay = false
			m.SessionSummary = ""
			m.SessionSummaryLoading = true
			m.Screen = screenDone
			m.DoneChoice = 0
			return m, runSessionSummary(m.SessionName, sessType, elapsedMin, m.PendingNote)
		}

		// pause key only on countdown — don't spawn a new tick chain, the global loop is always alive
		if k == "p" && m.Screen == screenCountdown {
			m.Paused = !m.Paused
			return m, nil
		}

		switch m.Screen {
		case screenHome:
			return updateHome(msg, m)
		case screenQuick:
			return updateQuick(msg, m)
		case screenCountdown:
			return updateCountdown(msg, m)
		case screenDone:
			return updateDone(msg, m)
		case screenReport:
			return updateReport(msg, m)
		case screenTemplatePicker:
			return updateTemplatePicker(msg, m)
		case screenTemplateManager:
			return updateTemplateManager(msg, m)
		case screenTemplateEditor:
			return updateTemplateEditor(msg, m)
		case screenIntervalEditor:
			return updateIntervalEditor(msg, m)
		case screenNotes:
			return updateNotes(msg, m)
		case screenContinueSession:
			return updateContinueSession(msg, m)
		case screenSessionManager:
			return updateSessionManager(msg, m)
		case screenConfig:
			return updateConfig(msg, m)
		}
	}

	return m, nil
}

func (m model) View() string {
	if m.Quitting {
		return "\n"
	}

	// help overlay replaces everything
	if m.HelpVisible {
		return "\n" + helpOverlayView(m.Width) + "\n"
	}

	var body string
	switch m.Screen {
	case screenHome:
		naming := ""
		if m.Naming {
			naming = "\n\n" + indent.String(namingView(m), 2)
		}
		return "\n" + homeView(m) + naming + "\n\n"
	case screenQuick:
		body = optionsView(m)
	case screenCountdown:
		if m.CompactMode {
			return compactCountdownView(m)
		}
		body = countdownView(m)
	case screenDone:
		return indent.String("\n"+completionMenuView(m)+"\n\n", 2)
	case screenReport:
		rv := reportView(m.ReportData, m.Width, m.ReportMode)
		extra := ""
		switch m.ExportState {
		case 1:
			ranges := []string{"All time", "This month", "This week", "Today"}
			var rrows []string
			for i, r := range ranges {
				if i == m.ExportRange {
					rrows = append(rrows, accentStyle.Render("[x] "+r))
				} else {
					rrows = append(rrows, mutedStyle.Render("[ ] "+r))
				}
			}
			extra = "\n\n" + panelStyle.Render(
				headerStyle.Render("Export CSV")+"\n\n"+
					strings.Join(rrows, "\n")+"\n\n"+
					dimStyle.Render("enter: export  •  backspace: cancel"),
			)
		case 2:
			extra = "\n\n" + accentStyle.Render(m.ExportMsg)
		case 3:
			extra = "\n\n" + warnStyle.Render(m.ExportMsg)
		}
		return indent.String("\n"+rv+extra+"\n\n", 2)
	case screenTemplatePicker:
		body = templatePickerView(m)
	case screenTemplateManager:
		body = templateManagerView(m)
	case screenTemplateEditor:
		body = templateEditorView(m)
	case screenIntervalEditor:
		body = intervalEditorView(m)
	case screenNotes:
		body = notesView(m)
	case screenContinueSession:
		body = continueSessionView(m)
	case screenSessionManager:
		body = sessionManagerView(m)
	case screenConfig:
		body = configView(m)
	default:
		return "\n" + homeView(m) + "\n\n"
	}

	naming := ""
	if m.Naming {
		naming = "\n\n" + namingView(m)
	}

	return indent.String("\n"+m.FigletHeader+body+naming+"\n\n", 2)
}

func updateHome(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	maxChoice := 6
	if m.BackgroundTimer {
		maxChoice = 7
	}
	switch keyMsg.String() {
	case "j", "down":
		m.Choice = min(m.Choice+1, maxChoice)
	case "k", "up":
		m.Choice = max(m.Choice-1, 0)
	case "enter":
		// choice 0 = resume when timer is backgrounded; otherwise shift indices
		effectiveChoice := m.Choice
		if m.BackgroundTimer {
			if m.Choice == 0 {
				m.BackgroundTimer = false
				m.Screen = screenCountdown
				return m, nil
			}
			effectiveChoice = m.Choice - 1
		}
		switch effectiveChoice {
		case 0: // Quick session
			m.Screen = screenQuick
			m.Choice = 0
		case 1: // From template
			if m.DB != nil {
				m.Templates, _ = listTemplates(m.DB)
			}
			m.Screen = screenTemplatePicker
			m.TemplateCursor = 0
		case 2: // Continue session
			if m.DB != nil {
				m.RecentNames, _ = listRecentNames(m.DB)
			}
			m.RecentNameCursor = 0
			m.Screen = screenContinueSession
		case 3: // Report
			if m.DB != nil {
				m.ReportData, _ = reportData(m.DB)
			}
			m.Screen = screenReport
		case 4: // Sessions
			if m.DB != nil {
				m.AllSessions, _ = listAllSessions(m.DB)
			}
			m.SessionMgrCursor = 0
			m.SessionMgrOffset = 0
			m.ViewingNote = false
			m.DeleteSessionConfirm = false
			m.CleanOldConfirm = false
			m.Screen = screenSessionManager
		case 5: // Manage templates
			if m.DB != nil {
				m.Templates, _ = listTemplates(m.DB)
			}
			m.Screen = screenTemplateManager
			m.TemplateCursor = 0
		case 6: // Config
			m.ConfigCursor = 0
			m.ConfigEditing = false
			m.ConfigIntBuf = fmt.Sprintf("%d", m.Config.OldSessionDays)
			m.ConfigStrBuf = m.Config.CSVExportPath
			m.ConfigDiscardPrompt = false
			m.Screen = screenConfig
		}
	case "r":
		if m.DB != nil {
			m.ReportData, _ = reportData(m.DB)
		}
		m.Screen = screenReport
	case "q", "esc", "ctrl+c":
		m.Quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func updateQuick(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.Choice++
			if m.Choice > len(m.Options)-1 {
				m.Choice = len(m.Options) - 1
			}
		case "k", "up":
			m.Choice--
			if m.Choice < 0 {
				m.Choice = 0
			}
		case "1", "2", "3", "4", "5", "6", "7", "8", "9", "0":
			if m.Options[len(m.Options)-1].Type == "Custom" {
				m.Options[len(m.Options)-1].Time = m.Options[len(m.Options)-1].Time*10 + int(msg.String()[0]-48)
				m.Choice = len(m.Options) - 1
				m.IsCustom = true
				break
			}
			m.Options = append(m.Options, options{"Custom", int(msg.String()[0] - 48)})
			m.Choice = len(m.Options) - 1
			m.IsCustom = true

		case "backspace":
			if m.IsCustom {
				m.Options[len(m.Options)-1].Time = m.Options[len(m.Options)-1].Time / 10
				if m.Options[len(m.Options)-1].Time == 0 {
					m.Options = m.Options[:len(m.Options)-1]
					m.Choice = len(m.Options) - 1
					m.IsCustom = false
				}
			} else {
				m.Screen = screenHome
				m.Choice = 0
			}

		case "enter":
			m.Screen = screenCountdown
			m.RunID = 0
			m.ActiveTemplate = nil
			m.TemplateIntervals = nil
			m.ActiveChoiceIdx = m.Choice
			m.LastSessionDuration = 0
			m.ClaudeThread = nil
			m.NewAchievements = nil
			m.SessionSummary = ""
			m.Ticks = m.Options[m.Choice].Time * 60
			m.StartedAt = time.Now()
			if m.DB != nil {
				if sessions, err := todaySessions(m.DB); err == nil {
					m.TodaySessions = sessions
				}
				if mins, err := todayFocusMinutes(m.DB); err == nil {
					m.TodayMinutes = mins
				}
				if s, err := currentStreak(m.DB); err == nil {
					m.Streak = s
				}
			}
			return m, nil

		case "esc":
			m.Screen = screenHome
			m.Choice = 0

		case "q", "ctrl+c":
			m.Quitting = true
			return m, tea.Quit

		case "n":
			m.Naming = true
			m.NameInput = m.SessionName

		case "r":
			if m.DB != nil {
				m.ReportData, _ = reportData(m.DB)
			}
			m.Screen = screenReport
		}
	}
	return m, nil
}

func updateCountdown(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		k := msg.String()
		if k == "backspace" {
			m.BackgroundTimer = true
			m.NotesOverlay = false
			m.ClaudeOpen = false
			m.Screen = screenHome
			m.Choice = 0
			return m, nil
		}
		return m, nil
	case tickMsg:
		if m.Paused {
			return m, tick()
		}
		if m.Ticks == 0 {
			fmt.Print("\a")
			m.BackgroundTimer = false
			if m.DB != nil {
				var sessType string
				var sessDur int
				if m.RunID > 0 && m.CurrentInterval < len(m.TemplateIntervals) {
					sessType = m.TemplateIntervals[m.CurrentInterval].Type
					sessDur = m.TemplateIntervals[m.CurrentInterval].DurationMin
				} else {
					sessType = m.Options[m.ActiveChoiceIdx].Type
					sessDur = m.Options[m.ActiveChoiceIdx].Time
				}
				s := Session{
					Name:        m.SessionName,
					Type:        sessType,
					DurationMin: sessDur,
					StartedAt:   m.StartedAt,
					CompletedAt: time.Now(),
					Completed:   true,
				}
				id, _ := saveSession(m.DB, s, m.RunID)
				m.LastSessionID = id
				m.LastSessionDuration = sessDur
				if m.PendingNote != "" {
					_ = updateSessionNotes(m.DB, id, m.PendingNote)
					m.PendingNote = ""
				}
				if sessions, err := todaySessions(m.DB); err == nil {
					m.TodaySessions = sessions
				}
				if mins, err := todayFocusMinutes(m.DB); err == nil {
					m.TodayMinutes = mins
				}
				if streak, err := currentStreak(m.DB); err == nil {
					m.Streak = streak
				}
			}
			newAch, updatedAch := checkAchievements(m.DB, m.Achievements, m.Streak, m.TodayMinutes, false)
			m.Achievements = updatedAch
			m.NewAchievements = newAch
			m.WasEarlyFinish = false
			m.SessionSummary = ""
			m.SessionSummaryLoading = true
			m.Screen = screenDone
			m.DoneChoice = 0
			var st, sn, pn string
			var sd int
			if m.RunID > 0 && m.CurrentInterval < len(m.TemplateIntervals) {
				st = m.TemplateIntervals[m.CurrentInterval].Type
				sd = m.TemplateIntervals[m.CurrentInterval].DurationMin
			} else {
				st = m.Options[m.ActiveChoiceIdx].Type
				sd = m.Options[m.ActiveChoiceIdx].Time
			}
			sn = m.SessionName
			pn = m.PendingNote
			return m, runSessionSummary(sn, st, sd, pn)
		}
		m.Ticks--
		if m.RunID > 0 && m.CurrentInterval < len(m.TemplateIntervals) {
			totalTicks := m.TemplateIntervals[m.CurrentInterval].DurationMin * 60
			m.Progress = 1 - float64(m.Ticks)/float64(totalTicks)
		} else {
			m.Progress = 1 - float64(m.Ticks)/float64(m.Options[m.ActiveChoiceIdx].Time*60)
		}
		return m, tick()
	}
	return m, nil
}

func updateDone(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	k := keyMsg.String()

	// 'm' to add/edit note — pre-load existing DB note (PendingNote already written at expiry)
	if k == "m" {
		m.NotesInput = ""
		if m.DB != nil && m.LastSessionID != 0 {
			var existing sql.NullString
			m.DB.QueryRow("SELECT notes FROM sessions WHERE id = ?", m.LastSessionID).Scan(&existing) //nolint
			if existing.Valid {
				m.NotesInput = existing.String
			}
		}
		m.Screen = screenNotes
		return m, nil
	}

	if m.RunID > 0 {
		next := m.CurrentInterval + 1
		isRunComplete := next >= len(m.TemplateIntervals)

		if isRunComplete {
			// run complete: call completeSessionRun once
			if !m.RunCompleted {
				if m.DB != nil {
					_ = completeSessionRun(m.DB, m.RunID)
				}
				m.RunCompleted = true
			}
			maxChoice := 1
			switch k {
			case "j", "down":
				m.DoneChoice = min(m.DoneChoice+1, maxChoice)
			case "k", "up":
				m.DoneChoice = max(m.DoneChoice-1, 0)
			case "enter":
				switch m.DoneChoice {
				case 0: // Back to home
					m.Screen = screenHome
					m.RunID = 0
					m.ActiveTemplate = nil
					m.TemplateIntervals = nil
					m.CurrentInterval = 0
					m.RunCompleted = false
					m.DoneChoice = 0
					m.Progress = 0
					m.Paused = false
				case 1: // Quit
					return m, tea.Quit
				}
			case "q", "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		}

		// run in progress — 4 choices
		maxChoice := 3
		switch k {
		case "j", "down":
			m.DoneChoice = min(m.DoneChoice+1, maxChoice)
		case "k", "up":
			m.DoneChoice = max(m.DoneChoice-1, 0)
		case "enter":
			choice := m.DoneChoice
			m.DoneChoice = 0
			m.Progress = 0
			m.Paused = false
			m.LastSessionDuration = 0
			switch choice {
			case 0: // Next interval
				m.CurrentInterval++
				iv := m.TemplateIntervals[m.CurrentInterval]
				m.Ticks = iv.DurationMin * 60
				m.StartedAt = time.Now()
				m.Screen = screenCountdown
				return m, tick()
			case 1: // Skip to next (same as next)
				m.CurrentInterval++
				iv := m.TemplateIntervals[m.CurrentInterval]
				m.Ticks = iv.DurationMin * 60
				m.StartedAt = time.Now()
				m.Screen = screenCountdown
				return m, tick()
			case 2: // End run early
				if m.DB != nil {
					_ = abortSessionRun(m.DB, m.RunID)
				}
				m.RunID = 0
				m.ActiveTemplate = nil
				m.TemplateIntervals = nil
				m.CurrentInterval = 0
				m.RunCompleted = false
				m.Screen = screenHome
				m.Choice = 0
			case 3: // Quit
				return m, tea.Quit
			}
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}

	// Quick session — 5 choices: Focus again, Break 5, Long break 15, Back to home, Quit
	maxChoice := 4
	switch k {
	case "j", "down":
		m.DoneChoice = min(m.DoneChoice+1, maxChoice)
	case "k", "up":
		m.DoneChoice = max(m.DoneChoice-1, 0)
	case "enter":
		choice := m.DoneChoice
		m.DoneChoice = 0
		m.Progress = 0
		m.Paused = false
		m.LastSessionDuration = 0
		switch choice {
		case 0: // Focus again
			m.Screen = screenCountdown
			m.Ticks = m.Options[m.ActiveChoiceIdx].Time * 60
			m.StartedAt = time.Now()
			return m, tick()
		case 1: // Break 5
			m.Choice = 1
			m.ActiveChoiceIdx = 1
			m.Screen = screenCountdown
			m.Ticks = m.Options[1].Time * 60
			m.StartedAt = time.Now()
			return m, tick()
		case 2: // Long break 15
			m.Choice = 2
			m.ActiveChoiceIdx = 2
			m.Screen = screenCountdown
			m.Ticks = m.Options[2].Time * 60
			m.StartedAt = time.Now()
			return m, tick()
		case 3: // Back to home
			m.Screen = screenHome
			m.Choice = 0
		case 4: // Quit
			return m, tea.Quit
		}
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func updateReport(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	k := keyMsg.String()

	// export range picker
	if m.ExportState == 1 {
		switch k {
		case "j", "down":
			m.ExportRange = min(m.ExportRange+1, 3)
		case "k", "up":
			m.ExportRange = max(m.ExportRange-1, 0)
		case "enter":
			days := []int{0, 30, 7, 1}
			outPath, err := exportToCSV(m.DB, m.Config.CSVExportPath, days[m.ExportRange])
			if err != nil {
				m.ExportMsg = "✗ Export failed: " + err.Error()
				m.ExportState = 3
			} else {
				m.ExportMsg = "✓ Exported to " + outPath
				m.ExportState = 2
			}
		case "backspace", "esc":
			m.ExportState = 0
		}
		return m, nil
	}

	switch k {
	case "backspace", "r", "esc":
		m.ExportState = 0
		m.ExportMsg = ""
		m.Screen = screenHome
	case "x":
		if m.Config.CSVExportPath == "" {
			m.ExportMsg = "Set csv_export_path in Config first"
			m.ExportState = 3
		} else {
			m.ExportState = 1
			m.ExportRange = 0
		}
	case "tab":
		m.ExportState = 0
		m.ExportMsg = ""
		m.ReportMode = (m.ReportMode + 1) % 3
		if m.DB != nil {
			switch m.ReportMode {
			case 0:
				m.ReportData, _ = reportData(m.DB)
			case 1:
				m.ReportData, _ = reportDataWeekly(m.DB)
			case 2:
				m.ReportData, _ = reportDataMonthly(m.DB)
			}
		}
	case "q", "ctrl+c":
		m.Quitting = true
		return m, tea.Quit
	default:
		// clear flash message on any navigation key
		if m.ExportState == 2 || m.ExportState == 3 {
			m.ExportState = 0
			m.ExportMsg = ""
		}
	}
	return m, nil
}

func updateTemplatePicker(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "j", "down":
		if len(m.Templates) > 0 {
			m.TemplateCursor = min(m.TemplateCursor+1, len(m.Templates)-1)
		}
	case "k", "up":
		m.TemplateCursor = max(m.TemplateCursor-1, 0)
	case "enter":
		if len(m.Templates) == 0 {
			return m, nil
		}
		tmpl, err := loadTemplate(m.DB, m.Templates[m.TemplateCursor].ID)
		if err != nil || len(tmpl.Intervals) == 0 {
			return m, nil
		}
		runID, _ := startSessionRun(m.DB, tmpl)
		m.RunID = runID
		m.ActiveTemplate = tmpl
		m.TemplateIntervals = tmpl.Intervals
		m.CurrentInterval = 0
		m.RunCompleted = false
		iv := tmpl.Intervals[0]
		m.Ticks = iv.DurationMin * 60
		m.StartedAt = time.Now()
		if m.SessionName == "" {
			m.SessionName = tmpl.Name
		}
		m.Screen = screenCountdown
		m.DoneChoice = 0
		m.Progress = 0
		return m, computeFigletCmd(m.SessionName, m.Width)
	case "e":
		if len(m.Templates) == 0 {
			return m, nil
		}
		tmpl, err := loadTemplate(m.DB, m.Templates[m.TemplateCursor].ID)
		if err != nil {
			return m, nil
		}
		m.EditingTemplate = tmpl
		m.EditorNameInput = tmpl.Name
		m.EditorIntervals = make([]TemplateInterval, len(tmpl.Intervals))
		copy(m.EditorIntervals, tmpl.Intervals)
		m.EditorCursor = 0
		if len(m.EditorIntervals) == 0 {
			m.EditorCursor = -1
		}
		m.EditorNaming = false
		m.EditorDurBuf = ""
		m.Screen = screenTemplateEditor
	case "n":
		m.Naming = true
		m.NameInput = m.SessionName
	case "backspace", "esc":
		m.Screen = screenHome
	case "q", "ctrl+c":
		m.Quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func updateTemplateManager(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	k := keyMsg.String()

	if m.DeleteConfirm {
		switch k {
		case "y":
			if len(m.Templates) > 0 {
				_ = deleteTemplate(m.DB, m.Templates[m.TemplateCursor].ID)
				m.Templates, _ = listTemplates(m.DB)
				if m.TemplateCursor >= len(m.Templates) && m.TemplateCursor > 0 {
					m.TemplateCursor--
				}
			}
			m.DeleteConfirm = false
		case "esc", "n":
			m.DeleteConfirm = false
		}
		return m, nil
	}

	switch k {
	case "j", "down":
		if len(m.Templates) > 0 {
			m.TemplateCursor = min(m.TemplateCursor+1, len(m.Templates)-1)
		}
	case "k", "up":
		m.TemplateCursor = max(m.TemplateCursor-1, 0)
	case "enter", "e":
		if len(m.Templates) == 0 {
			return m, nil
		}
		tmpl, err := loadTemplate(m.DB, m.Templates[m.TemplateCursor].ID)
		if err != nil {
			return m, nil
		}
		m.EditingTemplate = tmpl
		m.EditorNameInput = tmpl.Name
		m.EditorIntervals = make([]TemplateInterval, len(tmpl.Intervals))
		copy(m.EditorIntervals, tmpl.Intervals)
		m.EditorCursor = 0
		if len(m.EditorIntervals) == 0 {
			m.EditorCursor = -1
		}
		m.EditorNaming = false
		m.EditorDurBuf = ""
		m.Screen = screenTemplateEditor
	case "n":
		m.EditingTemplate = nil
		m.EditorIntervals = nil
		m.EditorNameInput = ""
		m.EditorCursor = -1
		m.EditorNaming = false
		m.EditorDurBuf = ""
		m.Screen = screenTemplateEditor
	case "d":
		if len(m.Templates) > 0 {
			m.DeleteConfirm = true
		}
	case "backspace", "esc":
		m.Screen = screenHome
	case "q", "ctrl+c":
		m.Quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func updateTemplateEditor(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	k := keyMsg.String()

	if m.EditorNaming {
		switch k {
		case "enter":
			m.EditorNaming = false
		case "esc":
			m.EditorNaming = false
		case "backspace":
			runes := []rune(m.EditorNameInput)
			if len(runes) > 0 {
				m.EditorNameInput = string(runes[:len(runes)-1])
			}
		case " ":
			m.EditorNameInput += " "
		default:
			if keyMsg.Type == tea.KeyRunes {
				m.EditorNameInput += string(keyMsg.Runes)
			}
		}
		return m, nil
	}

	switch k {
	case "j", "down":
		if len(m.EditorIntervals) > 0 {
			m.EditorCursor = min(m.EditorCursor+1, len(m.EditorIntervals)-1)
		}
	case "k", "up":
		if m.EditorCursor > 0 {
			m.EditorCursor--
		}
	case "n":
		m.EditorNaming = true
	case "a":
		m.EditorIntervals = append(m.EditorIntervals, TemplateInterval{Type: "Focus", DurationMin: 25})
		m.EditorCursor = len(m.EditorIntervals) - 1
	case "d":
		if len(m.EditorIntervals) > 0 && m.EditorCursor >= 0 {
			m.EditorIntervals = append(m.EditorIntervals[:m.EditorCursor], m.EditorIntervals[m.EditorCursor+1:]...)
			if m.EditorCursor >= len(m.EditorIntervals) {
				m.EditorCursor = len(m.EditorIntervals) - 1
			}
			if len(m.EditorIntervals) == 0 {
				m.EditorCursor = -1
			}
		}
	case "enter":
		if m.EditorCursor >= 0 && m.EditorCursor < len(m.EditorIntervals) {
			m.EditorDurBuf = ""
			m.Screen = screenIntervalEditor
		}
	case "s":
		if m.DB != nil {
			name := m.EditorNameInput
			if name == "" {
				name = "Unnamed"
			}
			if m.EditingTemplate != nil {
				_ = updateTemplate(m.DB, m.EditingTemplate.ID, name, m.EditorIntervals)
			} else {
				_, _ = createTemplate(m.DB, name, m.EditorIntervals)
			}
			m.Templates, _ = listTemplates(m.DB)
		}
		m.Screen = screenTemplateManager
	case "backspace", "esc":
		m.Screen = screenTemplateManager
	case "q", "ctrl+c":
		m.Quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func updateIntervalEditor(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	k := keyMsg.String()

	if m.EditorCursor < 0 || m.EditorCursor >= len(m.EditorIntervals) {
		m.Screen = screenTemplateEditor
		return m, nil
	}

	switch k {
	case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if len(m.EditorDurBuf) < 3 {
			m.EditorDurBuf += k
		}
	case "backspace":
		runes := []rune(m.EditorDurBuf)
		if len(runes) > 0 {
			m.EditorDurBuf = string(runes[:len(runes)-1])
		} else {
			m.Screen = screenTemplateEditor
		}
	case "t":
		if m.EditorIntervals[m.EditorCursor].Type == "Focus" {
			m.EditorIntervals[m.EditorCursor].Type = "Break"
		} else {
			m.EditorIntervals[m.EditorCursor].Type = "Focus"
		}
	case "enter":
		if m.EditorDurBuf != "" {
			dur, err := strconv.Atoi(m.EditorDurBuf)
			if err == nil && dur > 0 {
				m.EditorIntervals[m.EditorCursor].DurationMin = dur
			}
		}
		m.EditorDurBuf = ""
		m.Screen = screenTemplateEditor
	case "esc":
		m.EditorDurBuf = ""
		m.Screen = screenTemplateEditor
	}
	return m, nil
}

func updateNotes(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	k := keyMsg.String()
	switch k {
	case "ctrl+s":
		if m.DB != nil && m.LastSessionID != 0 {
			_ = updateSessionNotes(m.DB, m.LastSessionID, m.NotesInput)
		}
		m.NotesInput = ""
		m.NotesConfirmDiscard = false
		m.Screen = screenDone
	case "esc":
		if m.NotesInput != "" && !m.NotesConfirmDiscard {
			m.NotesConfirmDiscard = true
			return m, nil
		}
		m.NotesInput = ""
		m.NotesConfirmDiscard = false
		m.Screen = screenDone
	case "enter":
		m.NotesInput += "\n"
	case "backspace":
		m.NotesConfirmDiscard = false
		runes := []rune(m.NotesInput)
		if len(runes) > 0 {
			m.NotesInput = string(runes[:len(runes)-1])
		}
	case " ":
		m.NotesConfirmDiscard = false
		m.NotesInput += " "
	default:
		m.NotesConfirmDiscard = false
		if keyMsg.Type == tea.KeyRunes {
			m.NotesInput += string(keyMsg.Runes)
		}
	}
	return m, nil
}

func optionsView(m model) string {
	c := m.Choice
	tpl := "Time to focus?\n\n"
	for i := 0; i < len(m.Options); i++ {
		tpl += fmt.Sprintf("%s\n", checkbox(fmt.Sprintf("%s — %d mins", m.Options[i].Type, m.Options[i].Time), c == i))
	}
	tpl += "\n" + subtle("tip: type any number for a custom duration") + "\n"
	tpl += "\n" + subtle("j/k") + dimStyle.Render(": navigate") + dot +
		subtle("enter") + dimStyle.Render(": start") + dot +
		subtle("n") + dimStyle.Render(": name") + dot +
		subtle("r") + dimStyle.Render(": report") + dot +
		subtle("backspace") + dimStyle.Render(": back") + dot +
		subtle("q") + dimStyle.Render(": quit")
	return tpl
}

func countdownView(m model) string {
	var label string
	var totalDur int
	if m.RunID > 0 && m.CurrentInterval < len(m.TemplateIntervals) {
		iv := m.TemplateIntervals[m.CurrentInterval]
		label = fmt.Sprintf("%s — %s mins — Interval %d of %d\n",
			intervalTypeStyle(iv.Type),
			keyword(strconv.Itoa(iv.DurationMin)),
			m.CurrentInterval+1,
			len(m.TemplateIntervals),
		)
		totalDur = iv.DurationMin
	} else {
		label = fmt.Sprintf("%s — %s mins\n", intervalTypeStyle(m.Options[m.ActiveChoiceIdx].Type), keyword(strconv.Itoa(m.Options[m.ActiveChoiceIdx].Time)))
		totalDur = m.Options[m.ActiveChoiceIdx].Time
	}

	var timeLeft string
	if m.Ticks > 60 {
		mins := m.Ticks / 60
		secs := m.Ticks % 60
		timeLeft = fmt.Sprintf("Time remaining %s min %s sec\n\n",
			colorFg(strconv.Itoa(mins), "79"),
			colorFg(fmt.Sprintf("%02d", secs), "79"),
		)
	} else {
		timeLeft = fmt.Sprintf("Time remaining %s sec\n\n", colorFg(strconv.Itoa(m.Ticks), "79"))
	}

	_ = totalDur // used for progress calc in updateCountdown
	bar := progressbar(m.Progress) + "%\n"

	pauseIndicator := ""
	if m.Paused {
		pauseIndicator = "\n" + mutedStyle.Render("⏸  paused")
	}

	dash := ""
	if m.DB != nil {
		var curOpt options
		if m.RunID > 0 && m.CurrentInterval < len(m.TemplateIntervals) {
			iv := m.TemplateIntervals[m.CurrentInterval]
			curOpt = options{Type: iv.Type, Time: iv.DurationMin}
		} else {
			curOpt = m.Options[m.Choice]
		}
		dash = "\n" + dashboardPanel(m.TodaySessions, curOpt, m.SessionName, m.TodayMinutes, m.Streak, m.Width)
	}

	notePreview := ""
	if m.PendingNote != "" && !m.NotesOverlay {
		preview := m.PendingNote
		if len([]rune(preview)) > 60 {
			preview = string([]rune(preview)[:59]) + "…"
		}
		preview = strings.ReplaceAll(preview, "\n", " ↵ ")
		notePreview = "\n" + mutedStyle.Render("✎ "+preview)
	}

	notesOverlayPanel := ""
	if m.NotesOverlay {
		hint := dimStyle.Render("ctrl+s: save  •  esc: cancel")
		if m.NotesOverlayDiscard {
			hint = warnStyle.Render("Unsaved — esc again to discard")
		}
		notesOverlayPanel = "\n\n" + panelStyle.Render(
			headerStyle.Render("Note")+"  "+dimStyle.Render("(saved with session)")+"\n\n"+
				m.NotesOverlayBuf+"▌\n\n"+hint,
		)
	}

	claudePanel := ""
	if m.ClaudeOpen {
		innerW := m.Width - 10
		if innerW < 50 {
			innerW = 50
		}
		if innerW > 90 {
			innerW = 90
		}
		var content string
		threadInfo := ""
		if len(m.ClaudeThread) > 0 {
			threadInfo = "  " + dimStyle.Render(fmt.Sprintf("· %d msg thread  t: clear", len(m.ClaudeThread)/2))
		}
		if m.ClaudeLoading {
			spinner := claudeSpinner[m.ClaudeFrame%len(claudeSpinner)]
			thought := claudeThoughts[(m.ClaudeFrame/3)%len(claudeThoughts)]
			loadLine := accentStyle.Render(spinner) + "  " + mutedStyle.Render(thought)
			content = headerStyle.Render("Ask Claude") + threadInfo + "\n\n" +
				accentStyle.Render("> ") + dimStyle.Render(m.ClaudeInput) + "\n\n" +
				loadLine
		} else if m.ClaudeResponse != "" {
			wrapped := wordwrap.String(m.ClaudeResponse, innerW-4)
			content = headerStyle.Render("Claude") + threadInfo + "\n\n" +
				wrapped + "\n\n" +
				dimStyle.Render("any key: dismiss  •  c: follow-up")
		} else {
			content = headerStyle.Render("Ask Claude") + threadInfo + "\n\n" +
				m.ClaudeInput + "▌\n\n" +
				dimStyle.Render("enter: ask  •  t: clear thread  •  esc: cancel")
		}
		claudePanel = "\n\n" + panelStyle.Width(innerW).Render(content)
	}

	backHint := subtle("backspace") + dimStyle.Render(": background")
	hints := "\n" + subtle("p")+dimStyle.Render(": pause") + dot +
		subtle("f")+dimStyle.Render(": finish early") + dot +
		subtle("c")+dimStyle.Render(": ask claude") + dot +
		subtle("m")+dimStyle.Render(": note") + dot +
		subtle("z")+dimStyle.Render(": compact") + dot +
		backHint + dot +
		subtle("q")+dimStyle.Render(": quit")

	return label + "\n" + timeLeft + bar + pauseIndicator + notePreview + notesOverlayPanel + claudePanel + dash + hints
}

func compactCountdownView(m model) string {
	var typLabel, timeStr string
	if m.RunID > 0 && m.CurrentInterval < len(m.TemplateIntervals) {
		iv := m.TemplateIntervals[m.CurrentInterval]
		typLabel = intervalTypeStyle(iv.Type)
	} else {
		typLabel = intervalTypeStyle(m.Options[m.ActiveChoiceIdx].Type)
	}
	mins := m.Ticks / 60
	secs := m.Ticks % 60
	timeStr = accentStyle.Render(fmt.Sprintf("%02d:%02d", mins, secs))

	pct := int(m.Progress * 100)
	barFull := pct * 10 / 100
	if barFull > 10 {
		barFull = 10
	}
	bar := accentStyle.Render(strings.Repeat("█", barFull)) +
		dimStyle.Render(strings.Repeat("░", 10-barFull)) +
		fmt.Sprintf(" %d%%", pct)

	pause := ""
	if m.Paused {
		pause = "  " + mutedStyle.Render("⏸")
	}

	noteIndicator := ""
	if m.PendingNote != "" {
		noteIndicator = "  " + mutedStyle.Render("✎")
	}

	line1 := typLabel + "  " + timeStr + pause + noteIndicator
	line2 := bar
	hint := dimStyle.Render("z: expand  p: pause  f: finish  m: note  q: quit")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("238")).
		Padding(0, 1).
		Render(line1 + "\n" + line2 + "\n" + hint)

	centered := "\n" + lipgloss.NewStyle().Width(m.Width).Align(lipgloss.Center).Render(box) + "\n"

	if m.NotesOverlay {
		overlayHint := dimStyle.Render("ctrl+s: save  •  esc: cancel")
		if m.NotesOverlayDiscard {
			overlayHint = warnStyle.Render("Unsaved — esc again to discard")
		}
		overlay := panelStyle.Render(
			headerStyle.Render("Note") + "\n\n" +
				m.NotesOverlayBuf + "▌\n\n" + overlayHint,
		)
		return centered + "\n" + indent.String(overlay, 2) + "\n"
	}

	if m.ClaudeOpen {
		var content string
		if m.ClaudeLoading {
			spinner := claudeSpinner[m.ClaudeFrame%len(claudeSpinner)]
			thought := claudeThoughts[(m.ClaudeFrame/3)%len(claudeThoughts)]
			content = headerStyle.Render("Ask Claude") + "\n\n" +
				accentStyle.Render(spinner) + "  " + mutedStyle.Render(thought)
		} else if m.ClaudeResponse != "" {
			wrapped := wordwrap.String(m.ClaudeResponse, 60)
			content = headerStyle.Render("Claude") + "\n\n" + wrapped + "\n\n" + dimStyle.Render("any key: dismiss")
		} else {
			content = headerStyle.Render("Ask Claude") + "\n\n" +
				m.ClaudeInput + "▌\n\n" + dimStyle.Render("enter: ask  •  esc: cancel")
		}
		return centered + "\n" + indent.String(panelStyle.Render(content), 2) + "\n"
	}

	return centered
}

func updateContinueSession(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "j", "down":
		if len(m.RecentNames) > 0 {
			m.RecentNameCursor = min(m.RecentNameCursor+1, len(m.RecentNames)-1)
		}
	case "k", "up":
		m.RecentNameCursor = max(m.RecentNameCursor-1, 0)
	case "enter":
		if len(m.RecentNames) > 0 {
			m.SessionName = m.RecentNames[m.RecentNameCursor]
			m.Screen = screenQuick
			m.Choice = 0
			return m, computeFigletCmd(m.SessionName, m.Width)
		}
	case "backspace", "esc":
		m.Screen = screenHome
		m.Choice = 2
	}
	return m, nil
}

func updateSessionManager(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	k := keyMsg.String()

	// confirm delete selected session
	if m.DeleteSessionConfirm {
		switch k {
		case "y":
			if len(m.AllSessions) > 0 {
				_ = deleteSessionByID(m.DB, m.AllSessions[m.SessionMgrCursor].ID)
				_ = resetSequenceIfEmpty(m.DB)
				m.AllSessions, _ = listAllSessions(m.DB)
				if m.SessionMgrCursor >= len(m.AllSessions) && m.SessionMgrCursor > 0 {
					m.SessionMgrCursor--
				}
			}
			m.DeleteSessionConfirm = false
		case "esc", "n":
			m.DeleteSessionConfirm = false
		}
		return m, nil
	}

	// confirm clean old sessions
	if m.CleanOldConfirm {
		switch k {
		case "y":
			_ = cleanOldSessions(m.DB, m.Config.OldSessionDays)
			_ = resetSequenceIfEmpty(m.DB)
			m.AllSessions, _ = listAllSessions(m.DB)
			m.SessionMgrCursor = 0
			m.CleanOldConfirm = false
			m.CleanOldCount = 0
		case "esc", "n":
			m.CleanOldConfirm = false
			m.CleanOldCount = 0
		}
		return m, nil
	}

	viewH := m.Height - 10
	if viewH < 5 {
		viewH = 5
	}

	switch k {
	case "j", "down":
		m.CleanNoResults = false
		m.CleanOldCount = 0
		m.ViewingNote = false
		if len(m.AllSessions) > 0 {
			m.SessionMgrCursor = min(m.SessionMgrCursor+1, len(m.AllSessions)-1)
			if m.SessionMgrCursor >= m.SessionMgrOffset+viewH {
				m.SessionMgrOffset++
			}
		}
	case "k", "up":
		m.CleanNoResults = false
		m.CleanOldCount = 0
		m.ViewingNote = false
		m.SessionMgrCursor = max(m.SessionMgrCursor-1, 0)
		if m.SessionMgrCursor < m.SessionMgrOffset {
			m.SessionMgrOffset--
		}
	case "v":
		if len(m.AllSessions) > 0 && m.AllSessions[m.SessionMgrCursor].Notes != "" {
			m.ViewingNote = !m.ViewingNote
		}
	case "d":
		m.CleanNoResults = false
		m.CleanOldCount = 0
		m.ViewingNote = false
		if len(m.AllSessions) > 0 {
			m.DeleteSessionConfirm = true
		}
	case "c":
		m.ViewingNote = false
		if !m.Config.CleanupEnabled {
			m.CleanOldCount = -1
			m.CleanOldConfirm = false
			m.CleanNoResults = false
		} else {
			count, _ := countOldSessions(m.DB, m.Config.OldSessionDays)
			m.CleanOldCount = count
			if count > 0 {
				m.CleanOldConfirm = true
				m.CleanNoResults = false
			} else {
				m.CleanNoResults = true
			}
		}
	case "backspace", "esc":
		m.CleanNoResults = false
		m.CleanOldCount = 0
		m.ViewingNote = false
		m.Screen = screenHome
		m.Choice = 4
	}
	return m, nil
}

func updateConfig(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	k := keyMsg.String()

	if m.ConfigEditing {
		switch m.ConfigCursor {
		case 0: // old_session_days — digit buffer
			switch k {
			case "enter", "esc":
				m.ConfigEditing = false
			case "backspace":
				runes := []rune(m.ConfigIntBuf)
				if len(runes) > 0 {
					m.ConfigIntBuf = string(runes[:len(runes)-1])
				}
			default:
				if keyMsg.Type == tea.KeyRunes && len(m.ConfigIntBuf) < 4 {
					r := string(keyMsg.Runes)
					if r >= "0" && r <= "9" {
						m.ConfigIntBuf += r
					}
				}
			}
		case 2: // csv_export_path — string buffer
			switch k {
			case "enter", "esc":
				m.ConfigEditing = false
			case "backspace":
				runes := []rune(m.ConfigStrBuf)
				if len(runes) > 0 {
					m.ConfigStrBuf = string(runes[:len(runes)-1])
				}
			case " ":
				m.ConfigStrBuf += " "
			default:
				if keyMsg.Type == tea.KeyRunes {
					m.ConfigStrBuf += string(keyMsg.Runes)
				}
			}
		}
		return m, nil
	}

	switch k {
	case "j", "down":
		m.ConfigCursor = min(m.ConfigCursor+1, 2)
	case "k", "up":
		m.ConfigCursor = max(m.ConfigCursor-1, 0)
	case "enter", " ":
		switch m.ConfigCursor {
		case 0:
			m.ConfigEditing = true
		case 1:
			m.Config.CleanupEnabled = !m.Config.CleanupEnabled
		case 2:
			m.ConfigEditing = true
		}
	case "s":
		if v, err := strconv.Atoi(m.ConfigIntBuf); err == nil && v > 0 {
			m.Config.OldSessionDays = v
		}
		m.Config.CSVExportPath = m.ConfigStrBuf
		_ = saveConfig(m.Config)
		m.ConfigDiscardPrompt = false
		m.Screen = screenHome
		m.Choice = 6
	case "backspace", "esc":
		isDirty := m.ConfigIntBuf != fmt.Sprintf("%d", m.Config.OldSessionDays) ||
			m.ConfigStrBuf != m.Config.CSVExportPath
		if isDirty && !m.ConfigDiscardPrompt {
			m.ConfigDiscardPrompt = true
			return m, nil
		}
		m.ConfigDiscardPrompt = false
		m.Screen = screenHome
		m.Choice = 6
	case "q", "ctrl+c":
		m.Quitting = true
		return m, tea.Quit
	}
	return m, nil
}
