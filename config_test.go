package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigRoundTripAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	t.Setenv("ANI_TUI_CONFIG", path)
	cfg := defaultConfig()
	cfg.Quality = "1080"
	cfg.Mode = "dub"
	cfg.Player = "custom"
	cfg.CustomPlayer = "kitty +kitten icat"
	cfg.HistoryDir = "/tmp/ani-history"
	cfg.UpdateBranch = "dev"
	cfg.LogPlayback = false
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("config permissions = %o, want 600", info.Mode().Perm())
	}
	loaded, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Quality != "1080" || loaded.Mode != "dub" || loaded.CustomPlayer != cfg.CustomPlayer || loaded.UpdateBranch != "dev" {
		t.Fatalf("loaded config did not match saved values: %#v", loaded)
	}
	if loaded.LogPlayback {
		t.Fatal("LogPlayback changed during round trip")
	}
}

func TestSaveConfigReplacesExistingFileWithoutTemporaryArtifacts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("ANI_TUI_CONFIG", path)

	cfg := defaultConfig()
	cfg.Quality = "720"
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Quality = "1080"
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Quality != "1080" {
		t.Fatalf("replacement quality = %q, want 1080", loaded.Quality)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		t.Fatalf("unexpected files after replacement: %v", entries)
	}
}

func TestLoadConfigUsesDefaultsAndNormalizes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("ANI_TUI_CONFIG", path)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AniCLIPath != "ani-cli" || cfg.Quality != "best" || !cfg.LogPlayback || cfg.UpdateBranch != "master" {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}

	invalid := Config{Quality: "4k", Mode: "raw", Player: "unknown", Action: "erase", ExitAfterPlay: true}
	data, _ := json.Marshal(invalid)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	normalized, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Quality != "best" || normalized.Mode != "sub" || normalized.Player != "auto" || normalized.Action != "stream" {
		t.Fatalf("invalid values were not normalized: %#v", normalized)
	}
	if !normalized.NoDetach {
		t.Fatal("exit-after-play must imply no-detach")
	}
	if normalized.UpdateBranch != "master" {
		t.Errorf("empty update branch normalized to %q", normalized.UpdateBranch)
	}
}

func TestConfigPathHonorsXDGLocation(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("ANI_TUI_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	want := filepath.Join(xdg, "ani-tui", "config.json")
	if got := configPath(); got != want {
		t.Fatalf("configPath() = %q, want %q", got, want)
	}
}

func TestCycleWraps(t *testing.T) {
	values := []string{"a", "b", "c"}
	if got := cycle(values, "a", -1); got != "c" {
		t.Errorf("backward wrap = %q", got)
	}
	if got := cycle(values, "c", 1); got != "a" {
		t.Errorf("forward wrap = %q", got)
	}
}
