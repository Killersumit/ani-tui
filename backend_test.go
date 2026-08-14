package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildArgsCoversAniCLIFeatures(t *testing.T) {
	cfg := defaultConfig()
	cfg.Quality = "720"
	cfg.Mode = "dub"
	cfg.Action = "download"
	cfg.SkipIntro = true
	cfg.NoDetach = true
	cfg.ExitAfterPlay = true
	cfg.Player = "vlc"

	got := buildArgs(cfg, commandSpec{Operation: opSearch, Query: "Cowboy Bebop"})
	want := []string{"-q", "720", "--dub", "--skip", "--no-detach", "--exit-after-play", "--vlc", "-d", "Cowboy Bebop"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildArgs() = %#v, want %#v", got, want)
	}

	cases := []struct {
		spec commandSpec
		want []string
	}{
		{commandSpec{Operation: opContinue}, []string{"-q", "720", "--dub", "--skip", "--no-detach", "--exit-after-play", "--vlc", "-d", "-c"}},
		{commandSpec{Operation: opCountdown, Query: "Frieren"}, []string{"-q", "720", "--dub", "--skip", "--no-detach", "--exit-after-play", "--vlc", "-d", "-N", "Frieren"}},
		{commandSpec{Operation: opSearch, Query: "Frieren", Episode: "2-4", Index: "3"}, []string{"-q", "720", "--dub", "--skip", "--no-detach", "--exit-after-play", "--vlc", "-d", "-e", "2-4", "-S", "3", "Frieren"}},
		{commandSpec{Operation: opUpdate}, []string{"-U"}},
		{commandSpec{Operation: opDelete}, []string{"-D"}},
		{commandSpec{Operation: opLogs}, []string{"-l"}},
	}
	for _, tc := range cases {
		if got := buildArgs(cfg, tc.spec); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("buildArgs(%q) = %#v, want %#v", tc.spec.Operation, got, tc.want)
		}
	}
}

func TestBuildEnvUsesBridgeAndPlayerConfiguration(t *testing.T) {
	cfg := defaultConfig()
	cfg.CustomPlayer = "my-player"
	cfg.Player = "custom"
	cfg.PlayerFlags = "--profile=anime"
	cfg.DownloadDir = "/tmp/anime"
	cfg.HistoryDir = "/tmp/history"
	cfg.UpdateBranch = "dev"
	cfg.LogPlayback = false
	bridge := &bridgeServer{address: "127.0.0.1:4567", token: "secret"}

	env := buildEnv(cfg, bridge, "/tmp/ani tui")
	joined := strings.Join(env, "\n")
	for _, expected := range []string{
		"ANI_TUI_MENU_HELPER=1",
		"ANI_TUI_BRIDGE_ADDR=127.0.0.1:4567",
		"ANI_TUI_BRIDGE_TOKEN=secret",
		"ANI_CLI_PLAYER=my-player",
		"ANI_CLI_PLAYER_FLAGS=--profile=anime",
		"ANI_CLI_DOWNLOAD_DIR=/tmp/anime",
		"ANI_CLI_HIST_DIR=/tmp/history",
		"ANI_CLI_BRANCH=dev",
		"ANI_CLI_LOG=0",
	} {
		if !strings.Contains(joined, expected) {
			t.Errorf("environment does not contain %q", expected)
		}
	}
	if !strings.Contains(joined, "ANI_CLI_MENU=/tmp/ani tui") {
		t.Errorf("menu executable path was not preserved: %s", joined)
	}
}

func TestStripANSI(t *testing.T) {
	input := "\x1b[31merror\x1b[0m\r\nnext\x1b]0;title\x07"
	if got := stripANSI(input); got != "error\nnext" {
		t.Fatalf("stripANSI() = %q", got)
	}
}

func TestMenuHelperProtocol(t *testing.T) {
	if os.Getenv("ANI_TUI_TEST_HELPER") == "1" {
		os.Exit(runMenuHelper())
	}
	bridge, err := newBridgeServer()
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()

	cmd := exec.Command(os.Args[0], "-test.run=TestMenuHelperProtocol", "--", "Search Anime")
	cmd.Env = append(os.Environ(),
		"ANI_TUI_TEST_HELPER=1",
		"ANI_TUI_BRIDGE_ADDR="+bridge.address,
		"ANI_TUI_BRIDGE_TOKEN="+bridge.token,
	)
	cmd.Stdin = strings.NewReader("first row\nsecond row\n")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	msg := waitForBridge(bridge)()
	request, ok := msg.(menuRequestMsg)
	if !ok {
		t.Fatalf("bridge message = %T (%v)", msg, msg)
	}
	if request.Request.Prompt != "Search Anime" {
		t.Errorf("prompt = %q", request.Request.Prompt)
	}
	if !reflect.DeepEqual(request.Request.Options, []string{"first row", "second row"}) {
		t.Errorf("options = %#v", request.Request.Options)
	}
	if err := jsonReplyNow(request.Conn, bridgeResponse{Selected: []string{"second row"}}); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("helper failed: %v: %s", err, stdout.String())
	}
	if got := stdout.String(); got != "second row" {
		t.Fatalf("helper stdout = %q", got)
	}
}

func TestProcessManagerWithMockAniCLI(t *testing.T) {
	if os.Getenv("ANI_TUI_MOCK_HELPER") == "1" {
		os.Exit(runMenuHelper())
	}
	temp := t.TempDir()
	testBinary, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(temp, "menu-helper")
	helperScript := fmt.Sprintf("#!/bin/sh\nANI_TUI_MOCK_HELPER=1 exec %q -test.run=TestProcessManagerWithMockAniCLI -- \"$@\"\n", testBinary)
	writeExecutable(t, helper, helperScript)

	mock := filepath.Join(temp, "ani-cli")
	mockScript := `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "Version: 5.0.2"
  exit 0
fi
search=$(printf 'Cowboy Bebop\nFrieren: Beyond Journey End\n' | $ANI_CLI_MENU 'Search Anime') || exit 21
echo "search=$search"
episodes=$(printf 'Episode 1\nEpisode 2\nEpisode 3\n' | $ANI_CLI_MENU 'Select Episode [1-3]') || exit 22
printf 'episodes=%s\n' "$episodes"
action=$(printf 'next\nreplay\nprevious\nselect\nchange_quality\nquit\n' | $ANI_CLI_MENU 'Playing episode 1 of Cowboy Bebop... ') || exit 23
echo "action=$action"
action2=$(printf 'next\nreplay\nprevious\nselect\nchange_quality\nquit\n' | $ANI_CLI_MENU 'Playing episode 2 of Cowboy Bebop... ') || exit 24
echo "action2=$action2"
`
	writeExecutable(t, mock, mockScript)

	bridge, err := newBridgeServer()
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()
	pm := newProcessManager()
	cfg := defaultConfig()
	cfg.AniCLIPath = mock

	cmd := pm.startCmd(cfg, bridge, helper, commandSpec{Operation: opSearch, Query: "bebop"})
	if msg := cmd(); msg != nil {
		t.Fatalf("start command unexpectedly returned %T", msg)
	}

	done := make(chan processDoneMsg, 1)
	var output []string
	go func() {
		for {
			msg := (<-pm.events).(interface{})
			switch value := msg.(type) {
			case processLineMsg:
				output = append(output, value.Line)
			case processDoneMsg:
				done <- value
				return
			}
		}
	}()

	selections := [][]string{{"Cowboy Bebop"}, {"Episode 1", "Episode 3"}, {"next"}, {"quit"}}
	for i, selected := range selections {
		requestMsg := waitForBridgeWithTimeout(t, bridge)
		if i >= 2 && !isPlaybackMenu(requestMsg.Request.Options) {
			t.Fatalf("request %d did not preserve ani-cli playback actions: %#v", i, requestMsg.Request.Options)
		}
		if err := jsonReplyNow(requestMsg.Conn, bridgeResponse{Selected: selected}); err != nil {
			t.Fatalf("reply %d: %v", i, err)
		}
	}

	select {
	case result := <-done:
		if result.Err != nil {
			t.Fatalf("mock ani-cli failed: %v; output=%q", result.Err, output)
		}
	case <-time.After(5 * time.Second):
		pm.cancel()
		t.Fatal("timed out waiting for mock ani-cli")
	}
	joined := strings.Join(output, "\n")
	for _, expected := range []string{"search=Cowboy Bebop", "episodes=Episode 1", "Episode 3", "action=next", "action2=quit"} {
		if !strings.Contains(joined, expected) {
			t.Errorf("output %q does not contain %q", joined, expected)
		}
	}
}

func waitForBridgeWithTimeout(t *testing.T, bridge *bridgeServer) menuRequestMsg {
	t.Helper()
	result := make(chan teaMessage, 1)
	go func() { result <- waitForBridge(bridge)() }()
	select {
	case msg := <-result:
		request, ok := msg.(menuRequestMsg)
		if !ok {
			t.Fatalf("bridge result = %T (%v)", msg, msg)
		}
		return request
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for menu request")
		return menuRequestMsg{}
	}
}

type teaMessage interface{}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestExecutableStatusForcesDependencyFreeDebugPlayer(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ani-cli")
	content := `#!/bin/sh
[ "$ANI_CLI_PLAYER" = debug ] || exit 42
[ "$1" = -V ] || exit 43
printf '5.0.2\n'
`
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	version, err := executableStatus(script)
	if err != nil {
		t.Fatal(err)
	}
	if version != "5.0.2" {
		t.Fatalf("version = %q", version)
	}
}

func TestShellDisplay(t *testing.T) {
	got := shellDisplay("/tmp/ani tui", []string{"-q", "best", "O'Reilly"})
	want := "'/tmp/ani tui' -q best 'O'\\''Reilly'"
	if got != want {
		t.Fatalf("shellDisplay() = %q, want %q", got, want)
	}
}

func TestBridgeRejectsWrongToken(t *testing.T) {
	bridge, err := newBridgeServer()
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()
	conn, err := net.Dial("tcp", bridge.address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = fmt.Fprintln(conn, `{"token":"wrong","prompt":"x","options":["y"]}`)
	msg := waitForBridge(bridge)()
	if _, ok := msg.(bridgeErrorMsg); !ok {
		t.Fatalf("wrong token produced %T, want bridgeErrorMsg", msg)
	}
}
