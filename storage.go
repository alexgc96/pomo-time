package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const dbTimeFormat = "2006-01-02T15:04:05"

func initDB() (*sql.DB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".pomo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", filepath.Join(dir, "sessions.db")+"?_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		name         TEXT,
		type         TEXT,
		duration_min INTEGER,
		started_at   TEXT,
		completed_at TEXT,
		completed    INTEGER
	)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS templates (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		name       TEXT NOT NULL,
		created_at TEXT
	)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS template_intervals (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		template_id  INTEGER NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
		position     INTEGER NOT NULL,
		type         TEXT NOT NULL,
		duration_min INTEGER NOT NULL
	)`)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS session_runs (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		template_id  INTEGER REFERENCES templates(id),
		name         TEXT,
		started_at   TEXT,
		completed_at TEXT,
		completed    INTEGER DEFAULT 0
	)`)
	if err != nil {
		return nil, err
	}

	// additive column migrations
	if err := ensureColumn(db, "sessions", "run_id", "INTEGER"); err != nil {
		return nil, err
	}
	if err := ensureColumn(db, "sessions", "notes", "TEXT"); err != nil {
		return nil, err
	}

	// seed default template if empty
	if err := seedDefaultTemplate(db); err != nil {
		return nil, err
	}

	return db, nil
}

func ensureColumn(db *sql.DB, table, col, colDef string) error {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk) //nolint
		if name == col {
			return nil
		}
	}
	_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, col, colDef))
	return err
}

func seedDefaultTemplate(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM templates").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	id, err := createTemplate(db, "Pomodoro", []TemplateInterval{
		{Type: "Focus", DurationMin: 25},
		{Type: "Break", DurationMin: 5},
		{Type: "Focus", DurationMin: 25},
		{Type: "Break", DurationMin: 5},
		{Type: "Focus", DurationMin: 25},
		{Type: "Break", DurationMin: 5},
		{Type: "Focus", DurationMin: 25},
		{Type: "Break", DurationMin: 15},
	})
	_ = id
	return err
}

// resetSequenceIfEmpty resets the AUTOINCREMENT sequence for sessions if the table is empty.
func resetSequenceIfEmpty(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		_, err := db.Exec("DELETE FROM sqlite_sequence WHERE name='sessions'")
		return err
	}
	return nil
}

func saveSession(db *sql.DB, s Session, runID int64) (int64, error) {
	var rid interface{}
	if runID != 0 {
		rid = runID
	}
	result, err := db.Exec(
		`INSERT INTO sessions (name, type, duration_min, started_at, completed_at, completed, run_id) VALUES (?, ?, ?, ?, ?, 1, ?)`,
		s.Name, s.Type, s.DurationMin,
		s.StartedAt.Format(dbTimeFormat),
		s.CompletedAt.Format(dbTimeFormat),
		rid,
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return id, err
}

func updateSessionNotes(db *sql.DB, id int64, notes string) error {
	_, err := db.Exec("UPDATE sessions SET notes = ? WHERE id = ?", notes, id)
	return err
}

func todaySessions(db *sql.DB) ([]Session, error) {
	today := time.Now().Format("2006-01-02")
	rows, err := db.Query(
		`SELECT id, name, type, duration_min, started_at, completed_at FROM sessions WHERE date(started_at) = ? AND completed = 1 ORDER BY started_at ASC`,
		today,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []Session
	for rows.Next() {
		var s Session
		var startedAt, completedAt string
		if err := rows.Scan(&s.ID, &s.Name, &s.Type, &s.DurationMin, &startedAt, &completedAt); err != nil {
			continue
		}
		s.StartedAt, _ = time.ParseInLocation(dbTimeFormat, startedAt, time.Local)
		s.CompletedAt, _ = time.ParseInLocation(dbTimeFormat, completedAt, time.Local)
		s.Completed = true
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// counts Focus + Custom sessions (custom sessions default to focus context)
func todayFocusMinutes(db *sql.DB) (int, error) {
	today := time.Now().Format("2006-01-02")
	var total int
	err := db.QueryRow(
		`SELECT COALESCE(SUM(duration_min), 0) FROM sessions WHERE date(started_at) = ? AND completed = 1 AND type IN ('Focus', 'Custom')`,
		today,
	).Scan(&total)
	return total, err
}

func currentStreak(db *sql.DB) (int, error) {
	rows, err := db.Query(
		`SELECT date(started_at) as d FROM sessions WHERE completed = 1 AND type IN ('Focus', 'Custom') GROUP BY d ORDER BY d DESC`,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	streak := 0
	var nextExpected string
	for rows.Next() {
		var dateStr string
		if err := rows.Scan(&dateStr); err != nil {
			break
		}
		if nextExpected == "" {
			streak = 1
			d, _ := time.ParseInLocation("2006-01-02", dateStr, time.Local)
			nextExpected = d.AddDate(0, 0, -1).Format("2006-01-02")
		} else if dateStr == nextExpected {
			streak++
			d, _ := time.ParseInLocation("2006-01-02", dateStr, time.Local)
			nextExpected = d.AddDate(0, 0, -1).Format("2006-01-02")
		} else {
			break
		}
	}
	return streak, nil
}

func reportData(db *sql.DB) ([]DayReport, error) {
	rows, err := db.Query(
		`SELECT date(started_at) as day, COALESCE(SUM(duration_min), 0), COUNT(*),
		        GROUP_CONCAT(CASE WHEN notes IS NOT NULL AND notes != '' THEN substr(notes, 1, 60) END, '||')
		 FROM sessions WHERE completed = 1 AND type IN ('Focus', 'Custom')
		 GROUP BY day ORDER BY day DESC LIMIT 30`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var data []DayReport
	for rows.Next() {
		var d DayReport
		var notes sql.NullString
		if err := rows.Scan(&d.Date, &d.TotalMin, &d.Count, &notes); err != nil {
			continue
		}
		if notes.Valid {
			d.Notes = notes.String
		}
		data = append(data, d)
	}
	return data, nil
}

// — Session management —

func listRecentNames(db *sql.DB) ([]string, error) {
	rows, err := db.Query(
		`SELECT DISTINCT name FROM sessions WHERE name != '' ORDER BY started_at DESC LIMIT 10`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			continue
		}
		names = append(names, n)
	}
	return names, nil
}

func listAllSessions(db *sql.DB) ([]Session, error) {
	rows, err := db.Query(
		`SELECT id, name, type, duration_min, started_at, completed_at, notes FROM sessions ORDER BY started_at DESC LIMIT 100`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []Session
	for rows.Next() {
		var s Session
		var startedAt, completedAt string
		var notes sql.NullString
		if err := rows.Scan(&s.ID, &s.Name, &s.Type, &s.DurationMin, &startedAt, &completedAt, &notes); err != nil {
			continue
		}
		s.StartedAt, _ = time.ParseInLocation(dbTimeFormat, startedAt, time.Local)
		s.CompletedAt, _ = time.ParseInLocation(dbTimeFormat, completedAt, time.Local)
		if notes.Valid {
			s.Notes = notes.String
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func exportToCSV(db *sql.DB, rawPath string, days int) (string, error) {
	home, _ := os.UserHomeDir()
	path := rawPath
	if strings.HasPrefix(path, "~/") {
		path = filepath.Join(home, path[2:])
	}

	// auto-number to avoid overwriting an existing file
	if _, err := os.Stat(path); err == nil {
		ext := filepath.Ext(path)
		base := strings.TrimSuffix(path, ext)
		for i := 1; ; i++ {
			candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
			if _, err := os.Stat(candidate); os.IsNotExist(err) {
				path = candidate
				break
			}
		}
	}

	var rows *sql.Rows
	var err error
	if days == 0 {
		rows, err = db.Query(
			`SELECT id, started_at, completed_at, name, type, duration_min, COALESCE(notes,'')
			 FROM sessions WHERE completed=1 ORDER BY started_at DESC`,
		)
	} else {
		rows, err = db.Query(
			`SELECT id, started_at, completed_at, name, type, duration_min, COALESCE(notes,'')
			 FROM sessions WHERE completed=1
			 AND julianday('now')-julianday(started_at) <= ?
			 ORDER BY started_at DESC`,
			days,
		)
	}
	if err != nil {
		return "", err
	}
	defer rows.Close()

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	_ = w.Write([]string{"id", "start_time", "end_time", "date", "name", "type", "duration_min", "notes"})
	for rows.Next() {
		var id int64
		var startedAt, completedAt, name, stype, notes string
		var dur int
		if err := rows.Scan(&id, &startedAt, &completedAt, &name, &stype, &dur, &notes); err != nil {
			continue
		}
		date := ""
		if len(startedAt) >= 10 {
			date = startedAt[:10]
		}
		_ = w.Write([]string{
			strconv.FormatInt(id, 10),
			startedAt,
			completedAt,
			date,
			name,
			stype,
			strconv.Itoa(dur),
			notes,
		})
	}
	w.Flush()
	return path, w.Error()
}

func deleteSessionByID(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func countOldSessions(db *sql.DB, days int) (int, error) {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE julianday('now') - julianday(started_at) > ?`, days,
	).Scan(&count)
	return count, err
}

func cleanOldSessions(db *sql.DB, days int) error {
	_, err := db.Exec(
		`DELETE FROM sessions WHERE julianday('now') - julianday(started_at) > ?`, days,
	)
	return err
}

// — Template storage —

func listTemplates(db *sql.DB) ([]Template, error) {
	rows, err := db.Query(`SELECT id, name, created_at FROM templates ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var templates []Template
	for rows.Next() {
		var t Template
		var createdAt sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &createdAt); err != nil {
			continue
		}
		if createdAt.Valid {
			t.CreatedAt, _ = time.ParseInLocation(dbTimeFormat, createdAt.String, time.Local)
		}
		templates = append(templates, t)
	}
	for i := range templates {
		intervals, err := loadIntervals(db, templates[i].ID)
		if err == nil {
			templates[i].Intervals = intervals
		}
	}
	return templates, nil
}

func loadIntervals(db *sql.DB, templateID int64) ([]TemplateInterval, error) {
	rows, err := db.Query(
		`SELECT id, template_id, position, type, duration_min FROM template_intervals WHERE template_id = ? ORDER BY position ASC`,
		templateID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var intervals []TemplateInterval
	for rows.Next() {
		var iv TemplateInterval
		if err := rows.Scan(&iv.ID, &iv.TemplateID, &iv.Position, &iv.Type, &iv.DurationMin); err != nil {
			continue
		}
		intervals = append(intervals, iv)
	}
	return intervals, nil
}

func loadTemplate(db *sql.DB, id int64) (*Template, error) {
	var t Template
	var createdAt sql.NullString
	err := db.QueryRow(`SELECT id, name, created_at FROM templates WHERE id = ?`, id).
		Scan(&t.ID, &t.Name, &createdAt)
	if err != nil {
		return nil, err
	}
	if createdAt.Valid {
		t.CreatedAt, _ = time.ParseInLocation(dbTimeFormat, createdAt.String, time.Local)
	}
	intervals, err := loadIntervals(db, t.ID)
	if err != nil {
		return nil, err
	}
	t.Intervals = intervals
	return &t, nil
}

func createTemplate(db *sql.DB, name string, intervals []TemplateInterval) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO templates (name, created_at) VALUES (?, ?)`,
		name, time.Now().Format(dbTimeFormat),
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, iv := range intervals {
		_, err = db.Exec(
			`INSERT INTO template_intervals (template_id, position, type, duration_min) VALUES (?, ?, ?, ?)`,
			id, i, iv.Type, iv.DurationMin,
		)
		if err != nil {
			return 0, err
		}
	}
	return id, nil
}

func updateTemplate(db *sql.DB, id int64, name string, intervals []TemplateInterval) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint
	if _, err := tx.Exec(`UPDATE templates SET name = ? WHERE id = ?`, name, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM template_intervals WHERE template_id = ?`, id); err != nil {
		return err
	}
	for i, iv := range intervals {
		if _, err := tx.Exec(
			`INSERT INTO template_intervals (template_id, position, type, duration_min) VALUES (?, ?, ?, ?)`,
			id, i, iv.Type, iv.DurationMin,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func deleteTemplate(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM templates WHERE id = ?`, id)
	return err
}

func reportDataWeekly(db *sql.DB) ([]DayReport, error) {
	rows, err := db.Query(
		`SELECT strftime('%Y-W%W', started_at) as week, COALESCE(SUM(duration_min), 0), COUNT(*)
		 FROM sessions WHERE completed = 1 AND type IN ('Focus', 'Custom')
		 GROUP BY week ORDER BY week DESC LIMIT 12`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var data []DayReport
	for rows.Next() {
		var d DayReport
		if err := rows.Scan(&d.Date, &d.TotalMin, &d.Count); err != nil {
			continue
		}
		data = append(data, d)
	}
	return data, nil
}

func reportDataMonthly(db *sql.DB) ([]DayReport, error) {
	rows, err := db.Query(
		`SELECT strftime('%Y-%m', started_at) as month, COALESCE(SUM(duration_min), 0), COUNT(*)
		 FROM sessions WHERE completed = 1 AND type IN ('Focus', 'Custom')
		 GROUP BY month ORDER BY month DESC LIMIT 12`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var data []DayReport
	for rows.Next() {
		var d DayReport
		if err := rows.Scan(&d.Date, &d.TotalMin, &d.Count); err != nil {
			continue
		}
		data = append(data, d)
	}
	return data, nil
}

// — Session run storage —

func startSessionRun(db *sql.DB, tmpl *Template) (int64, error) {
	result, err := db.Exec(
		`INSERT INTO session_runs (template_id, name, started_at, completed) VALUES (?, ?, ?, 0)`,
		tmpl.ID, tmpl.Name, time.Now().Format(dbTimeFormat),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func completeSessionRun(db *sql.DB, runID int64) error {
	_, err := db.Exec(
		`UPDATE session_runs SET completed = 1, completed_at = ? WHERE id = ?`,
		time.Now().Format(dbTimeFormat), runID,
	)
	return err
}

func abortSessionRun(db *sql.DB, runID int64) error {
	_, err := db.Exec(
		`UPDATE session_runs SET completed_at = ? WHERE id = ?`,
		time.Now().Format(dbTimeFormat), runID,
	)
	return err
}
