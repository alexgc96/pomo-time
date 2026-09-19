package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	OldSessionDays int    `json:"old_session_days"`
	CleanupEnabled bool   `json:"cleanup_enabled"`
	CSVExportPath  string `json:"csv_export_path"`
}

func defaultConfig() Config {
	return Config{
		OldSessionDays: 7,
		CleanupEnabled: true,
		CSVExportPath:  "~/Desktop/pomo-export.csv",
	}
}

func loadConfig() Config {
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultConfig()
	}
	f, err := os.Open(filepath.Join(home, ".pomo", "config.json"))
	if err != nil {
		return defaultConfig()
	}
	defer f.Close()
	cfg := defaultConfig()
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return defaultConfig()
	}
	if cfg.OldSessionDays == 0 {
		cfg.OldSessionDays = 7
	}
	if cfg.CSVExportPath == "" {
		cfg.CSVExportPath = "~/Desktop/pomo-export.csv"
	}
	return cfg
}

func saveConfig(cfg Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".pomo", "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
