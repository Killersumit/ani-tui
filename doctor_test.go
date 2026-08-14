package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fakeDoctorEnv(goos string, commands ...string) doctorEnvironment {
	available := make(map[string]bool)
	for _, command := range commands {
		available[command] = true
	}
	return doctorEnvironment{
		GOOS: goos,
		LookPath: func(name string) (string, error) {
			if available[name] {
				return "/trusted/bin/" + name, nil
			}
			return "", fmt.Errorf("%s not found", name)
		},
		PathExists: func(path string) bool { return available[path] },
		ReadFile:   func(string) ([]byte, error) { return nil, errors.New("missing") },
		IsRoot:     true,
	}
}

func requirementIDs(requirements []dependencyRequirement) []dependencyID {
	ids := make([]dependencyID, 0, len(requirements))
	for _, requirement := range requirements {
		ids = append(ids, requirement.ID)
	}
	return ids
}

func TestRequiredDependenciesFollowActiveConfiguration(t *testing.T) {
	cfg := defaultConfig()
	cfg.AniCLIPath = "ani-cli"
	cfg.Player = "debug"
	env := fakeDoctorEnv("linux")

	base := requirementIDs(requiredDependencies(cfg, commandSpec{Operation: opSearch, Query: "Frieren"}, env))
	if !reflect.DeepEqual(base, []dependencyID{depAniCLI, depCurl, depSed, depGrep}) {
		t.Fatalf("debug requirements = %#v", base)
	}

	cfg.Action = "download"
	download := requirementIDs(requiredDependencies(cfg, commandSpec{Operation: opSearch, Query: "Frieren"}, env))
	if !reflect.DeepEqual(download, []dependencyID{depAniCLI, depCurl, depSed, depGrep, depYTDLP}) {
		t.Fatalf("download requirements = %#v", download)
	}

	cfg.SkipIntro = true
	countdownDownload := requirementIDs(requiredDependencies(cfg, commandSpec{Operation: opCountdown, Query: "Frieren"}, env))
	if !reflect.DeepEqual(countdownDownload, []dependencyID{depAniCLI, depCurl, depSed, depGrep, depYTDLP, depAniSkip}) {
		t.Fatalf("download countdown requirements = %#v", countdownDownload)
	}

	cfg.Action = "stream"
	cfg.SkipIntro = false
	cfg.Player = "syncplay"
	syncplay := requirementIDs(requiredDependencies(cfg, commandSpec{Operation: opSearch, Query: "Frieren"}, env))
	if !reflect.DeepEqual(syncplay, []dependencyID{depAniCLI, depCurl, depSed, depGrep, depMPV, depSync}) {
		t.Fatalf("syncplay requirements = %#v", syncplay)
	}

	cfg.Player = "debug"
	update := requirementIDs(requiredDependencies(cfg, commandSpec{Operation: opUpdate}, env))
	if !reflect.DeepEqual(update, []dependencyID{depAniCLI, depCurl, depSed, depGrep, depPatch}) {
		t.Fatalf("update requirements = %#v", update)
	}
}

func TestAlternativeRequirementsAcceptAnyExecutable(t *testing.T) {
	env := fakeDoctorEnv("linux", "ffmpeg")
	requirement := dependencyRequirement{Alternatives: []string{"yt-dlp", "ffmpeg"}}
	if !requirementSatisfied(requirement, env) {
		t.Fatal("ffmpeg did not satisfy the downloader alternative group")
	}

	nonExecutable := fakeDoctorEnv("linux")
	nonExecutable.PathExists = func(path string) bool { return path == "/custom/ani-cli" }
	custom := dependencyRequirement{ID: depAniCLI, Alternatives: []string{"/custom/ani-cli"}}
	if requirementSatisfied(custom, nonExecutable) {
		t.Fatal("a merely existing custom ani-cli path was accepted as executable")
	}

	flatpak := "$HOME/.local/share/flatpak/app/io.mpv.Mpv/"
	flatpakEnv := fakeDoctorEnv("linux")
	flatpakEnv.PathExists = func(path string) bool { return strings.Contains(filepath.ToSlash(path), "/flatpak/app/io.mpv.Mpv") }
	if !requirementSatisfied(dependencyRequirement{ID: depMPV, Alternatives: []string{expandHome(flatpak)}}, flatpakEnv) {
		t.Fatal("the upstream Flatpak mpv directory sentinel was not accepted")
	}
}

func TestParseOSReleaseAndManagerDetection(t *testing.T) {
	distro := parseOSRelease([]byte("ID=ubuntu\nID_LIKE=debian\nVERSION_ID=24.04\nVERSION_CODENAME=noble\n"))
	if distro.ID != "ubuntu" || !reflect.DeepEqual(distro.IDLike, []string{"debian"}) || distro.Codename != "noble" {
		t.Fatalf("parsed distro = %#v", distro)
	}
	env := fakeDoctorEnv("linux", "dnf", "apt-get")
	manager, ok := detectPackageManager(env, distro)
	if !ok || manager.ID != "apt" || manager.Binary != "/trusted/bin/apt-get" {
		t.Fatalf("manager = %#v, ok %v", manager, ok)
	}
}

func TestVerifiedManagerSpecificPlans(t *testing.T) {
	cfg := defaultConfig()
	tests := []struct {
		name       string
		manager    packageManager
		dependency dependencyID
		distro     distroInfo
		env        doctorEnvironment
		want       string
	}{
		{
			name: "Debian 13 ani-cli", manager: packageManager{ID: "apt", Binary: "/usr/bin/apt-get", SourceURL: managerSourceURL("apt")},
			dependency: depAniCLI, distro: distroInfo{ID: "debian", VersionID: "13", Codename: "trixie"}, env: fakeDoctorEnv("linux"),
			want: "/usr/bin/apt-get install -y ani-cli",
		},
		{
			name: "Fedora ffmpeg", manager: packageManager{ID: "dnf", Binary: "/usr/bin/dnf", SourceURL: managerSourceURL("dnf")},
			dependency: depFFmpeg, distro: distroInfo{ID: "fedora"}, env: fakeDoctorEnv("linux"),
			want: "/usr/bin/dnf install -y ffmpeg-free",
		},
		{
			name: "Homebrew IINA cask", manager: packageManager{ID: "brew", Binary: "/opt/homebrew/bin/brew", SourceURL: managerSourceURL("brew")},
			dependency: depIINA, env: fakeDoctorEnv("darwin"), want: "/opt/homebrew/bin/brew install --cask iina",
		},
		{
			name: "Scoop extras setup", manager: packageManager{ID: "scoop", Binary: "scoop", SourceURL: managerSourceURL("scoop")},
			dependency: depMPV, env: fakeDoctorEnv("windows"), want: "scoop bucket add extras && scoop install mpv",
		},
		{
			name: "winget exact yt-dlp", manager: packageManager{ID: "winget", Binary: "winget", SourceURL: managerSourceURL("winget")},
			dependency: depYTDLP, env: fakeDoctorEnv("windows"),
			want: "winget install --id yt-dlp.yt-dlp -e --source winget --accept-package-agreements --accept-source-agreements",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, ok := makeInstallPlan(tc.manager, tc.dependency, cfg, tc.distro, tc.env)
			if !ok {
				t.Fatal("expected a verified plan")
			}
			if got := plan.DisplayCommand(); got != tc.want {
				t.Fatalf("command = %q, want %q", got, tc.want)
			}
			if plan.SourceURL == "" {
				t.Fatal("verified plan omitted its official source URL")
			}
		})
	}
}

func TestInstallPlanRunsScoopBucketSetupBeforePackage(t *testing.T) {
	temp := t.TempDir()
	logPath := filepath.Join(temp, "calls.log")
	scriptPath := filepath.Join(temp, "scoop-fixture")
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> \"$DOCTOR_CALLS\"\n" +
		"if [ \"$1\" = bucket ]; then echo 'bucket already exists' >&2; exit 1; fi\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCTOR_CALLS", logPath)
	plan := installPlan{
		Prep:   []installStep{{Binary: scriptPath, Args: []string{"bucket", "add", "extras"}, AllowAlreadyExists: true}},
		Binary: scriptPath, Args: []string{"install", "mpv"},
	}
	command := newTeeExecCommand(plan)
	if err := command.Run(); err != nil {
		t.Fatalf("multi-step plan failed: %v\n%s", err, command.capture.String())
	}
	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(calls), "bucket add extras\ninstall mpv\n"; got != want {
		t.Fatalf("Scoop calls = %q, want %q", got, want)
	}
}

func TestAniCLIAutoInstallIsRestrictedToVerifiedRoutes(t *testing.T) {
	cfg := defaultConfig()
	nonRoot := fakeDoctorEnv("linux")
	nonRoot.IsRoot = false
	if _, ok := makeInstallPlan(packageManager{ID: "apt", Binary: "/usr/bin/apt-get", SourceURL: managerSourceURL("apt")}, depMPV, cfg, distroInfo{ID: "debian", VersionID: "13"}, nonRoot); ok {
		t.Fatal("non-root system package plan must not be offered without sudo")
	}
	if _, ok := packageFor("apt", depAniCLI, cfg, distroInfo{ID: "ubuntu", VersionID: "24.04"}); ok {
		t.Fatal("Ubuntu must not be offered Debian's ani-cli package")
	}
	if _, ok := packageFor("apt", depAniCLI, cfg, distroInfo{ID: "debian", VersionID: "12", Codename: "bookworm"}); ok {
		t.Fatal("Debian 12 must not be offered the Debian 13 package")
	}
	if _, ok := packageFor("dnf", depAniCLI, cfg, distroInfo{ID: "fedora"}); ok {
		t.Fatal("the doctor must not silently enable a COPR")
	}
	if _, ok := packageFor("scoop", depAniCLI, cfg, distroInfo{}); !ok {
		t.Fatal("upstream's Scoop ani-cli route should be available")
	}
}

func TestFailureClassificationUsesPermanentPrecedence(t *testing.T) {
	tests := []struct {
		output string
		want   installFailureKind
	}{
		{"connection timed out; unable to locate package mpv", installFailurePermanent},
		{"HTTP status 503 Service Unavailable", installFailureTransient},
		{"HTTP 408 Request Timeout", installFailureTransient},
		{"429 Too Many Requests", installFailureTransient},
		{"HTTP 403 Forbidden while downloading", installFailurePermanent},
		{"temporary failure resolving mirror.example", installFailureTransient},
		{"checksum mismatch after download", installFailurePermanent},
		{"failed to download package", installFailureUnknown},
		{"installer exited with code 7", installFailureUnknown},
	}
	for _, tc := range tests {
		got := classifyInstallFailure(tc.output, errors.New("exit status 1"))
		if got.Kind != tc.want {
			t.Errorf("classify(%q) = %s (%s), want %s", tc.output, got.Kind, got.Reason, tc.want)
		}
	}
	if got := classifyInstallFailure("", nil); got.Kind != installFailureNone {
		t.Fatalf("successful install classified as %s", got.Kind)
	}
}

func TestRetryBudgetAndBackoffAreDeterministic(t *testing.T) {
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second}
	for attempt, expected := range want {
		if got := doctorBackoff(attempt + 1); got != expected {
			t.Errorf("attempt %d backoff = %s, want %s", attempt+1, got, expected)
		}
	}
	if doctorMaxAttempts != 5 {
		t.Fatalf("retry budget = %d, want five total attempts", doctorMaxAttempts)
	}
}

func TestBoundedCaptureKeepsOnlyTail(t *testing.T) {
	capture := newBoundedCapture(8)
	_, _ = capture.Write([]byte("first"))
	_, _ = capture.Write([]byte("-second"))
	if got := capture.String(); got != "t-second" {
		t.Fatalf("capture tail = %q", got)
	}
	if strings.Contains(capture.String(), "first") {
		t.Fatal("capture exceeded its bound")
	}
}
