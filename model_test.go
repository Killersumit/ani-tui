package main

import (
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestEpisodeMenuSupportsRangeAndPreservesRawRows(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	menu := newSelectionModal(bridgeRequest{
		Prompt:  "Select episode: ",
		Options: []string{"Episode 1", "Episode 2", "Episode 3"},
	}, server, 100)
	if !menu.multi {
		t.Fatal("episode prompt did not enable multi-selection")
	}
	menu.selected[0] = true
	menu.selected[2] = true
	m := model{menu: menu}
	_, cmd := m.finishMenuSelection()
	if cmd == nil {
		t.Fatal("selection did not produce a bridge reply")
	}
	done := make(chan struct{})
	go func() {
		_ = cmd()
		close(done)
	}()
	var response bridgeResponse
	if err := json.NewDecoder(client).Decode(&response); err != nil {
		t.Fatal(err)
	}
	<-done
	want := "Episode 1\nEpisode 3"
	if got := strings.Join(response.Selected, "\n"); got != want {
		t.Fatalf("selected rows = %q, want %q", got, want)
	}
}

func TestMenuFilterIsCaseInsensitive(t *testing.T) {
	menu := newSelectionModal(bridgeRequest{Prompt: "Select anime", Options: []string{"Cowboy Bebop", "Frieren", "Bocchi the Rock"}}, nil, 80)
	menu.filter.SetValue("ROCK")
	menu.applyFilter()
	if len(menu.filtered) != 1 || menu.filtered[0] != "Bocchi the Rock" {
		t.Fatalf("filtered = %#v", menu.filtered)
	}
}

func TestPaletteExposesAdvancedAniCLIActions(t *testing.T) {
	palette := newPalette(true, 100)
	ids := make(map[string]bool)
	for _, item := range palette.items {
		ids[item.id] = true
	}
	for _, id := range []string{"search", "continue", "episode", "index", "countdown", "logs", "update", "delete", "cancel"} {
		if !ids[id] {
			t.Errorf("palette is missing action %q", id)
		}
	}
}

func TestGlobalNavigationMutatesModel(t *testing.T) {
	t.Setenv("ANI_TUI_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	cfg := defaultConfig()
	bridge, err := newBridgeServer()
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()
	m := newModel(cfg, bridge, newProcessManager(), "/tmp/ani-tui")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.search.Focused() {
		t.Fatal("escape did not release search focus")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(model)
	if m.route != routeSettings {
		t.Fatalf("route = %v, want settings", m.route)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(model)
	if m.settingsCursor != 1 {
		t.Fatalf("settings cursor = %d, want 1", m.settingsCursor)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = updated.(model)
	if m.cfg.Mode != "dub" {
		t.Fatalf("audio mode = %q, want dub", m.cfg.Mode)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(model)
	if m.palette == nil {
		t.Fatal("ctrl+p did not open command palette")
	}
}

func TestReturningToDiscoverRefocusesSearchInput(t *testing.T) {
	t.Setenv("ANI_TUI_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	bridge, err := newBridgeServer()
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()
	m := newModel(defaultConfig(), bridge, newProcessManager(), "/tmp/ani-tui")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	if m.route != routeHistory || m.search.Focused() {
		t.Fatalf("tab away produced route %v, focused %v", m.route, m.search.Focused())
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = updated.(model)
	if m.route != routeHome || !m.search.Focused() {
		t.Fatalf("return to Discover produced route %v, focused %v", m.route, m.search.Focused())
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Frieren")})
	m = updated.(model)
	if got := m.search.Value(); got != "Frieren" {
		t.Fatalf("search value after returning to Discover = %q", got)
	}
}

func TestEditableSettingPersists(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("ANI_TUI_CONFIG", configPath)
	cfg := defaultConfig()
	bridge, err := newBridgeServer()
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()
	m := newModel(cfg, bridge, newProcessManager(), "/tmp/ani-tui")
	m.width = 100
	m.route = routeSettings
	m.settingsCursor = 10 // Player flags.

	if cmd := m.updateSettingsKeys("enter"); cmd == nil || m.edit == nil {
		t.Fatal("editable setting did not open its editor")
	}
	m.edit.input.SetValue("--profile=anime")
	updated, _ := m.updateEdit(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.cfg.PlayerFlags != "--profile=anime" {
		t.Fatalf("player flags = %q", m.cfg.PlayerFlags)
	}
	persisted, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if persisted.PlayerFlags != "--profile=anime" {
		t.Fatalf("persisted player flags = %q", persisted.PlayerFlags)
	}
}

func TestProcessCompletionRepliesToActiveEmbeddedSelection(t *testing.T) {
	t.Setenv("ANI_TUI_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	client, server := net.Pipe()
	defer client.Close()
	if err := client.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	m := model{
		cfg:  defaultConfig(),
		pm:   newProcessManager(),
		menu: newSelectionModal(bridgeRequest{Prompt: "Select anime:", Options: []string{"Frieren"}}, server, 100),
	}
	updated, cmd := m.Update(processDoneMsg{})
	got := updated.(model)
	if got.menu != nil {
		t.Fatal("completed process left its embedded selection active")
	}
	if cmd == nil {
		t.Fatal("completed process returned no commands")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("completion command type = %T, want tea.BatchMsg", cmd())
	}
	if len(batch) != 3 {
		t.Fatalf("completion queued %d commands, want bridge reply, toast, and event wait", len(batch))
	}

	replied := make(chan tea.Msg, 1)
	go func() { replied <- batch[0]() }()
	var response bridgeResponse
	if err := json.NewDecoder(client).Decode(&response); err != nil {
		t.Fatalf("decode completion bridge reply: %v", err)
	}
	if !response.Canceled {
		t.Fatalf("completion bridge reply = %#v, want canceled", response)
	}
	if msg := <-replied; msg != nil {
		t.Fatalf("bridge reply returned unexpected message %#v", msg)
	}
}

func TestOperationStartupReturnsBubbleTeaCommand(t *testing.T) {
	cfg := defaultConfig()
	cfg.AniCLIPath = "/bin/true"
	cfg.Player = "debug"
	bridge, err := newBridgeServer()
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()
	m := newModel(cfg, bridge, newProcessManager(), "/tmp/ani-tui")
	m.search.SetValue("cowboy bebop")

	scan := m.startSearch()
	if scan == nil {
		t.Fatal("operation startup did not return a dependency scan command")
	}
	if m.running || m.doctor == nil || m.doctor.phase != doctorScanning {
		t.Fatalf("preflight state = running %v, doctor %#v", m.running, m.doctor)
	}
	updated, start := m.Update(scan())
	m = updated.(model)
	if start == nil {
		t.Fatal("successful preflight did not return the ani-cli start command")
	}
	if !m.running || m.route != routeActivity {
		t.Fatalf("startup state = running %v, route %v", m.running, m.route)
	}
}

func TestMouseSidebarAndSettingsWheelNavigation(t *testing.T) {
	cfg := defaultConfig()
	bridge, err := newBridgeServer()
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()
	m := newModel(cfg, bridge, newProcessManager(), "/tmp/ani-tui")
	m.width, m.height = 120, 34

	updated, _ := m.Update(tea.MouseMsg{X: 4, Y: 6, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m = updated.(model)
	if m.route != routeSettings || m.search.Focused() {
		t.Fatalf("sidebar click produced route %v, search focused %v", m.route, m.search.Focused())
	}
	updated, _ = m.Update(tea.MouseMsg{X: 60, Y: 12, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	m = updated.(model)
	if m.settingsCursor != 3 {
		t.Fatalf("settings cursor after wheel = %d, want 3", m.settingsCursor)
	}
	updated, _ = m.Update(tea.MouseMsg{X: 4, Y: 4, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m = updated.(model)
	if m.route != routeHome || !m.search.Focused() {
		t.Fatalf("Discover click produced route %v, search focused %v", m.route, m.search.Focused())
	}
}

func TestResponsiveViewAndSmallTerminalFallback(t *testing.T) {
	cfg := defaultConfig()
	bridge, err := newBridgeServer()
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()
	m := newModel(cfg, bridge, newProcessManager(), "/tmp/ani-tui")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	wide := updated.(model).View()
	if !strings.Contains(wide, "ANI") || !strings.Contains(wide, "What do you want to watch?") || !strings.Contains(wide, "NAVIGATION") {
		t.Fatalf("wide view is missing expected sections")
	}
	updated, _ = updated.(model).Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	small := updated.(model).View()
	if !strings.Contains(small, "Terminal is too small") {
		t.Fatalf("small terminal fallback not rendered")
	}
}

func TestWideInteractionsStayInsidePersistentWorkspace(t *testing.T) {
	setups := []struct {
		name string
		want string
		set  func(*model)
	}{
		{
			name: "ani-cli selector",
			want: "ANI-CLI REQUEST",
			set: func(m *model) {
				req := bridgeRequest{Prompt: "Select anime:", Options: []string{"Frieren", "Cowboy Bebop"}}
				m.menu = newSelectionModal(req, nil, m.width)
				m.beginWorkflowPrompt(req)
			},
		},
		{name: "command palette", want: "COMMAND PALETTE", set: func(m *model) { m.palette = newPalette(false, m.width) }},
		{name: "confirmation", want: "CONFIRM ACTION", set: func(m *model) {
			m.confirm = &confirmState{title: "Delete watch history?", body: "Keep or delete history.", action: "delete"}
		}},
		{name: "editor", want: "EDIT VALUE", set: func(m *model) {
			m.openLaunchEditor("launch-index", "Search result index", "Example: 2")
		}},
		{name: "help", want: "KEYBOARD-FIRST", set: func(m *model) { m.help = true }},
	}

	for _, tc := range setups {
		t.Run(tc.name, func(t *testing.T) {
			m := model{width: 120, height: 32, cfg: defaultConfig(), aniVersion: "5.0.2"}
			tc.set(&m)
			view := ansi.Strip(m.View())
			for _, persistent := range []string{"ANI//TUI", "NAVIGATION", "Discover", "SESSION FLOW", tc.want} {
				if !strings.Contains(view, persistent) {
					t.Fatalf("workspace lost %q while rendering %s\n%s", persistent, tc.name, view)
				}
			}
			if lines := len(strings.Split(view, "\n")); lines != m.height {
				t.Fatalf("workspace height = %d lines, want %d", lines, m.height)
			}
		})
	}
}

func TestCompactInteractionRetainsWorkspaceContext(t *testing.T) {
	m := model{width: 72, height: 24, cfg: defaultConfig(), aniVersion: "5.0.2", running: true}
	req := bridgeRequest{Prompt: "Select episode:", Options: []string{"Episode 1", "Episode 2", "Episode 3"}}
	m.workflow = []workflowStep{{label: "Anime search", detail: "Frieren", state: "done"}}
	m.menu = newSelectionModal(req, nil, m.width)
	m.beginWorkflowPrompt(req)

	view := ansi.Strip(m.View())
	for _, want := range []string{"ANI//TUI", "WORKSPACE", "[Discover]", "History", "Settings", "Activity", "SESSION", "Select episode", "ANI-CLI REQUEST"} {
		if !strings.Contains(view, want) {
			t.Fatalf("compact workspace is missing %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "NAVIGATION") {
		t.Fatal("compact workspace rendered the wide context pane")
	}
	if lines := len(strings.Split(view, "\n")); lines != m.height {
		t.Fatalf("compact workspace height = %d lines, want %d", lines, m.height)
	}
}

func TestWorkflowTracksAniCLIChoicesAndCompletion(t *testing.T) {
	m := model{}
	req := bridgeRequest{Prompt: "Select anime:", Options: []string{"Frieren", "Cowboy Bebop"}}
	m.workflow = []workflowStep{{label: "Anime search", detail: "frieren", state: "done"}}
	m.beginWorkflowPrompt(req)
	if got := m.workflow[len(m.workflow)-1]; got.state != "active" || got.detail != "2 choices available" {
		t.Fatalf("active workflow step = %#v", got)
	}
	m.completeWorkflowStep("1\tFrieren", "done")
	m.finishWorkflow("Ready", "ani-cli finished successfully", "done")
	if got := m.workflow[len(m.workflow)-2]; got.state != "done" || got.detail != "1 Frieren" {
		t.Fatalf("completed choice = %#v", got)
	}
	if got := m.workflow[len(m.workflow)-1]; got.label != "Ready" || got.state != "done" {
		t.Fatalf("final workflow step = %#v", got)
	}
}

func TestMinimumSupportedWorkspaceKeepsActiveRequestVisible(t *testing.T) {
	m := model{width: 54, height: 16, cfg: defaultConfig(), aniVersion: "5.0.2"}
	req := bridgeRequest{Prompt: "Select anime:", Options: []string{"Frieren", "Cowboy Bebop"}}
	m.menu = newSelectionModal(req, nil, m.width)
	m.beginWorkflowPrompt(req)
	view := ansi.Strip(m.View())
	for _, want := range []string{"ANI//TUI", "WORKSPACE", "SESSION", "ANI-CLI REQUEST", "Select anime", "Frieren"} {
		if !strings.Contains(view, want) {
			t.Fatalf("minimum workspace is missing %q\n%s", want, view)
		}
	}
	if lines := len(strings.Split(view, "\n")); lines != m.height {
		t.Fatalf("minimum workspace height = %d lines, want %d", lines, m.height)
	}
}

func TestCompactCompletedSessionRemainsResponsive(t *testing.T) {
	for _, width := range []int{54, 67, 87} {
		m := model{width: width, height: 20, cfg: defaultConfig(), aniVersion: "5.0.2"}
		m.workflow = []workflowStep{{label: "A completed anime session", detail: "Frieren", state: "done"}}
		m.help = true
		view := ansi.Strip(m.View())
		for _, want := range []string{"WORKSPACE", "SESSION", "A completed anime session", "KEYBOARD-FIRST"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%d-column workspace is missing %q\n%s", width, want, view)
			}
		}
		if lines := len(strings.Split(view, "\n")); lines != m.height {
			t.Fatalf("%d-column workspace height = %d lines, want %d", width, lines, m.height)
		}
	}
}

func TestLoadingPulseMovesSubtly(t *testing.T) {
	m := model{running: true, animationFrame: 1}
	first := ansi.Strip(m.renderProgressPulse(12))
	m.animationFrame++
	second := ansi.Strip(m.renderProgressPulse(12))
	if first == second {
		t.Fatalf("loading pulse did not move: %q", first)
	}
	if len([]rune(first)) != 12 || len([]rune(second)) != 12 {
		t.Fatalf("loading pulse changed width: %q / %q", first, second)
	}
}

func TestSidebarRouteClickWorksDuringInlineInteraction(t *testing.T) {
	m := model{width: 120, height: 32, cfg: defaultConfig()}
	m.palette = newPalette(false, m.width)
	updated, _ := m.updateMouse(tea.MouseMsg{X: 5, Y: 6, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	got := updated.(model)
	if got.route != routeSettings {
		t.Fatalf("route = %v, want settings", got.route)
	}
	if got.palette == nil {
		t.Fatal("inline interaction was discarded when context route changed")
	}
}

func playbackMenuRequest() bridgeRequest {
	return bridgeRequest{
		Prompt:  "Playing episode 4 of Frieren... ",
		Options: []string{"next", "replay", "previous", "select", "change_quality", "quit"},
	}
}

func TestPlaybackMenuIsRenderedAsNativeEpisodeControls(t *testing.T) {
	req := playbackMenuRequest()
	menu := newSelectionModal(req, nil, 100)
	if !menu.playback {
		t.Fatal("ani-cli post-play actions were not recognized as playback controls")
	}
	m := model{width: 100, height: 30, cfg: defaultConfig(), menu: menu}
	m.beginWorkflowPrompt(req)
	if got := m.workflow[len(m.workflow)-1]; got.label != "Playback controls" || got.detail != "Choose what ani-cli does next" {
		t.Fatalf("playback workflow step = %#v", got)
	}
	view := ansi.Strip(m.renderSelectionModal(60, 24))
	for _, want := range []string{
		"PLAYBACK CONTROLS",
		"Next episode",
		"Replay episode",
		"Previous episode",
		"Change quality",
		"Quit playback",
		"n next",
		"q quit",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("playback controls are missing %q\n%s", want, view)
		}
	}
}

func TestPlaybackShortcutsReturnExactAniCLIActions(t *testing.T) {
	cases := []struct {
		key  rune
		want string
	}{
		{'n', "next"},
		{'r', "replay"},
		{'p', "previous"},
		{'c', "change_quality"},
		{'q', "quit"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			m := model{menu: newSelectionModal(playbackMenuRequest(), server, 100)}
			updated, cmd := m.updateMenu(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tc.key}})
			if cmd == nil {
				t.Fatalf("shortcut %q did not produce a bridge reply", tc.key)
			}
			if got := updated.(model); got.menu != nil {
				t.Fatalf("shortcut %q left the playback selector open", tc.key)
			}
			done := make(chan struct{})
			go func() {
				_ = cmd()
				close(done)
			}()
			_ = client.SetReadDeadline(time.Now().Add(time.Second))
			var response bridgeResponse
			if err := json.NewDecoder(client).Decode(&response); err != nil {
				t.Fatal(err)
			}
			<-done
			if response.Canceled {
				t.Fatalf("shortcut %q canceled the ani-cli request", tc.key)
			}
			if len(response.Selected) != 1 || response.Selected[0] != tc.want {
				t.Fatalf("shortcut %q selected %#v, want exact ani-cli action %q", tc.key, response.Selected, tc.want)
			}
		})
	}
}

func TestRepeatedPlaybackMenusRemainInOneWorkflow(t *testing.T) {
	m := model{}
	req := playbackMenuRequest()
	m.beginWorkflowPrompt(req)
	m.completeWorkflowStep("Next episode", "done")
	m.beginWorkflowPrompt(req)
	if len(m.workflow) != 2 {
		t.Fatalf("workflow contains %d steps, want 2", len(m.workflow))
	}
	if m.workflow[0].state != "done" || m.workflow[0].detail != "Next episode" {
		t.Fatalf("first playback action = %#v", m.workflow[0])
	}
	if m.workflow[1].state != "active" || m.workflow[1].label != "Playback controls" {
		t.Fatalf("repeated playback request = %#v", m.workflow[1])
	}
}
