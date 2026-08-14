package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	AniCLIPath    string `json:"ani_cli_path"`
	Quality       string `json:"quality"`
	Mode          string `json:"mode"`
	Player        string `json:"player"`
	CustomPlayer  string `json:"custom_player"`
	Action        string `json:"action"`
	DownloadDir   string `json:"download_dir"`
	PlayerFlags   string `json:"player_flags"`
	HistoryDir    string `json:"history_dir"`
	UpdateBranch  string `json:"update_branch"`
	SkipIntro     bool   `json:"skip_intro"`
	NoDetach      bool   `json:"no_detach"`
	ExitAfterPlay bool   `json:"exit_after_play"`
	LogPlayback   bool   `json:"log_playback"`
	Mouse         bool   `json:"mouse"`
}

func defaultConfig() Config {
	return Config{
		AniCLIPath:   "ani-cli",
		Quality:      "best",
		Mode:         "sub",
		Player:       "auto",
		Action:       "stream",
		DownloadDir:  ".",
		UpdateBranch: "master",
		LogPlayback:  true,
		Mouse:        true,
	}
}

func configHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	dir, err := os.UserConfigDir()
	if err == nil && dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config")
}

func configPath() string {
	if v := os.Getenv("ANI_TUI_CONFIG"); v != "" {
		return v
	}
	return filepath.Join(configHome(), "ani-tui", "config.json")
}

func loadConfig() (Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(configPath())
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultConfig(), err
	}
	cfg.normalize()
	return cfg, nil
}

func (c *Config) normalize() {
	if strings.TrimSpace(c.AniCLIPath) == "" {
		c.AniCLIPath = "ani-cli"
	}
	if !contains([]string{"best", "1080", "720", "480", "360", "worst"}, c.Quality) {
		c.Quality = "best"
	}
	if c.Mode != "dub" {
		c.Mode = "sub"
	}
	if !contains([]string{"auto", "mpv", "vlc", "syncplay", "debug", "catt", "custom"}, c.Player) {
		c.Player = "auto"
	}
	if c.Action != "download" {
		c.Action = "stream"
	}
	if strings.TrimSpace(c.CustomPlayer) == "" {
		c.CustomPlayer = "mpv"
	}
	if strings.TrimSpace(c.DownloadDir) == "" {
		c.DownloadDir = "."
	}
	if strings.TrimSpace(c.UpdateBranch) == "" {
		c.UpdateBranch = "master"
	}
	if c.ExitAfterPlay {
		c.NoDetach = true
	}
}

func saveConfig(cfg Config) error {
	cfg.normalize()
	path := configPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	file, err := os.CreateTemp(dir, ".ani-tui-config-*.tmp")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return replaceFile(tmp, path)
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func cycle(items []string, current string, delta int) string {
	idx := 0
	for i, item := range items {
		if item == current {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(items)) % len(items)
	return items[idx]
}
