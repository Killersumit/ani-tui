package main

import (
	"strings"
	"testing"
)

func TestVersionStringDevelopmentAndRelease(t *testing.T) {
	oldVersion, oldCommit, oldBuildDate := version, commit, buildDate
	t.Cleanup(func() {
		version, commit, buildDate = oldVersion, oldCommit, oldBuildDate
	})

	version, commit, buildDate = "dev", "unknown", "unknown"
	if got := versionString(); got != "ani-tui dev" {
		t.Fatalf("development version = %q", got)
	}

	version, commit, buildDate = "v1.2.3", "abc123", "2026-08-14T12:00:00Z"
	got := versionString()
	for _, want := range []string{"ani-tui v1.2.3", "commit abc123", "built 2026-08-14T12:00:00Z"} {
		if !strings.Contains(got, want) {
			t.Errorf("release version %q does not contain %q", got, want)
		}
	}
}
