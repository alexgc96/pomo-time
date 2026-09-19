package main

import (
	"database/sql"
	"time"
)

type screen int

const (
	screenHome screen = iota
	screenQuick
	screenCountdown
	screenDone
	screenReport
	screenTemplatePicker
	screenTemplateManager
	screenTemplateEditor
	screenIntervalEditor
	screenNotes
	screenContinueSession
	screenSessionManager
	screenConfig
)

type model struct {
	Choice    int
	Ticks     int
	Progress  float64
	Quitting  bool
	DoneChoice int
	Paused    bool
	Options   []options
	IsCustom  bool
	SessionName  string
	FigletHeader string
	Naming    bool
	NameInput string
	StartedAt time.Time
	Width     int
	DB        *sql.DB
	Config    Config
	TodaySessions []Session
	TodayMinutes  int
	Streak        int
	ReportData    []DayReport

	// screen state
	Screen screen

	// template runtime state
	RunID           int64
	ActiveTemplate  *Template
	TemplateIntervals []TemplateInterval
	CurrentInterval int
	RunCompleted    bool

	// template browser/manager
	Templates      []Template
	TemplateCursor int

	// template editor
	EditingTemplate *Template    // nil = creating new
	EditorCursor    int          // -1 = name field, ≥0 = interval index
	EditorNaming    bool         // name input active in editor
	EditorNameInput string
	EditorDurBuf    string       // digit buffer for interval duration input
	EditorIntervals []TemplateInterval // working copy
	DeleteConfirm   bool         // waiting for 'y' to confirm template delete

	// notes
	NotesInput          string
	LastSessionID       int64
	LastSessionDuration int

	// in-countdown notes overlay
	PendingNote         string
	NotesOverlay        bool
	NotesOverlayBuf     string
	NotesOverlayDiscard bool

	// compact mode
	CompactMode bool

	// home screen
	HomeHeader string

	// continue session
	RecentNames      []string
	RecentNameCursor int

	// session manager
	AllSessions          []Session
	SessionMgrCursor     int
	SessionMgrOffset     int
	ViewingNote          bool
	DeleteSessionConfirm bool
	CleanOldConfirm      bool
	CleanOldCount        int
	CleanNoResults       bool

	// layout
	Height int

	// report
	ReportMode  int    // 0=daily 1=weekly 2=monthly
	ExportState int    // 0=off 1=picking 2=success 3=error
	ExportRange int    // 0=all 1=month 2=week 3=day
	ExportMsg   string

	// background timer
	BackgroundTimer  bool
	ActiveChoiceIdx  int  // frozen option index for the running timer (not the nav cursor)

	// notes discard confirm
	NotesConfirmDiscard bool

	// config editor
	ConfigCursor        int
	ConfigEditing       bool
	ConfigIntBuf        string
	ConfigStrBuf        string
	ConfigDiscardPrompt bool

	// help overlay
	HelpVisible bool
}

type options struct {
	Type string
	Time int
}

type tickMsg struct{}

type figletMsg struct{ header string }

type homeHeaderMsg struct{ header string }

type Session struct {
	ID          int64
	Name        string
	Type        string
	DurationMin int
	StartedAt   time.Time
	CompletedAt time.Time
	Completed   bool
	Notes       string
	RunID       sql.NullInt64
}

type DayReport struct {
	Date     string
	TotalMin int
	Count    int
	Notes    string
}

type Template struct {
	ID        int64
	Name      string
	CreatedAt time.Time
	Intervals []TemplateInterval
}

type TemplateInterval struct {
	ID          int64
	TemplateID  int64
	Position    int
	Type        string
	DurationMin int
}

type SessionRun struct {
	ID         int64
	TemplateID sql.NullInt64
	Name       string
	StartedAt  time.Time
}
