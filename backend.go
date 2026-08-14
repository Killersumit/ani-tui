package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

type bridgeRequest struct {
	Token   string   `json:"token"`
	Prompt  string   `json:"prompt"`
	Options []string `json:"options"`
}

type bridgeResponse struct {
	Selected []string `json:"selected,omitempty"`
	Input    string   `json:"input,omitempty"`
	Canceled bool     `json:"canceled,omitempty"`
}

type menuRequestMsg struct {
	Request bridgeRequest
	Conn    net.Conn
}

type bridgeErrorMsg struct{ Err error }

type bridgeServer struct {
	listener net.Listener
	address  string
	token    string
}

func newBridgeServer() (*bridgeServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		_ = ln.Close()
		return nil, err
	}
	return &bridgeServer{listener: ln, address: ln.Addr().String(), token: hex.EncodeToString(buf)}, nil
}

func (s *bridgeServer) close() {
	if s != nil && s.listener != nil {
		_ = s.listener.Close()
	}
}

func waitForBridge(s *bridgeServer) tea.Cmd {
	return func() tea.Msg {
		conn, err := s.listener.Accept()
		if err != nil {
			return bridgeErrorMsg{Err: err}
		}
		var req bridgeRequest
		if err := json.NewDecoder(io.LimitReader(conn, 4<<20)).Decode(&req); err != nil {
			_ = conn.Close()
			return bridgeErrorMsg{Err: fmt.Errorf("decode ani-cli menu request: %w", err)}
		}
		if req.Token != s.token {
			_ = json.NewEncoder(conn).Encode(bridgeResponse{Canceled: true})
			_ = conn.Close()
			return bridgeErrorMsg{Err: errors.New("rejected menu request with an invalid token")}
		}
		return menuRequestMsg{Request: req, Conn: conn}
	}
}

func encodeBridgeResponse(writer io.Writer, response bridgeResponse) error {
	return json.NewEncoder(writer).Encode(response)
}

func replyToBridge(conn net.Conn, response bridgeResponse) tea.Cmd {
	return func() tea.Msg {
		if conn == nil {
			return nil
		}
		err := encodeBridgeResponse(conn, response)
		_ = conn.Close()
		if err != nil {
			return bridgeErrorMsg{Err: err}
		}
		return nil
	}
}

// runMenuHelper is the tiny process ani-cli invokes through ANI_CLI_MENU.
// Stdin is ani-cli's candidate list; the final argv value is its prompt.
func runMenuHelper() int {
	address := os.Getenv("ANI_TUI_BRIDGE_ADDR")
	token := os.Getenv("ANI_TUI_BRIDGE_TOKEN")
	if address == "" || token == "" {
		fmt.Fprintln(os.Stderr, "ani-tui menu bridge is missing its connection settings")
		return 2
	}
	prompt := "Select: "
	if len(os.Args) > 1 {
		prompt = os.Args[len(os.Args)-1]
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 4<<20))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	text := strings.TrimRight(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	var options []string
	if text != "" {
		options = strings.Split(text, "\n")
	}
	conn, err := net.Dial("tcp", address)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ani-tui menu bridge:", err)
		return 2
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(bridgeRequest{Token: token, Prompt: prompt, Options: options}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	var response bridgeResponse
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if response.Canceled {
		return 1
	}
	if len(options) == 0 {
		fmt.Print(response.Input)
		return 0
	}
	for i, item := range response.Selected {
		if i > 0 {
			fmt.Println()
		}
		fmt.Print(item)
	}
	return 0
}

type operation string

const (
	opSearch    operation = "search"
	opContinue  operation = "continue"
	opCountdown operation = "countdown"
	opUpdate    operation = "update"
	opDelete    operation = "delete-history"
	opLogs      operation = "logs"
)

type commandSpec struct {
	Operation operation
	Query     string
	Episode   string
	Index     string
}

type processStartedMsg struct {
	Command string
}

type processLineMsg struct {
	Line  string
	IsErr bool
}

type processDoneMsg struct {
	Err error
}

type processManager struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	running bool
	events  chan tea.Msg
}

func newProcessManager() *processManager {
	return &processManager{events: make(chan tea.Msg, 1024)}
}

func waitForProcessEvent(pm *processManager) tea.Cmd {
	return func() tea.Msg { return <-pm.events }
}

func (pm *processManager) startCmd(cfg Config, bridge *bridgeServer, selfPath string, spec commandSpec) tea.Cmd {
	return func() tea.Msg {
		pm.mu.Lock()
		if pm.running {
			pm.mu.Unlock()
			pm.emit(processDoneMsg{Err: errors.New("ani-cli is already running")}, true)
			return nil
		}

		args := buildArgs(cfg, spec)
		cmd := exec.Command(cfg.AniCLIPath, args...)
		// Keeping stdin attached makes ani-cli preserve ANI_CLI_MENU even on a
		// machine that has rofi/dmenu installed. The parent script does not read
		// stdin because ani-tui always supplies search text itself.
		cmd.Stdin = os.Stdin
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			pm.mu.Unlock()
			pm.emit(processDoneMsg{Err: err}, true)
			return nil
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			pm.mu.Unlock()
			pm.emit(processDoneMsg{Err: err}, true)
			return nil
		}
		cmd.Env = buildEnv(cfg, bridge, selfPath)
		pm.cmd = cmd
		pm.running = true
		pm.mu.Unlock()

		if err := cmd.Start(); err != nil {
			pm.mu.Lock()
			pm.running = false
			pm.cmd = nil
			pm.mu.Unlock()
			pm.emit(processDoneMsg{Err: err}, true)
			return nil
		}

		pm.emit(processStartedMsg{Command: shellDisplay(cfg.AniCLIPath, args)}, false)
		go pm.consume(cmd, stdout, stderr)
		return nil
	}
}

func (pm *processManager) consume(cmd *exec.Cmd, stdout, stderr io.Reader) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		pm.scan(stdout, false)
	}()
	go func() {
		defer wg.Done()
		pm.scan(stderr, true)
	}()
	// Drain both pipes before Wait closes them. The child can still exit and
	// deliver EOF through the kernel pipes, while the concurrent scanners keep
	// large outputs from blocking it. Calling Wait first can race the final
	// lines and surface spurious "file already closed" scanner errors.
	wg.Wait()
	err := cmd.Wait()
	pm.mu.Lock()
	pm.running = false
	pm.cmd = nil
	pm.mu.Unlock()
	pm.emit(processDoneMsg{Err: err}, true)
}

func (pm *processManager) scan(reader io.Reader, isErr bool) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(stripANSI(scanner.Text()))
		if line != "" {
			pm.emit(processLineMsg{Line: line, IsErr: isErr}, false)
		}
	}
	if err := scanner.Err(); err != nil {
		pm.emit(processLineMsg{Line: "output reader: " + err.Error(), IsErr: true}, false)
	}
}

func (pm *processManager) emit(msg tea.Msg, essential bool) {
	if essential {
		pm.events <- msg
		return
	}
	select {
	case pm.events <- msg:
	default:
	}
}

func (pm *processManager) cancel() {
	pm.mu.Lock()
	cmd := pm.cmd
	pm.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func buildArgs(cfg Config, spec commandSpec) []string {
	if spec.Operation == opUpdate {
		args := []string{"-U"}
		if strings.TrimSpace(cfg.UpdateBranch) != "" && cfg.UpdateBranch != "master" {
			args = append(args, cfg.UpdateBranch)
		}
		return args
	}
	if spec.Operation == opDelete {
		return []string{"-D"}
	}
	if spec.Operation == opLogs {
		return []string{"-l"}
	}

	args := []string{"-q", cfg.Quality}
	if cfg.Mode == "dub" {
		args = append(args, "--dub")
	}
	if cfg.SkipIntro {
		args = append(args, "--skip")
	}
	if cfg.NoDetach {
		args = append(args, "--no-detach")
	}
	if cfg.ExitAfterPlay {
		args = append(args, "--exit-after-play")
	}
	switch cfg.Player {
	case "vlc":
		args = append(args, "--vlc")
	case "syncplay":
		args = append(args, "--syncplay")
	}
	// Player switches and -d all replace ani-cli's active player function. Keep
	// the requested TUI action last so download mode cannot be accidentally
	// overridden by a persisted VLC or Syncplay preference.
	if cfg.Action == "download" {
		args = append(args, "-d")
	}
	if strings.TrimSpace(spec.Episode) != "" {
		args = append(args, "-e", strings.TrimSpace(spec.Episode))
	}
	if strings.TrimSpace(spec.Index) != "" {
		args = append(args, "-S", strings.TrimSpace(spec.Index))
	}
	switch spec.Operation {
	case opContinue:
		args = append(args, "-c")
	case opCountdown:
		args = append(args, "-N", strings.TrimSpace(spec.Query))
	default:
		args = append(args, strings.TrimSpace(spec.Query))
	}
	return args
}

func buildEnv(cfg Config, bridge *bridgeServer, selfPath string) []string {
	env := append([]string{}, os.Environ()...)
	env = append(env,
		"ANI_CLI_MENU="+selfPath,
		"ANI_TUI_MENU_HELPER=1",
		"ANI_TUI_BRIDGE_ADDR="+bridge.address,
		"ANI_TUI_BRIDGE_TOKEN="+bridge.token,
		"ANI_CLI_QUALITY="+cfg.Quality,
		"ANI_CLI_MODE="+cfg.Mode,
		"ANI_CLI_DOWNLOAD_DIR="+cfg.DownloadDir,
		"ANI_CLI_PLAYER_FLAGS="+cfg.PlayerFlags,
		"ANI_CLI_LOG="+boolNumber(cfg.LogPlayback),
		"ANI_CLI_BRANCH="+cfg.UpdateBranch,
		"ANI_CLI_NO_DETACH="+boolNumber(cfg.NoDetach),
		"ANI_CLI_EXIT_AFTER_PLAY="+boolNumber(cfg.ExitAfterPlay),
		"ANI_CLI_SKIP_INTRO="+boolNumber(cfg.SkipIntro),
		"SYSTEMD_PAGER=cat",
		"PAGER=cat",
	)
	if strings.TrimSpace(cfg.HistoryDir) != "" {
		env = append(env, "ANI_CLI_HIST_DIR="+cfg.HistoryDir)
	}
	switch cfg.Player {
	case "mpv", "debug", "catt":
		env = append(env, "ANI_CLI_PLAYER="+cfg.Player)
	case "custom":
		env = append(env, "ANI_CLI_PLAYER="+cfg.CustomPlayer)
	}
	return env
}

func boolNumber(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

var shellSafePattern = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)

func shellDisplay(binary string, args []string) string {
	all := append([]string{binary}, args...)
	parts := make([]string, 0, len(all))
	for _, arg := range all {
		if shellSafePattern.MatchString(arg) {
			parts = append(parts, arg)
		} else {
			parts = append(parts, "'"+strings.ReplaceAll(arg, "'", "'\\''")+"'")
		}
	}
	return strings.Join(parts, " ")
}

var ansiPattern = regexp.MustCompile(`(?:\x1B\][^\x07]*(?:\x07|\x1B\\))|(?:\x1B\[[0-?]*[ -/]*[@-~])`)

func stripANSI(value string) string {
	return strings.ReplaceAll(ansiPattern.ReplaceAllString(value, ""), "\r", "")
}

func executableStatus(path string) (string, error) {
	resolved, err := exec.LookPath(path)
	if err != nil {
		return "", err
	}
	cmd := exec.Command(resolved, "-V")
	// ani-cli chooses and validates its default player before it parses -V.
	// Debug mode has no external player dependency, so version detection must
	// force it rather than incorrectly reporting a valid install as missing.
	cmd.Env = append(os.Environ(), "ANI_CLI_PLAYER=debug")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return resolved, err
	}
	return strings.TrimSpace(string(out)), nil
}

func platformName() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}
