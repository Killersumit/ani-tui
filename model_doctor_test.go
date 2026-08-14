package main

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func modelDoctorEnv() doctorEnvironment {
	return fakeDoctorEnv("linux", "apt-get")
}

func missingMPVIssue(t *testing.T, env doctorEnvironment) dependencyIssue {
	t.Helper()
	manager, ok := detectPackageManager(env, distroInfo{ID: "debian", VersionID: "13", Codename: "trixie"})
	if !ok {
		t.Fatal("fixture package manager was not detected")
	}
	plan, ok := makeInstallPlan(manager, depMPV, defaultConfig(), distroInfo{ID: "debian", VersionID: "13", Codename: "trixie"}, env)
	if !ok {
		t.Fatal("fixture install plan was not created")
	}
	return dependencyIssue{
		Requirement: dependencyRequirement{ID: depMPV, Name: "mpv", Reason: "the selected player is mpv", Alternatives: []string{"mpv"}},
		Plan:        &plan,
	}
}

func TestPreOperationDoctorBlocksMissingDependencyInline(t *testing.T) {
	cfg := defaultConfig()
	cfg.Player = "mpv"
	env := fakeDoctorEnv("linux", "ani-cli", "curl", "sed", "grep", "apt-get")
	bridge, err := newBridgeServer()
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()
	m := newModel(cfg, bridge, newProcessManager(), "/tmp/ani-tui")
	m.doctorEnv = env
	m.width, m.height = 120, 32
	m.search.SetValue("Frieren")

	scan := m.startSearch()
	updated, _ := m.Update(scan())
	m = updated.(model)
	if m.running || m.doctor == nil || m.doctor.phase != doctorOffering {
		t.Fatalf("missing mpv did not gate launch: running %v, doctor %#v", m.running, m.doctor)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"DEPENDENCY DOCTOR", "mpv not found", "/trusted/bin/apt-get install -y mpv", managerSourceURL("apt"), "NAVIGATION"} {
		if !strings.Contains(view, want) {
			t.Fatalf("doctor view is missing %q\n%s", want, view)
		}
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(model)
	if m.running || m.doctor != nil {
		t.Fatalf("declined requirement should block and close preflight: running %v, doctor %#v", m.running, m.doctor)
	}
	if !strings.Contains(m.toast, "Launch blocked") {
		t.Fatalf("blocked launch toast = %q", m.toast)
	}
}

func TestDoctorRetriesOnlyTransientFailures(t *testing.T) {
	requirement := dependencyRequirement{ID: depMPV, Name: "mpv", Alternatives: []string{"mpv"}}
	plan := installPlan{Binary: "apt-get", Args: []string{"install", "-y", "mpv"}}
	m := model{
		doctorToken: 7,
		doctorEnv:   fakeDoctorEnv("linux"),
		doctor: &doctorState{
			active: true, token: 7, phase: doctorInstalling, attempt: 1,
			issues: []dependencyIssue{{Requirement: requirement, Plan: &plan}},
		},
	}

	updated, retry := m.handleDoctorInstall(doctorInstallMsg{Token: 7, Index: 0, Attempt: 1, Output: "HTTP status 503", Err: errors.New("exit status 1")})
	m = updated.(model)
	if retry == nil || m.doctor.phase != doctorBackoffWait {
		t.Fatalf("transient failure was not queued for retry: phase %v, cmd %v", m.doctor.phase, retry)
	}

	m.doctor.phase = doctorInstalling
	m.doctor.attempt = 2
	updated, retry = m.handleDoctorInstall(doctorInstallMsg{Token: 7, Index: 0, Attempt: 2, Output: "permission denied after timeout", Err: errors.New("exit status 1")})
	m = updated.(model)
	if retry != nil || m.doctor.phase != doctorFailed || m.doctor.failure.Kind != installFailurePermanent {
		t.Fatalf("permanent failure retried: phase %v, failure %#v, cmd %v", m.doctor.phase, m.doctor.failure, retry)
	}

	m.doctor.phase = doctorInstalling
	m.doctor.attempt = doctorMaxAttempts
	updated, retry = m.handleDoctorInstall(doctorInstallMsg{Token: 7, Index: 0, Attempt: doctorMaxAttempts, Output: "connection timed out", Err: errors.New("exit status 1")})
	m = updated.(model)
	if retry != nil || m.doctor.phase != doctorFailed {
		t.Fatalf("fifth total attempt exceeded retry budget: phase %v, cmd %v", m.doctor.phase, retry)
	}
}

func TestStartupDoctorScanIsInformationalAndStaleMessagesAreIgnored(t *testing.T) {
	env := modelDoctorEnv()
	issue := missingMPVIssue(t, env)
	m := model{route: routeActivity, doctorToken: 4, doctorEnv: env}

	updated, _ := m.Update(doctorScanMsg{Token: 4, Issues: []dependencyIssue{issue}})
	got := updated.(model)
	if got.doctor == nil || got.doctor.phase != doctorOffering || got.doctor.blocking || got.doctor.pending != nil {
		t.Fatalf("informational scan state = %#v", got.doctor)
	}

	state := got.doctor
	updated, _ = got.Update(doctorScanMsg{Token: 3, Blocking: true, Spec: commandSpec{Operation: opSearch}})
	got = updated.(model)
	if got.doctor != state {
		t.Fatal("a stale scan token replaced the active doctor interaction")
	}

	got.doctor.phase = doctorBackoffWait
	got.doctor.attempt = 2
	updated, _ = got.Update(doctorRetryMsg{Token: 3, Index: 0})
	got = updated.(model)
	if got.doctor.phase != doctorBackoffWait || got.doctor.attempt != 2 {
		t.Fatalf("stale retry mutated state: phase=%v attempt=%d", got.doctor.phase, got.doctor.attempt)
	}

	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got = updated.(model)
	if got.doctor != nil || got.doctorToken != 5 || !strings.Contains(got.toast, "retry canceled") {
		t.Fatalf("backoff cancellation did not invalidate retry: token=%d doctor=%#v toast=%q", got.doctorToken, got.doctor, got.toast)
	}
}

func TestDoctorConfirmationStartsTerminalHandoffInline(t *testing.T) {
	env := modelDoctorEnv()
	m := model{
		cfg: defaultConfig(), route: routeActivity, width: 84, height: 24,
		doctorToken: 6, doctorEnv: env,
		doctor: &doctorState{active: true, blocking: true, token: 6, phase: doctorOffering,
			issues: []dependencyIssue{missingMPVIssue(t, env)}, pending: &commandSpec{Operation: opSearch, Query: "Frieren"}},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	got := updated.(model)
	if cmd == nil || got.doctor == nil || got.doctor.phase != doctorInstalling || got.doctor.attempt != 1 {
		t.Fatalf("confirmation did not begin handoff: cmd=%v doctor=%#v", cmd != nil, got.doctor)
	}
	view := stripANSI(got.View())
	if !strings.Contains(view, "Installing with terminal handoff") || !strings.Contains(view, "attempt 1/5") {
		t.Fatalf("handoff state was not rendered inline:\n%s", view)
	}
}

func TestDoctorInteractionStaysWithinCompactViewport(t *testing.T) {
	env := modelDoctorEnv()
	m := model{
		cfg: defaultConfig(), route: routeActivity, width: 54, height: 16,
		doctorToken: 8, doctorEnv: env,
		doctor: &doctorState{active: true, token: 8, phase: doctorOffering,
			issues: []dependencyIssue{missingMPVIssue(t, env)}},
	}

	plain := ansi.Strip(m.View())
	lines := strings.Split(plain, "\n")
	if len(lines) != m.height {
		t.Fatalf("compact doctor height = %d, want %d\n%s", len(lines), m.height, plain)
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got > m.width {
			t.Fatalf("compact doctor line %d is %d cells, want <= %d: %q", i+1, got, m.width, line)
		}
	}
}

func TestDoctorRechecksExecutableAfterSuccessfulInstaller(t *testing.T) {
	installed := false
	env := fakeDoctorEnv("linux")
	env.LookPath = func(name string) (string, error) {
		if name == "mpv" && installed {
			return "/usr/bin/mpv", nil
		}
		return "", errors.New("missing")
	}
	requirement := dependencyRequirement{ID: depMPV, Name: "mpv", Alternatives: []string{"mpv"}}
	plan := installPlan{Binary: "apt-get", Args: []string{"install", "-y", "mpv"}}
	m := model{
		route: routeActivity, doctorToken: 3, doctorEnv: env,
		doctor: &doctorState{active: true, token: 3, phase: doctorInstalling, attempt: 1, issues: []dependencyIssue{{Requirement: requirement, Plan: &plan}}},
	}

	updated, _ := m.handleDoctorInstall(doctorInstallMsg{Token: 3, Index: 0, Attempt: 1})
	m = updated.(model)
	if m.doctor == nil || m.doctor.phase != doctorFailed {
		t.Fatal("successful exit without an executable incorrectly passed verification")
	}

	installed = true
	m.doctor.phase = doctorInstalling
	updated, _ = m.handleDoctorInstall(doctorInstallMsg{Token: 3, Index: 0, Attempt: 1})
	m = updated.(model)
	if m.doctor != nil {
		t.Fatal("verified executable did not advance and close the completed scan")
	}
}
