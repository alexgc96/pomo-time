package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Achievement struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Desc       string    `json:"desc"`
	UnlockedAt time.Time `json:"unlocked_at"`
}

var achievementDefs = []struct {
	id    string
	name  string
	desc  string
	check func(streak, todayMin, totalSessions int) bool
}{
	{"first_session", "First drop", "Complete your first focus session",
		func(s, m, n int) bool { return n >= 1 }},
	{"streak_3", "Three's a habit", "3-day focus streak",
		func(s, m, n int) bool { return s >= 3 }},
	{"streak_7", "Week warrior", "7-day focus streak",
		func(s, m, n int) bool { return s >= 7 }},
	{"streak_30", "Consistency king", "30-day focus streak",
		func(s, m, n int) bool { return s >= 30 }},
	{"deep_day", "Deep work day", "4+ hours of focus in one day",
		func(s, m, n int) bool { return m >= 240 }},
	{"century", "The century", "100 focus sessions logged",
		func(s, m, n int) bool { return n >= 100 }},
	{"early_bird", "Early finish", "Use finish-early at least once",
		func(s, m, n int) bool { return n >= 1 }}, // checked separately via flag
}

func achievementsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pomo", "achievements.json")
}

func loadAchievements() map[string]time.Time {
	data, err := os.ReadFile(achievementsPath())
	if err != nil {
		return map[string]time.Time{}
	}
	var m map[string]time.Time
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]time.Time{}
	}
	return m
}

func saveAchievements(m map[string]time.Time) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(achievementsPath(), data, 0644)
}

// checkAchievements returns display names of newly unlocked achievements.
// earlyFinish should be true when the session was ended with the f key.
func checkAchievements(db *sql.DB, existing map[string]time.Time, streak, todayMin int, earlyFinish bool) ([]string, map[string]time.Time) {
	var totalSessions int
	if db != nil {
		_ = db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE type='Focus' AND completed=1`).Scan(&totalSessions)
	}

	updated := make(map[string]time.Time, len(existing))
	for k, v := range existing {
		updated[k] = v
	}

	var newlyUnlocked []string
	for _, def := range achievementDefs {
		if _, already := updated[def.id]; already {
			continue
		}
		if def.id == "early_bird" {
			if earlyFinish {
				updated[def.id] = time.Now()
				newlyUnlocked = append(newlyUnlocked, def.name)
			}
			continue
		}
		if def.check(streak, todayMin, totalSessions) {
			updated[def.id] = time.Now()
			newlyUnlocked = append(newlyUnlocked, def.name)
		}
	}

	if len(newlyUnlocked) > 0 {
		saveAchievements(updated)
	}
	return newlyUnlocked, updated
}
