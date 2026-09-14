# pomo

A terminal pomodoro timer built with Bubbletea. Opinionated, keyboard-driven, with
session history, custom templates, and Claude AI baked in. No config required to start.

Built on top of a base pomodoro timer; this fork adds persistent storage, a reporting
layer, template runs, a live Claude query panel with conversation thread, end-of-session
AI summaries, achievements, and a compact single-line mode.

---

## Features

- **Quick sessions** — pick Focus/Break/Custom from a menu, type any number for a
  custom duration
- **Template runs** — define multi-interval sequences (e.g. 4×25/5 Pomodoro), run
  them step-by-step; a default "Pomodoro" template is seeded on first launch
- **Background timer + resume** — press `backspace` mid-session to send the timer
  to the background; a live countdown appears on the home screen; `enter` to resume
- **In-countdown notes** — press `m` during a session to open a note overlay;
  the note is saved with the session on finish or early-end
- **Compact mode** — press `z` to collapse the countdown to a minimal centered box;
  `z` again to expand
- **Finish early** — press `f` to end the current interval now; elapsed time is
  logged as the actual duration (minimum 1 min)
- **Ask Claude panel** — press `c` during countdown for an inline chat panel;
  queries carry session name as context; maintains a conversation thread per session;
  all Q&A logged to `claude_queries` table for report scripts
- **End-of-session summary** — on interval complete, Claude generates a one-sentence
  summary of what you worked on; shown on the done screen
- **Session history + reports** — daily/weekly/monthly focus reports with ASCII
  bar charts; session notes surfaced inline
- **CSV export** — export sessions by date range; auto-numbered to avoid overwriting
- **Streak tracking** — consecutive days with at least one Focus session
- **Achievements** — milestones tracked in `~/.pomo/achievements.json`; unlocks
  shown on the done screen

---

## Install

Requires CGO (for sqlite3).

```bash
CGO_ENABLED=1 go build -o pomo-tui .
sudo cp pomo-tui /usr/local/bin/pomo
```

Optional: install `figlet` for the animated header (falls back gracefully without it).

```bash
sudo port install figlet   # macOS MacPorts
```

Launch:

```bash
pomo
pomo --version
pomo -s "Deep Work"    # start with a named session
```

---

## Keys

### Home screen

| Key     | Action           |
|---------|------------------|
| `j`/`k` | Navigate menu    |
| `enter` | Select           |
| `n`     | Name the session |
| `r`     | Open report      |
| `?`     | Help overlay     |
| `q`     | Quit             |

### Countdown

| Key         | Action                           |
|-------------|----------------------------------|
| `p`         | Pause / resume                   |
| `f`         | Finish early (logs elapsed time) |
| `m`         | Open in-session note overlay     |
| `c`         | Open / close Ask Claude panel    |
| `z`         | Toggle compact mode              |
| `n`         | Rename session                   |
| `backspace` | Background timer, return to home |
| `q`         | Quit                             |

### Ask Claude panel

| Key     | Action                        |
|---------|-------------------------------|
| `enter` | Submit query                  |
| `tab`   | Clear conversation thread     |
| `esc`   | Close panel / cancel loading  |

### Done screen

| Key     | Action                  |
|---------|-------------------------|
| `j`/`k` | Navigate choices        |
| `enter` | Select                  |
| `m`     | Add / edit session note |

### Report

| Key               | Action                         |
|-------------------|--------------------------------|
| `tab`             | Cycle Daily / Weekly / Monthly |
| `x`               | Export to CSV                  |
| `r` / `backspace` | Close                          |

---

## Config

Stored at `~/.pomo/config.json`. Editable in-app via the Config menu.

```json
{
  "old_session_days": 7,
  "cleanup_enabled": true,
  "csv_export_path": "~/Desktop/pomo-export.csv"
}
```

---

## Data

| Path                        | Contents                                           |
|-----------------------------|----------------------------------------------------|
| `~/.pomo/sessions.db`       | SQLite — sessions, templates, runs, claude_queries |
| `~/.pomo/achievements.json` | Achievement unlock timestamps                      |

The `claude_queries` table (`id, asked_at, session_name, session_type, query, response`)
is designed for markweek-style report scripts to join against session data by date
and session name.

---

## Claude integration

Press `c` during any countdown to open the Ask Claude panel. Requires the `claude`
CLI installed and authenticated.

**Session context:** if the session has a name, every query is prefixed with
`[Working on: "<name>"]` automatically.

**Conversation thread:** previous Q&A pairs from the current session are prepended
as context on each new query (last 3 exchanges). Press `tab` to clear the thread.

**Response length:** queries include `[non-interactive query from pomodoro app,
keep response under 270 chars]` to keep answers readable in the panel.

**End-of-session summary:** when an interval completes, a one-sentence session
summary is generated automatically and shown on the done screen.

**Logging:** every query and response is saved to `claude_queries` in the SQLite
database with timestamp, session name, and session type.
