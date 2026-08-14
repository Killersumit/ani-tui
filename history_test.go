package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadHistory(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultConfig()
	cfg.HistoryDir = dir
	content := "12\tcowboy-bebop\tCowboy Bebop\n3\tfrieren\tFrieren: Beyond Journey's End\nmalformed\n"
	if err := os.WriteFile(filepath.Join(dir, "ani-hsts"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadHistory(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []historyEntry{
		{Episode: "12", ID: "cowboy-bebop", Title: "Cowboy Bebop"},
		{Episode: "3", ID: "frieren", Title: "Frieren: Beyond Journey's End"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loadHistory() = %#v, want %#v", got, want)
	}
}

func TestHistoryPathUsesXDGState(t *testing.T) {
	t.Setenv("ANI_CLI_HIST_DIR", "")
	t.Setenv("XDG_STATE_HOME", "/tmp/state-root")
	if got := historyPath(defaultConfig()); got != "/tmp/state-root/ani-cli/ani-hsts" {
		t.Fatalf("historyPath() = %q", got)
	}
}

func TestMissingHistoryIsEmpty(t *testing.T) {
	cfg := defaultConfig()
	cfg.HistoryDir = t.TempDir()
	entries, err := loadHistory(cfg)
	if err != nil || len(entries) != 0 {
		t.Fatalf("missing history = %#v, %v", entries, err)
	}
}
