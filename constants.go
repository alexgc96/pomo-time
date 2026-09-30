package main

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

const (
	progressBarWidth  = 71
	progressFullChar  = "█"
	progressEmptyChar = "░"
)

// termenv styles — existing surfaces
var (
	term          = termenv.EnvColorProfile()
	keyword       = makeFgStyle("211")
	subtle        = makeFgStyle("241")
	progressEmpty = subtle(progressEmptyChar)
	dot           = colorFg(" • ", "236")
	ramp          = makeRamp("#B14FFF", "#00FFA3", progressBarWidth)
)

// lipgloss styles — new surfaces (dashboard, header, completion, naming, report)
var (
	panelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	mutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("79"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("236"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	focusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("79"))
	breakStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).
			Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("212")).Padding(0, 2)
)
