package main

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type route int

const (
	routeHome route = iota
	routeHistory
	routeSettings
	routeActivity
)

type statusMsg struct {
	Version string
	Path    string
	Err     error
}

type toastClearMsg struct{ ID int }

type selectionModal struct {
	prompt       string
	original     []string
	filtered     []string
	cursor       int
	offset       int
	multi        bool
	playback     bool
	selected     map[int]bool // indexes into original
	filter       textinput.Model
	filterActive bool
	inputMode    bool
	conn         net.Conn
}

type paletteItem struct {
	id    string
	title string
	desc  string
}

type paletteState struct {
	items    []paletteItem
	filtered []paletteItem
	cursor   int
	offset   int
	filter   textinput.Model
}

type confirmState struct {
	title  string
	body   string
	action string
}

type editState struct {
	field string
	label string
	input textinput.Model
}

type logLine struct {
	text  string
	isErr bool
}

type workflowStep struct {
	label  string
	detail string
	state  string // done, active, canceled, or error
}

type model struct {
	cfg       Config
	bridge    *bridgeServer
	pm        *processManager
	selfPath  string
	width     int
	height    int
	route     route
	navCursor int

	search         textinput.Model
	searchMode     operation
	pendingEpisode string
	pendingIndex   string
	spinner        spinner.Model

	menu        *selectionModal
	palette     *paletteState
	confirm     *confirmState
	edit        *editState
	doctor      *doctorState
	doctorEnv   doctorEnvironment
	doctorToken int
	help        bool
	quitting    bool

	settingsCursor int
	historyCursor  int
	history        []historyEntry
	logs           []logLine
	logOffset      int
	running        bool
	canceled       bool
	command        string
	workflow       []workflowStep
	animationFrame int

	aniVersion string
	aniPath    string
	aniErr     error
	toast      string
	toastID    int
}

func newModel(cfg Config, bridge *bridgeServer, pm *processManager, selfPath string) model {
	search := textinput.New()
	search.Placeholder = "Search anime…"
	search.Prompt = "❯ "
	search.CharLimit = 180
	search.Width = 52
	search.Focus()
	search.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	search.TextStyle = lipgloss.NewStyle().Foreground(colorText)
	search.PlaceholderStyle = lipgloss.NewStyle().Foreground(colorMuted)

	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = lipgloss.NewStyle().Foreground(colorAccent)

	history, _ := loadHistory(cfg)
	return model{
		cfg:        cfg,
		bridge:     bridge,
		pm:         pm,
		selfPath:   selfPath,
		search:     search,
		searchMode: opSearch,
		spinner:    spin,
		history:    history,
		doctorEnv:  systemDoctorEnvironment(),
		logs:       []logLine{{text: "Ready. Search for an anime or press c to continue.", isErr: false}},
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.spinner.Tick,
		waitForBridge(m.bridge),
		waitForProcessEvent(m.pm),
		checkStatusCmd(m.cfg),
		doctorScanCmd(m.cfg, commandSpec{Operation: opSearch}, m.doctorEnv, m.doctorToken, false),
	)
}

func checkStatusCmd(cfg Config) tea.Cmd {
	return func() tea.Msg {
		version, err := executableStatus(cfg.AniCLIPath)
		path, pathErr := exec.LookPath(cfg.AniCLIPath)
		if err == nil && pathErr == nil {
			return statusMsg{Version: version, Path: path}
		}
		if err == nil {
			err = pathErr
		}
		return statusMsg{Err: err, Path: path}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.Width = clamp(workspaceContentWidth(msg.Width)-8, 24, 64)
		m.resizeInputs()
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.animationFrame++
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case statusMsg:
		m.aniVersion, m.aniPath, m.aniErr = msg.Version, msg.Path, msg.Err
		if msg.Err != nil {
			return m.withToast("ani-cli not found — open Settings to configure its path")
		}
		return m, nil
	case toastClearMsg:
		if msg.ID == m.toastID {
			m.toast = ""
		}
		return m, nil
	case doctorScanMsg:
		if msg.Token != m.doctorToken {
			return m, nil
		}
		if len(msg.Issues) == 0 {
			m.doctor = nil
			if msg.Blocking {
				return m, m.startOperationNow(msg.Spec)
			}
			return m, nil
		}
		var pending *commandSpec
		if msg.Blocking {
			spec := msg.Spec
			pending = &spec
		}
		m.doctor = &doctorState{active: true, blocking: msg.Blocking, token: msg.Token, phase: doctorOffering, issues: msg.Issues, pending: pending}
		m.search.Blur()
		return m.advanceDoctor()
	case doctorInstallMsg:
		return m.handleDoctorInstall(msg)
	case doctorRetryMsg:
		if m.doctor == nil || msg.Token != m.doctorToken || msg.Index != m.doctor.index || m.doctor.phase != doctorBackoffWait {
			return m, nil
		}
		issue := m.doctor.current()
		if issue == nil || issue.Plan == nil {
			return m.advanceDoctor()
		}
		m.doctor.phase = doctorInstalling
		m.doctor.attempt++
		m.logs = appendLog(m.logs, logLine{text: fmt.Sprintf("Dependency install attempt %d/%d: %s", m.doctor.attempt, doctorMaxAttempts, issue.Plan.DisplayCommand())})
		return m, doctorInstallCmd(*issue.Plan, m.doctor.token, m.doctor.index, m.doctor.attempt)
	case bridgeErrorMsg:
		if !m.quitting && !errors.Is(msg.Err, net.ErrClosed) {
			m.logs = appendLog(m.logs, logLine{text: "Menu bridge: " + msg.Err.Error(), isErr: true})
			m, cmds = m.toastCmd("Menu bridge error")
		}
		if !m.quitting {
			cmds = append(cmds, waitForBridge(m.bridge))
		}
		return m, tea.Batch(cmds...)
	case menuRequestMsg:
		m.menu = newSelectionModal(msg.Request, msg.Conn, m.width)
		m.palette, m.confirm, m.edit, m.help = nil, nil, nil, false
		m.beginWorkflowPrompt(msg.Request)
		cmds = append(cmds, waitForBridge(m.bridge))
		if m.menu.inputMode || m.menu.filterActive {
			cmds = append(cmds, textinput.Blink)
		}
		return m, tea.Batch(cmds...)
	case processStartedMsg:
		m.running = true
		m.command = msg.Command
		m.logs = appendLog(m.logs, logLine{text: "$ " + msg.Command})
		cmds = append(cmds, waitForProcessEvent(m.pm))
		return m, tea.Batch(cmds...)
	case processLineMsg:
		m.logs = appendLog(m.logs, logLine{text: msg.Line, isErr: msg.IsErr})
		m.logOffset = 0
		cmds = append(cmds, waitForProcessEvent(m.pm))
		return m, tea.Batch(cmds...)
	case processDoneMsg:
		m.running = false
		m.command = ""
		if m.menu != nil {
			cmds = append(cmds, replyToBridge(m.menu.conn, bridgeResponse{Canceled: true}))
			m.menu = nil
		}
		var toastCmds []tea.Cmd
		if msg.Err != nil && !m.canceled {
			detail := friendlyProcessError(msg.Err)
			m.finishWorkflow("Stopped", detail, "error")
			m.logs = appendLog(m.logs, logLine{text: "ani-cli exited: " + detail, isErr: true})
			m, toastCmds = m.toastCmd("ani-cli stopped with an error — see Activity")
		} else if m.canceled {
			m.logs = appendLog(m.logs, logLine{text: "Canceled by user."})
			m, toastCmds = m.toastCmd("Canceled")
		} else {
			m.finishWorkflow("Ready", "ani-cli finished successfully", "done")
			m.logs = appendLog(m.logs, logLine{text: "ani-cli finished successfully."})
			m, toastCmds = m.toastCmd("Done")
		}
		cmds = append(cmds, toastCmds...)
		m.canceled = false
		m.history, _ = loadHistory(m.cfg)
		cmds = append(cmds, waitForProcessEvent(m.pm))
		return m, tea.Batch(cmds...)
	}

	if mouse, ok := msg.(tea.MouseMsg); ok {
		return m.updateMouse(mouse)
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		if key.String() == "ctrl+c" {
			if m.running {
				m.cancelProcess()
				return m, nil
			}
			m.quitting = true
			m.bridge.close()
			return m, tea.Quit
		}
		if m.doctor != nil && m.doctor.active {
			return m.updateDoctor(key)
		}
		if m.menu != nil {
			return m.updateMenu(key)
		}
		if m.edit != nil {
			return m.updateEdit(key)
		}
		if m.confirm != nil {
			return m.updateConfirm(key)
		}
		if m.help {
			if isOneOf(key.String(), "esc", "?", "enter", "q") {
				m.help = false
			}
			return m, nil
		}
		if m.palette != nil {
			return m.updatePalette(key)
		}
		if cmd := m.handleGlobalKey(key); cmd != nil {
			return m, cmd
		}
	}

	if m.route == routeHome && m.search.Focused() {
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	event := tea.MouseEvent(msg)
	if event.Action != tea.MouseActionPress {
		return m, nil
	}

	step := 0
	switch event.Button {
	case tea.MouseButtonWheelUp:
		step = -3
	case tea.MouseButtonWheelDown:
		step = 3
	}
	if step != 0 {
		switch {
		case m.menu != nil:
			m.menu.move(step)
		case m.palette != nil:
			m.palette.cursor = clamp(m.palette.cursor+step, 0, max(0, len(m.palette.filtered)-1))
		case m.doctor != nil || m.edit != nil || m.confirm != nil || m.help:
			return m, nil
		case m.route == routeHistory:
			m.historyCursor = clamp(m.historyCursor+step, 0, max(0, len(m.history)-1))
		case m.route == routeSettings:
			m.settingsCursor = clamp(m.settingsCursor+step, 0, len(settingsFields(m.cfg))-1)
		case m.route == routeActivity:
			if step < 0 {
				m.updateActivityKeys("up")
			} else {
				m.updateActivityKeys("down")
			}
		}
		return m, nil
	}

	// In the wide workspace the context pane owns the left third. Its four
	// route rows stay fixed at terminal rows 4–7 so mouse targets do not move
	// when a session adds workflow stages below them.
	contextWidth := clamp(m.width*31/100, 29, 38)
	if event.Button == tea.MouseButtonLeft && m.width >= 88 && event.X < contextWidth && event.Y >= 4 && event.Y <= 7 {
		return m, m.setRoute(route(event.Y - 4))
	}
	return m, nil
}

func (m *model) resizeInputs() {
	width := workspaceInputWidth(m.width)
	if m.menu != nil {
		m.menu.filter.Width = width
	}
	if m.palette != nil {
		m.palette.filter.Width = width
	}
	if m.edit != nil {
		m.edit.input.Width = width
	}
}

func (m *model) handleGlobalKey(key tea.KeyMsg) tea.Cmd {
	k := key.String()
	if isOneOf(k, "ctrl+p", "ctrl+k", ":") {
		m.palette = newPalette(m.running, m.width)
		m.search.Blur()
		return textinput.Blink
	}
	if k == "?" {
		m.help = true
		m.search.Blur()
		return nil
	}
	if k == "esc" {
		if m.search.Focused() {
			m.search.Blur()
			return nil
		}
		return m.setRoute(routeHome)
	}
	// Tab navigation remains global even while the Discover input owns normal
	// text keys. Returning to Discover restores focus immediately.
	if k == "tab" {
		return m.setRoute(route((int(m.route) + 1) % 4))
	}
	if k == "shift+tab" {
		return m.setRoute(route((int(m.route) + 3) % 4))
	}
	if m.search.Focused() && m.route == routeHome {
		if k == "enter" {
			return m.startSearch()
		}
		return nil
	}
	switch k {
	case "/":
		m.route, m.navCursor = routeHome, int(routeHome)
		m.searchMode = opSearch
		m.pendingEpisode, m.pendingIndex = "", ""
		m.search.Placeholder = "Search anime…"
		m.search.Focus()
		return textinput.Blink
	case "c":
		return m.startOperation(commandSpec{Operation: opContinue})
	case "1":
		return m.setRoute(routeHome)
	case "2", "h":
		return m.setRoute(routeHistory)
	case "3", "s":
		return m.setRoute(routeSettings)
	case "4", "a":
		return m.setRoute(routeActivity)
	case "q":
		if !m.running {
			m.quitting = true
			m.bridge.close()
			return tea.Quit
		}
	}

	switch m.route {
	case routeHistory:
		return m.updateHistoryKeys(k)
	case routeSettings:
		return m.updateSettingsKeys(k)
	case routeActivity:
		m.updateActivityKeys(k)
	}
	return nil
}

func (m *model) setRoute(next route) tea.Cmd {
	m.route, m.navCursor = next, int(next)
	if next == routeHistory {
		m.history, _ = loadHistory(m.cfg)
	}
	if next == routeHome && !m.hasInteraction() {
		return m.search.Focus()
	}
	m.search.Blur()
	return nil
}

func (m *model) startSearch() tea.Cmd {
	query := strings.TrimSpace(m.search.Value())
	if query == "" {
		return nil
	}
	spec := commandSpec{Operation: m.searchMode, Query: query, Episode: m.pendingEpisode, Index: m.pendingIndex}
	m.pendingEpisode, m.pendingIndex = "", ""
	return m.startOperation(spec)
}

func (m *model) startOperation(spec commandSpec) tea.Cmd {
	if m.running {
		next, cmds := m.toastCmd("ani-cli is already running")
		*m = next
		return tea.Batch(cmds...)
	}
	if (spec.Operation == opSearch || spec.Operation == opCountdown) && strings.TrimSpace(spec.Query) == "" {
		return nil
	}
	m.doctorToken++
	m.doctor = &doctorState{active: true, blocking: true, token: m.doctorToken, phase: doctorScanning, pending: &spec}
	m.search.Blur()
	return doctorScanCmd(m.cfg, spec, m.doctorEnv, m.doctorToken, true)
}

func (m *model) startOperationNow(spec commandSpec) tea.Cmd {
	m.running = true
	m.canceled = false
	m.workflow = []workflowStep{{label: operationLabel(spec.Operation), detail: operationDetail(spec), state: "done"}}
	m.route, m.navCursor = routeActivity, int(routeActivity)
	m.search.Blur()
	// Return process startup to the Bubble Tea runtime. Commands run outside the
	// update loop, while process and bridge events serialize state changes back
	// through Update.
	return m.pm.startCmd(m.cfg, m.bridge, m.selfPath, spec)
}

func (m model) updateDoctor(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.doctor == nil {
		return m, nil
	}
	switch m.doctor.phase {
	case doctorScanning:
		if isOneOf(key.String(), "esc", "n", "N") {
			m.doctorToken++
			m.doctor = nil
			if m.route == routeHome {
				m.search.Focus()
			}
			return m.withToast("Dependency check canceled")
		}
	case doctorOffering:
		issue := m.doctor.current()
		if issue == nil {
			return m.advanceDoctor()
		}
		switch key.String() {
		case "y", "Y", "enter":
			if requirementSatisfied(issue.Requirement, m.doctorEnv) {
				m.doctor.index++
				return m.advanceDoctor()
			}
			if issue.Plan == nil {
				m.doctor.declined = true
				m.doctor.index++
				return m.advanceDoctor()
			}
			if err := validateDoctorPlan(*issue.Plan); err != nil {
				m.doctor.phase = doctorFailed
				m.doctor.failure = installFailure{Kind: installFailurePermanent, Reason: err.Error()}
				return m, nil
			}
			m.doctor.phase = doctorInstalling
			m.doctor.attempt = 1
			m.logs = appendLog(m.logs, logLine{text: fmt.Sprintf("Dependency install attempt 1/%d: %s", doctorMaxAttempts, issue.Plan.DisplayCommand())})
			return m, doctorInstallCmd(*issue.Plan, m.doctor.token, m.doctor.index, m.doctor.attempt)
		case "n", "N", "esc", "q":
			m.doctor.declined = true
			m.doctor.index++
			return m.advanceDoctor()
		}
	case doctorBackoffWait:
		if isOneOf(key.String(), "esc", "n", "N", "q") {
			blocking := m.doctor.blocking
			m.doctorToken++ // invalidate the scheduled retry message
			m.doctor = nil
			if m.route == routeHome {
				m.search.Focus()
			}
			if blocking {
				return m.withToast("Dependency retry canceled — launch blocked")
			}
			return m.withToast("Dependency retry canceled")
		}
	case doctorFailed:
		if isOneOf(key.String(), "enter", "n", "N", "esc", "q") {
			m.doctor.declined = true
			m.doctor.index++
			return m.advanceDoctor()
		}
	}
	return m, nil
}

func (m model) handleDoctorInstall(msg doctorInstallMsg) (tea.Model, tea.Cmd) {
	if m.doctor == nil || msg.Token != m.doctorToken || msg.Index != m.doctor.index || msg.Attempt != m.doctor.attempt {
		return m, nil
	}
	issue := m.doctor.current()
	if issue == nil {
		return m.advanceDoctor()
	}
	m.doctor.lastOutput = conciseInstallerOutput(msg.Output)
	if msg.Err == nil {
		if requirementSatisfied(issue.Requirement, m.doctorEnv) {
			m.logs = appendLog(m.logs, logLine{text: issue.Requirement.Name + " installed and verified"})
			m.doctor.index++
			return m.advanceDoctor()
		}
		m.doctor.phase = doctorFailed
		m.doctor.failure = installFailure{Kind: installFailurePermanent, Reason: "installer exited successfully, but the required executable is still unavailable"}
		m.logs = appendLog(m.logs, logLine{text: issue.Requirement.Name + ": post-install verification failed", isErr: true})
		return m, nil
	}

	failure := classifyInstallFailure(msg.Output, msg.Err)
	m.doctor.failure = failure
	m.logs = appendLog(m.logs, logLine{text: fmt.Sprintf("Dependency install attempt %d failed: %s — %s", msg.Attempt, failure.Reason, m.doctor.lastOutput), isErr: true})
	if failure.Kind == installFailureTransient && msg.Attempt < doctorMaxAttempts {
		m.doctor.phase = doctorBackoffWait
		return m, doctorRetryCmd(m.doctor.token, m.doctor.index, msg.Attempt)
	}
	m.doctor.phase = doctorFailed
	return m, nil
}

func (m model) advanceDoctor() (tea.Model, tea.Cmd) {
	if m.doctor == nil {
		return m, nil
	}
	for m.doctor.index < len(m.doctor.issues) {
		issue := m.doctor.current()
		if issue != nil && requirementSatisfied(issue.Requirement, m.doctorEnv) {
			m.doctor.index++
			continue
		}
		m.doctor.phase = doctorOffering
		m.doctor.attempt = 0
		m.doctor.failure = installFailure{}
		m.doctor.lastOutput = ""
		return m, nil
	}

	state := m.doctor
	m.doctor = nil
	if m.route == routeHome {
		m.search.Focus()
	}
	if state.blocking && state.pending != nil {
		remaining := scanDependencies(m.cfg, *state.pending, m.doctorEnv)
		if len(remaining) == 0 {
			start := m.startOperationNow(*state.pending)
			return m, tea.Batch(start, checkStatusCmd(m.cfg))
		}
		next, cmds := m.toastCmd("Launch blocked — required dependencies are still missing")
		next.logs = appendLog(next.logs, logLine{text: "Launch blocked by unresolved dependencies.", isErr: true})
		return next, tea.Batch(append(cmds, checkStatusCmd(next.cfg))...)
	}
	return m, checkStatusCmd(m.cfg)
}

func (m *model) cancelProcess() {
	m.canceled = true
	m.finishWorkflow("Canceled", "Request stopped by you", "canceled")
	if m.menu != nil {
		_ = jsonReplyNow(m.menu.conn, bridgeResponse{Canceled: true})
		m.menu = nil
	}
	m.pm.cancel()
}

func operationLabel(op operation) string {
	switch op {
	case opContinue:
		return "Continue watching"
	case opCountdown:
		return "Countdown search"
	case opUpdate:
		return "Update ani-cli"
	case opDelete:
		return "Delete history"
	case opLogs:
		return "Playback logs"
	default:
		return "Anime search"
	}
}

func operationDetail(spec commandSpec) string {
	parts := make([]string, 0, 3)
	if value := strings.TrimSpace(spec.Query); value != "" {
		parts = append(parts, value)
	}
	if value := strings.TrimSpace(spec.Episode); value != "" {
		parts = append(parts, "episode "+value)
	}
	if value := strings.TrimSpace(spec.Index); value != "" {
		parts = append(parts, "result "+value)
	}
	if len(parts) == 0 {
		return "Request sent to ani-cli"
	}
	return strings.Join(parts, " · ")
}

func compactWorkflowText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func workflowPromptLabel(prompt string) string {
	prompt = strings.TrimSpace(strings.TrimRight(compactWorkflowText(prompt), ":"))
	if prompt == "" {
		return "ani-cli request"
	}
	return prompt
}

func (m *model) beginWorkflowPrompt(req bridgeRequest) {
	for i := len(m.workflow) - 1; i >= 0; i-- {
		if m.workflow[i].state == "active" {
			m.workflow[i].state = "done"
			break
		}
	}
	label := workflowPromptLabel(req.Prompt)
	detail := "Enter a value"
	if count := len(req.Options); count > 0 {
		detail = fmt.Sprintf("%d choices available", count)
	}
	if isPlaybackMenu(req.Options) {
		label = "Playback controls"
		detail = "Choose what ani-cli does next"
	}
	m.workflow = append(m.workflow, workflowStep{label: label, detail: detail, state: "active"})
	m.trimWorkflow()
}

func (m *model) completeWorkflowStep(detail, state string) {
	for i := len(m.workflow) - 1; i >= 0; i-- {
		if m.workflow[i].state == "active" {
			m.workflow[i].state = state
			m.workflow[i].detail = compactWorkflowText(detail)
			return
		}
	}
}

func (m *model) finishWorkflow(label, detail, state string) {
	m.completeWorkflowStep(detail, state)
	if len(m.workflow) > 0 && m.workflow[len(m.workflow)-1].label == label && m.workflow[len(m.workflow)-1].state == state {
		return
	}
	m.workflow = append(m.workflow, workflowStep{label: compactWorkflowText(label), detail: compactWorkflowText(detail), state: state})
	m.trimWorkflow()
}

func (m *model) trimWorkflow() {
	const maxSteps = 8
	if len(m.workflow) > maxSteps {
		m.workflow = append([]workflowStep(nil), m.workflow[len(m.workflow)-maxSteps:]...)
	}
}

func jsonReplyNow(conn net.Conn, response bridgeResponse) error {
	if conn == nil {
		return nil
	}
	err := writeBridgeResponse(conn, response)
	_ = conn.Close()
	return err
}

func (m model) updateMenu(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	menu := m.menu
	k := key.String()
	if menu.inputMode {
		switch k {
		case "enter":
			value := strings.TrimSpace(menu.filter.Value())
			m.menu = nil
			if value == "" {
				m.completeWorkflowStep("No value entered", "canceled")
				return m, replyToBridge(menu.conn, bridgeResponse{Canceled: true})
			}
			m.completeWorkflowStep(value, "done")
			return m, replyToBridge(menu.conn, bridgeResponse{Input: value})
		case "esc":
			m.menu = nil
			m.completeWorkflowStep("Skipped", "canceled")
			return m, replyToBridge(menu.conn, bridgeResponse{Canceled: true})
		}
		var cmd tea.Cmd
		menu.filter, cmd = menu.filter.Update(key)
		return m, cmd
	}
	if menu.filterActive {
		switch k {
		case "esc":
			menu.filterActive = false
			menu.filter.Blur()
			if menu.filter.Value() == "" {
				menu.filtered = append([]string{}, menu.original...)
			}
			return m, nil
		case "enter":
			return m.finishMenuSelection()
		case "up", "ctrl+p":
			menu.move(-1)
			return m, nil
		case "down", "ctrl+n":
			menu.move(1)
			return m, nil
		}
		var cmd tea.Cmd
		menu.filter, cmd = menu.filter.Update(key)
		menu.applyFilter()
		return m, cmd
	}
	if menu.playback {
		for _, option := range menu.original {
			shortcut, _ := playbackActionPresentation(option)
			if shortcut != "" && k == shortcut {
				return m.finishPlaybackAction(option)
			}
		}
	}
	switch k {
	case "esc", "q":
		m.menu = nil
		m.completeWorkflowStep("Skipped", "canceled")
		return m, replyToBridge(menu.conn, bridgeResponse{Canceled: true})
	case "up", "k":
		menu.move(-1)
	case "down", "j":
		menu.move(1)
	case "pgup", "ctrl+u":
		menu.move(-8)
	case "pgdown", "ctrl+d":
		menu.move(8)
	case "home", "g":
		menu.cursor = 0
	case "end", "G":
		if len(menu.filtered) > 0 {
			menu.cursor = len(menu.filtered) - 1
		}
	case "/":
		menu.filterActive = true
		menu.filter.Focus()
		return m, textinput.Blink
	case " ":
		if menu.multi && len(menu.filtered) > 0 {
			idx := menu.originalIndex(menu.filtered[menu.cursor])
			menu.selected[idx] = !menu.selected[idx]
			menu.move(1)
		}
	case "enter":
		return m.finishMenuSelection()
	}
	return m, nil
}

func (m model) finishPlaybackAction(option string) (tea.Model, tea.Cmd) {
	menu := m.menu
	m.menu = nil
	m.completeWorkflowStep(menu.displayLabel(option), "done")
	return m, replyToBridge(menu.conn, bridgeResponse{Selected: []string{option}})
}

func (m model) finishMenuSelection() (tea.Model, tea.Cmd) {
	menu := m.menu
	if len(menu.filtered) == 0 {
		return m, nil
	}
	var selected []string
	if menu.multi && len(menu.selected) > 0 {
		indexes := make([]int, 0, len(menu.selected))
		for idx, value := range menu.selected {
			if value {
				indexes = append(indexes, idx)
			}
		}
		sort.Ints(indexes)
		for _, idx := range indexes {
			if idx >= 0 && idx < len(menu.original) {
				selected = append(selected, menu.original[idx])
			}
		}
	}
	if len(selected) == 0 {
		selected = []string{menu.filtered[menu.cursor]}
	}
	m.menu = nil
	detail := menu.displayLabel(selected[0])
	if len(selected) > 1 {
		detail = fmt.Sprintf("%d selections", len(selected))
	}
	m.completeWorkflowStep(detail, "done")
	return m, replyToBridge(menu.conn, bridgeResponse{Selected: selected})
}

func newSelectionModal(req bridgeRequest, conn net.Conn, width int) *selectionModal {
	input := textinput.New()
	input.Prompt = "❯ "
	input.Placeholder = "Type a value…"
	input.CharLimit = 256
	input.Width = workspaceInputWidth(width)
	input.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	input.TextStyle = lipgloss.NewStyle().Foreground(colorText)
	input.PlaceholderStyle = lipgloss.NewStyle().Foreground(colorMuted)
	inputMode := len(req.Options) == 0
	if inputMode {
		input.Focus()
	}
	prompt := strings.TrimSpace(req.Prompt)
	lower := strings.ToLower(prompt)
	multi := strings.Contains(lower, "select episode")
	return &selectionModal{
		prompt: prompt, original: append([]string{}, req.Options...), filtered: append([]string{}, req.Options...),
		multi: multi, playback: isPlaybackMenu(req.Options), selected: make(map[int]bool), filter: input, inputMode: inputMode, conn: conn,
	}
}

func canonicalMenuAction(option string) string {
	return strings.ToLower(strings.TrimSpace(option))
}

func isPlaybackMenu(options []string) bool {
	actions := make(map[string]bool, len(options))
	for _, option := range options {
		actions[canonicalMenuAction(option)] = true
	}
	for _, required := range []string{"next", "replay", "previous", "change_quality", "quit"} {
		if !actions[required] {
			return false
		}
	}
	return true
}

func playbackActionPresentation(option string) (key, label string) {
	switch canonicalMenuAction(option) {
	case "next":
		return "n", "Next episode"
	case "replay":
		return "r", "Replay episode"
	case "previous":
		return "p", "Previous episode"
	case "select":
		return "e", "Choose episode"
	case "change_season":
		return "s", "Change season"
	case "change_quality":
		return "c", "Change quality"
	case "quit":
		return "q", "Quit playback"
	default:
		return "", option
	}
}

func (s *selectionModal) displayLabel(option string) string {
	if !s.playback {
		return option
	}
	_, label := playbackActionPresentation(option)
	return label
}

func (s *selectionModal) originalIndex(item string) int {
	for i, value := range s.original {
		if value == item {
			return i
		}
	}
	return -1
}

func (s *selectionModal) move(delta int) {
	if len(s.filtered) == 0 {
		s.cursor = 0
		return
	}
	s.cursor = clamp(s.cursor+delta, 0, len(s.filtered)-1)
}

func (s *selectionModal) applyFilter() {
	query := strings.ToLower(strings.TrimSpace(s.filter.Value()))
	s.filtered = s.filtered[:0]
	for _, option := range s.original {
		searchable := strings.ToLower(option + " " + s.displayLabel(option))
		if query == "" || strings.Contains(searchable, query) {
			s.filtered = append(s.filtered, option)
		}
	}
	s.cursor, s.offset = 0, 0
}

func newPalette(running bool, width int) *paletteState {
	items := []paletteItem{
		{id: "search", title: "Search anime", desc: "Browse and play with the current settings"},
		{id: "continue", title: "Continue watching", desc: "Open ani-cli history and resume"},
		{id: "episode", title: "Play an exact episode or range", desc: "Apply ani-cli -e/-r before searching"},
		{id: "index", title: "Select search result by index", desc: "Apply ani-cli -S and bypass the result chooser"},
		{id: "countdown", title: "Next episode countdown", desc: "Check sub and dub release times"},
		{id: "history", title: "Watch history", desc: "View locally stored ani-cli history"},
		{id: "mode", title: "Toggle sub / dub", desc: "Change the playback language mode"},
		{id: "action", title: "Toggle stream / download", desc: "Change what Enter does after episode selection"},
		{id: "settings", title: "Settings", desc: "Quality, player, skipping, paths and flags"},
		{id: "logs", title: "Open ani-cli logs", desc: "Run ani-cli --logview in the Activity panel"},
		{id: "update", title: "Update ani-cli", desc: "Use ani-cli's built-in updater"},
		{id: "delete", title: "Delete watch history", desc: "Permanently clear ani-cli history"},
		{id: "help", title: "Keyboard shortcuts", desc: "Show help"},
		{id: "quit", title: "Quit ani-tui", desc: "Restore the terminal and exit"},
	}
	if running {
		items = append([]paletteItem{{id: "cancel", title: "Cancel running ani-cli task", desc: "Stop the current backend process"}}, items...)
	}
	filter := textinput.New()
	filter.Prompt = "⌕ "
	filter.Placeholder = "Type a command…"
	filter.CharLimit = 100
	filter.Width = workspaceInputWidth(width)
	filter.Focus()
	filter.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	filter.TextStyle = lipgloss.NewStyle().Foreground(colorText)
	filter.PlaceholderStyle = lipgloss.NewStyle().Foreground(colorMuted)
	return &paletteState{items: items, filtered: append([]paletteItem{}, items...), filter: filter}
}

func (m model) updatePalette(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.palette
	switch key.String() {
	case "esc", "ctrl+p", "ctrl+k":
		m.palette = nil
		return m, nil
	case "up":
		p.cursor = clamp(p.cursor-1, 0, len(p.filtered)-1)
		return m, nil
	case "down", "ctrl+n":
		p.cursor = clamp(p.cursor+1, 0, len(p.filtered)-1)
		return m, nil
	case "enter":
		if len(p.filtered) == 0 {
			return m, nil
		}
		item := p.filtered[p.cursor]
		m.palette = nil
		return m.executePalette(item.id)
	}
	var cmd tea.Cmd
	p.filter, cmd = p.filter.Update(key)
	query := strings.ToLower(strings.TrimSpace(p.filter.Value()))
	p.filtered = p.filtered[:0]
	for _, item := range p.items {
		if query == "" || strings.Contains(strings.ToLower(item.title+" "+item.desc), query) {
			p.filtered = append(p.filtered, item)
		}
	}
	p.cursor = clamp(p.cursor, 0, max(0, len(p.filtered)-1))
	return m, cmd
}

func (m model) executePalette(id string) (tea.Model, tea.Cmd) {
	switch id {
	case "search":
		m.setRoute(routeHome)
		m.searchMode = opSearch
		m.pendingEpisode, m.pendingIndex = "", ""
		m.search.Placeholder = "Search anime…"
		m.search.Focus()
		return m, textinput.Blink
	case "continue":
		return m, m.startOperation(commandSpec{Operation: opContinue})
	case "episode":
		m.openLaunchEditor("launch-episode", "Episode or range", "Examples: 7, 1-12, or 1 3 5")
		return m, textinput.Blink
	case "index":
		m.openLaunchEditor("launch-index", "Search result index", "Example: 2")
		return m, textinput.Blink
	case "countdown":
		m.setRoute(routeHome)
		m.searchMode = opCountdown
		m.pendingEpisode, m.pendingIndex = "", ""
		m.search.Placeholder = "Anime for release countdown…"
		m.search.SetValue("")
		m.search.Focus()
		return m, textinput.Blink
	case "history":
		m.setRoute(routeHistory)
	case "mode":
		m.cfg.Mode = cycle([]string{"sub", "dub"}, m.cfg.Mode, 1)
		_ = saveConfig(m.cfg)
		return m.withToast("Mode: " + m.cfg.Mode)
	case "action":
		m.cfg.Action = cycle([]string{"stream", "download"}, m.cfg.Action, 1)
		_ = saveConfig(m.cfg)
		return m.withToast("Action: " + m.cfg.Action)
	case "settings":
		m.setRoute(routeSettings)
	case "logs":
		return m, m.startOperation(commandSpec{Operation: opLogs})
	case "update":
		m.confirm = &confirmState{title: "Update ani-cli?", body: "This runs ani-cli -U and may replace the installed script.", action: "update"}
	case "delete":
		m.confirm = &confirmState{title: "Delete watch history?", body: "This permanently clears ani-cli's local history file.", action: "delete"}
	case "help":
		m.help = true
	case "cancel":
		m.cancelProcess()
	case "quit":
		m.quitting = true
		m.bridge.close()
		return m, tea.Quit
	}
	return m, nil
}

func (m model) updateConfirm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "y", "Y", "enter":
		action := m.confirm.action
		m.confirm = nil
		if action == "update" {
			return m, m.startOperation(commandSpec{Operation: opUpdate})
		} else if action == "delete" {
			return m, m.startOperation(commandSpec{Operation: opDelete})
		}
	case "n", "N", "esc", "q":
		m.confirm = nil
	}
	return m, nil
}

func (m *model) updateHistoryKeys(k string) tea.Cmd {
	if len(m.history) == 0 {
		return nil
	}
	switch k {
	case "up", "k":
		m.historyCursor = clamp(m.historyCursor-1, 0, len(m.history)-1)
	case "down", "j":
		m.historyCursor = clamp(m.historyCursor+1, 0, len(m.history)-1)
	case "enter":
		m.search.SetValue(m.history[m.historyCursor].Title)
		m.searchMode = opSearch
		m.pendingEpisode, m.pendingIndex = "", ""
		return m.startSearch()
	case "r":
		m.history, _ = loadHistory(m.cfg)
	}
	return nil
}

func (m *model) updateActivityKeys(k string) {
	visible := max(3, m.height-11)
	maxOffset := max(0, len(m.logs)-visible)
	switch k {
	case "up", "k":
		m.logOffset = clamp(m.logOffset+1, 0, maxOffset)
	case "down", "j":
		m.logOffset = clamp(m.logOffset-1, 0, maxOffset)
	case "pgup", "ctrl+u":
		m.logOffset = clamp(m.logOffset+visible/2, 0, maxOffset)
	case "pgdown", "ctrl+d":
		m.logOffset = clamp(m.logOffset-visible/2, 0, maxOffset)
	case "end", "G":
		m.logOffset = 0
	case "x":
		m.logs = nil
	}
}

func (m *model) updateSettingsKeys(k string) tea.Cmd {
	fields := settingsFields(m.cfg)
	switch k {
	case "up", "k":
		m.settingsCursor = clamp(m.settingsCursor-1, 0, len(fields)-1)
	case "down", "j":
		m.settingsCursor = clamp(m.settingsCursor+1, 0, len(fields)-1)
	case "left", "h":
		m.cycleSetting(-1)
	case "right", "l", "enter", " ":
		if fields[m.settingsCursor].editable {
			m.openSettingEditor(fields[m.settingsCursor])
			return textinput.Blink
		}
		m.cycleSetting(1)
	case "e":
		if fields[m.settingsCursor].editable {
			m.openSettingEditor(fields[m.settingsCursor])
			return textinput.Blink
		}
	}
	return nil
}

type settingField struct {
	key      string
	label    string
	value    string
	desc     string
	editable bool
}

func settingsFields(cfg Config) []settingField {
	return []settingField{
		{key: "quality", label: "Video quality", value: cfg.Quality, desc: "best, worst, or a target resolution"},
		{key: "mode", label: "Audio mode", value: cfg.Mode, desc: "sub or dub (when a dub exists)"},
		{key: "action", label: "Default action", value: cfg.Action, desc: "stream in a player or download to disk"},
		{key: "player", label: "Player", value: cfg.Player, desc: "auto, mpv, VLC, Syncplay, debug, catt, or custom"},
		{key: "skip", label: "Skip intro", value: onOff(cfg.SkipIntro), desc: "uses ani-skip; mpv only"},
		{key: "detach", label: "Detach player", value: onOff(!cfg.NoDetach), desc: "keep the TUI available while mpv plays"},
		{key: "exit", label: "Exit after play", value: onOff(cfg.ExitAfterPlay), desc: "implies no-detach; mpv only"},
		{key: "logging", label: "Playback logging", value: onOff(cfg.LogPlayback), desc: "ANI_CLI_LOG; powers ani-cli's log viewer"},
		{key: "mouse", label: "Mouse events", value: onOff(cfg.Mouse), desc: "takes effect after restarting ani-tui"},
		{key: "download-dir", label: "Download directory", value: cfg.DownloadDir, desc: "ANI_CLI_DOWNLOAD_DIR", editable: true},
		{key: "player-flags", label: "Player flags", value: emptyLabel(cfg.PlayerFlags), desc: "extra flags passed to the player", editable: true},
		{key: "custom-player", label: "Custom player", value: cfg.CustomPlayer, desc: "used when Player is custom", editable: true},
		{key: "history-dir", label: "History directory", value: emptyDefault(cfg.HistoryDir, "(ani-cli default)"), desc: "ANI_CLI_HIST_DIR; empty uses XDG state", editable: true},
		{key: "update-branch", label: "Update branch", value: cfg.UpdateBranch, desc: "branch used by ani-cli -U", editable: true},
		{key: "ani-path", label: "ani-cli path", value: cfg.AniCLIPath, desc: "binary name or absolute path", editable: true},
	}
}

func (m *model) cycleSetting(delta int) {
	switch settingsFields(m.cfg)[m.settingsCursor].key {
	case "quality":
		m.cfg.Quality = cycle([]string{"best", "1080", "720", "480", "360", "worst"}, m.cfg.Quality, delta)
	case "mode":
		m.cfg.Mode = cycle([]string{"sub", "dub"}, m.cfg.Mode, delta)
	case "action":
		m.cfg.Action = cycle([]string{"stream", "download"}, m.cfg.Action, delta)
	case "player":
		m.cfg.Player = cycle([]string{"auto", "mpv", "vlc", "syncplay", "debug", "catt", "custom"}, m.cfg.Player, delta)
	case "skip":
		m.cfg.SkipIntro = !m.cfg.SkipIntro
	case "detach":
		m.cfg.NoDetach = !m.cfg.NoDetach
		if !m.cfg.NoDetach {
			m.cfg.ExitAfterPlay = false
		}
	case "exit":
		m.cfg.ExitAfterPlay = !m.cfg.ExitAfterPlay
		if m.cfg.ExitAfterPlay {
			m.cfg.NoDetach = true
		}
	case "logging":
		m.cfg.LogPlayback = !m.cfg.LogPlayback
	case "mouse":
		m.cfg.Mouse = !m.cfg.Mouse
	}
	_ = saveConfig(m.cfg)
	*m, _ = m.toastCmd("Settings saved")
}

func (m *model) openSettingEditor(field settingField) {
	input := textinput.New()
	input.Prompt = "❯ "
	input.CharLimit = 512
	input.Width = workspaceInputWidth(m.width)
	input.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	input.TextStyle = lipgloss.NewStyle().Foreground(colorText)
	var value string
	switch field.key {
	case "download-dir":
		value = m.cfg.DownloadDir
	case "player-flags":
		value = m.cfg.PlayerFlags
	case "custom-player":
		value = m.cfg.CustomPlayer
	case "history-dir":
		value = m.cfg.HistoryDir
	case "update-branch":
		value = m.cfg.UpdateBranch
	case "ani-path":
		value = m.cfg.AniCLIPath
	}
	input.SetValue(value)
	input.CursorEnd()
	input.Focus()
	m.edit = &editState{field: field.key, label: field.label, input: input}
}

func (m *model) openLaunchEditor(field, label, placeholder string) {
	input := textinput.New()
	input.Prompt = "❯ "
	input.Placeholder = placeholder
	input.CharLimit = 120
	input.Width = workspaceInputWidth(m.width)
	input.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	input.TextStyle = lipgloss.NewStyle().Foreground(colorText)
	input.PlaceholderStyle = lipgloss.NewStyle().Foreground(colorMuted)
	input.Focus()
	m.edit = &editState{field: field, label: label, input: input}
}

func (m model) updateEdit(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.edit = nil
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.edit.input.Value())
		if m.edit.field == "launch-episode" || m.edit.field == "launch-index" {
			if value == "" {
				return m, nil
			}
			if m.edit.field == "launch-episode" {
				m.pendingEpisode, m.pendingIndex = value, ""
				m.search.Placeholder = "Search anime for episode/range…"
			} else {
				m.pendingIndex, m.pendingEpisode = value, ""
				m.search.Placeholder = "Search anime by result index…"
			}
			m.edit = nil
			m.setRoute(routeHome)
			m.searchMode = opSearch
			m.search.SetValue("")
			m.search.Focus()
			return m, textinput.Blink
		}
		switch m.edit.field {
		case "download-dir":
			if value != "" {
				m.cfg.DownloadDir = value
			}
		case "player-flags":
			m.cfg.PlayerFlags = value
		case "custom-player":
			if value != "" {
				m.cfg.CustomPlayer = value
			}
		case "history-dir":
			m.cfg.HistoryDir = value
		case "update-branch":
			if value != "" {
				m.cfg.UpdateBranch = value
			}
		case "ani-path":
			if value != "" {
				m.cfg.AniCLIPath = value
			}
		}
		field := m.edit.field
		m.edit = nil
		_ = saveConfig(m.cfg)
		var cmds []tea.Cmd
		m, cmds = m.toastCmd("Settings saved")
		if field == "ani-path" {
			cmds = append(cmds, checkStatusCmd(m.cfg))
		}
		if field == "history-dir" {
			m.history, _ = loadHistory(m.cfg)
		}
		return m, tea.Batch(cmds...)
	}
	var cmd tea.Cmd
	m.edit.input, cmd = m.edit.input.Update(key)
	return m, cmd
}

func (m model) toastCmd(text string) (model, []tea.Cmd) {
	m.toast = text
	m.toastID++
	id := m.toastID
	return m, []tea.Cmd{tea.Tick(3*time.Second, func(time.Time) tea.Msg { return toastClearMsg{ID: id} })}
}

func (m model) withToast(text string) (tea.Model, tea.Cmd) {
	m, cmds := m.toastCmd(text)
	return m, tea.Batch(cmds...)
}

func friendlyProcessError(err error) string {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Sprintf("exit code %d", exitErr.ExitCode())
	}
	return err.Error()
}

func appendLog(lines []logLine, line logLine) []logLine {
	lines = append(lines, line)
	const maxLines = 1200
	if len(lines) > maxLines {
		lines = append([]logLine{}, lines[len(lines)-maxLines:]...)
	}
	return lines
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Starting ani-tui…"
	}
	if m.width < 54 || m.height < 16 {
		return smallTerminalView(m.width, m.height)
	}
	return m.renderShell(m.renderWorkspace())
}

func (m model) hasInteraction() bool {
	return m.doctor != nil || m.menu != nil || m.palette != nil || m.confirm != nil || m.edit != nil || m.help
}

func (m model) renderShell(content string) string {
	header := m.renderHeader()
	footer := m.renderFooter()
	bodyHeight := max(1, m.height-lipgloss.Height(header)-lipgloss.Height(footer))
	body := lipgloss.NewStyle().Width(m.width).Height(bodyHeight).MaxHeight(bodyHeight).Render(content)
	return header + "\n" + body + "\n" + footer
}

func (m model) renderHeader() string {
	logo := lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render("ANI") +
		lipgloss.NewStyle().Bold(true).Foreground(colorText).Render("//TUI")
	location := lipgloss.NewStyle().Foreground(colorMuted).Render("  /  " + strings.ToUpper(routeName(m.route)))
	badge := badgeStyle.Render(strings.ToUpper(m.cfg.Mode) + "  " + m.cfg.Quality + "  " + strings.ToUpper(m.cfg.Action))
	status := "ani-cli missing"
	statusColor := colorDanger
	if m.aniErr == nil && m.aniVersion != "" {
		status = "● ready  " + m.aniVersion
		statusColor = colorSuccess
	}
	if m.running {
		status = m.spinner.View() + " session active"
		statusColor = colorWarning
	} else if m.doctor != nil {
		status = m.spinner.View() + " dependency check"
		statusColor = colorWarning
	}
	if m.width < 68 {
		location = ""
		badge = badgeStyle.Render(strings.ToUpper(m.cfg.Mode))
		switch {
		case m.running:
			status = m.spinner.View() + " active"
		case m.doctor != nil:
			status = m.spinner.View() + " deps"
		case m.aniErr == nil && m.aniVersion != "":
			status = "● ready"
		default:
			status = "ani-cli missing"
		}
	}
	left := logo + location
	right := lipgloss.NewStyle().Foreground(statusColor).Render(status) + "  " + badge
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right)-4)
	line := "  " + left + strings.Repeat(" ", gap) + right + "  "
	return headerStyle.Width(m.width).Render(line)
}

func routeName(value route) string {
	switch value {
	case routeHistory:
		return "History"
	case routeSettings:
		return "Settings"
	case routeActivity:
		return "Activity"
	default:
		return "Discover"
	}
}

func (m model) renderFooter() string {
	left := "  ctrl+p commands   ? help   tab navigate"
	switch {
	case m.doctor != nil:
		switch m.doctor.phase {
		case doctorOffering:
			if issue := m.doctor.current(); issue != nil && issue.Plan != nil {
				left = "  y / enter install   n / esc skip this offer"
			} else {
				left = "  enter / n acknowledge   official manual link shown above"
			}
		case doctorFailed:
			left = "  enter / esc close this result   no automatic retry"
		default:
			left = "  dependency doctor is working"
		}
	case m.menu != nil:
		left = "  ↑↓ navigate   / filter   enter select   esc cancel"
		if m.menu.inputMode {
			left = "  type a value   enter submit   esc cancel"
		} else if m.menu.multi {
			left = "  ↑↓ navigate   space mark range   enter play   / filter   esc cancel"
		}
	case m.palette != nil:
		left = "  type to filter   ↑↓ navigate   enter run   esc close"
	case m.confirm != nil:
		left = "  y / enter confirm   n / esc keep things as they are"
	case m.edit != nil:
		left = "  enter save   esc discard"
	case m.help:
		left = "  esc / ? return to workspace"
	}
	if m.running {
		left += "   ctrl+c cancel session"
	}

	right := platformName() + "  "
	if m.width < 88 {
		right = ""
		switch {
		case m.doctor != nil && m.doctor.phase == doctorOffering && m.doctor.current() != nil && m.doctor.current().Plan != nil:
			left = "  y install   n skip"
		case m.doctor != nil && m.doctor.phase == doctorFailed:
			left = "  enter close   no retry"
		case m.doctor != nil:
			left = "  dependency doctor working"
		case m.menu != nil && m.menu.inputMode:
			left = "  enter submit   esc back"
		case m.menu != nil && m.menu.multi:
			left = "  ↑↓ move   space mark   enter play   esc back"
		case m.menu != nil:
			left = "  ↑↓ move   / filter   enter select   esc back"
		case m.palette != nil:
			left = "  type filter   ↑↓ move   enter run   esc close"
		case m.confirm != nil:
			left = "  y confirm   n keep"
		case m.edit != nil:
			left = "  enter save   esc discard"
		case m.help:
			left = "  esc / ? return"
		default:
			left = "  ctrl+p commands   ? help   tab navigate"
		}
	}
	if m.toast != "" {
		right = toastStyle.Render(m.toast) + "  "
	}
	if lipgloss.Width(left)+lipgloss.Width(right) >= m.width {
		right = ""
	}
	gap := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	return footerStyle.Width(m.width).Render(left + strings.Repeat(" ", gap) + right)
}

func (m model) renderWorkspace() string {
	headerHeight := lipgloss.Height(m.renderHeader())
	footerHeight := lipgloss.Height(m.renderFooter())
	bodyHeight := max(1, m.height-headerHeight-footerHeight)
	if m.width < 88 {
		return m.renderCompactWorkspace(m.width, bodyHeight)
	}

	contextWidth := clamp(m.width*31/100, 29, 38)
	activeWidth := m.width - contextWidth - 1
	context := m.renderSidebar(contextWidth, bodyHeight)
	activeInnerWidth := max(20, activeWidth-6)
	activeInnerHeight := max(3, bodyHeight-2)
	active := mainStyle.
		Width(activeInnerWidth).
		Height(activeInnerHeight).
		Render(m.renderActive(activeInnerWidth, activeInnerHeight))
	divider := dividerStyle.Render(strings.TrimSuffix(strings.Repeat("│\n", bodyHeight), "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, context, divider, active)
}

func (m model) renderCompactWorkspace(width, height int) string {
	context := m.renderCompactContext(width)
	contextHeight := lipgloss.Height(context)
	activeInnerWidth := max(20, width-4)
	activeInnerHeight := max(3, height-contextHeight-2)
	active := lipgloss.NewStyle().
		Padding(1, 2).
		Width(activeInnerWidth).
		Height(activeInnerHeight).
		Render(m.renderActive(activeInnerWidth, activeInnerHeight))
	return context + "\n" + active
}

func (m model) renderActive(width, height int) string {
	switch {
	case m.doctor != nil:
		return m.renderDoctor(width, height)
	case m.menu != nil:
		return m.renderSelectionModal(width, height)
	case m.palette != nil:
		return m.renderPalette(width, height)
	case m.confirm != nil:
		return m.renderConfirm(width, height)
	case m.edit != nil:
		return m.renderEdit(width, height)
	case m.help:
		return m.renderHelp(width, height)
	default:
		return m.renderRoute(width)
	}
}

func (m model) renderCompactContext(width int) string {
	status := "Ready for a new session"
	marker := ""
	if m.running {
		marker = m.spinner.View() + " "
		status = m.currentWorkflowLabel()
	} else if len(m.workflow) > 0 {
		marker = stageDoneStyle.Render("✓") + " "
		status = m.currentWorkflowLabel()
	}
	nav := []string{"Discover", "History", "Settings", "Activity"}
	for i := range nav {
		if i == int(m.route) {
			nav[i] = stageActiveStyle.Render("[" + nav[i] + "]")
		} else {
			nav[i] = contextLineStyle.Render(nav[i])
		}
	}
	innerWidth := max(20, width-4)
	lineOne := contextTitleStyle.Render("WORKSPACE") + "  " + strings.Join(nav, "  ")
	statusWidth := max(8, innerWidth-10-lipgloss.Width(marker))
	lineTwo := contextLineStyle.Render("SESSION  ") + marker + truncate(status, statusWidth)
	return contextPanelStyle.Width(innerWidth).Render(lineOne + "\n" + lineTwo)
}

func (m model) renderSidebar(width, height int) string {
	items := []struct{ label, icon string }{{"Discover", "⌕"}, {"History", "◷"}, {"Settings", "⚙"}, {"Activity", "≡"}}
	innerWidth := max(12, width-4)
	var b strings.Builder
	b.WriteString(sectionTitle.Render("NAVIGATION") + "\n\n")
	for i, item := range items {
		label := fmt.Sprintf(" %s  %-12s", item.icon, item.label)
		if i == int(m.route) {
			b.WriteString(navActive.Width(innerWidth).Render(label))
		} else {
			b.WriteString(navItem.Width(innerWidth).Render(label))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n" + sectionTitle.Render("SESSION FLOW") + "\n")
	status := "Ready for a new request"
	if m.running {
		status = m.spinner.View() + " ani-cli is working"
	} else if len(m.workflow) > 0 {
		status = "Session complete"
	}
	b.WriteString(stageDetailStyle.Render(status) + "\n")
	b.WriteString(m.renderProgressPulse(min(18, innerWidth)) + "\n\n")

	availableLines := max(0, height-13)
	b.WriteString(m.renderWorkflow(availableLines, innerWidth))
	return sidebarStyle.Width(innerWidth).Height(max(1, height-2)).Render(b.String())
}

func (m model) currentWorkflowLabel() string {
	if len(m.workflow) == 0 {
		return "Ready for a new session"
	}
	return m.workflow[len(m.workflow)-1].label
}

func (m model) renderProgressPulse(width int) string {
	width = max(6, width)
	if !m.running {
		return contextLineStyle.Render(strings.Repeat("·", width))
	}
	position := m.animationFrame % width
	var b strings.Builder
	for i := 0; i < width; i++ {
		if i == position {
			b.WriteString(stageActiveStyle.Render("◆"))
		} else if i == (position+width-1)%width {
			b.WriteString(lipgloss.NewStyle().Foreground(colorWarning).Render("•"))
		} else {
			b.WriteString(contextLineStyle.Render("·"))
		}
	}
	return b.String()
}

func (m model) renderWorkflow(maxLines, width int) string {
	if maxLines <= 0 {
		return ""
	}
	if len(m.workflow) == 0 {
		return workspaceHintStyle.Width(width).Render("Your ani-cli choices will stay visible here as one continuous flow.")
	}
	maxSteps := max(1, maxLines/2)
	start := max(0, len(m.workflow)-maxSteps)
	var b strings.Builder
	for i := start; i < len(m.workflow); i++ {
		step := m.workflow[i]
		icon := stageDoneStyle.Render("✓")
		switch step.state {
		case "active":
			icon = stageActiveStyle.Render(m.spinner.View())
		case "error", "canceled":
			icon = stageErrorStyle.Render("×")
		}
		b.WriteString(icon + " " + truncate(step.label, max(8, width-3)) + "\n")
		if maxLines > 1 {
			b.WriteString("  " + stageDetailStyle.Render(truncate(step.detail, max(8, width-2))) + "\n")
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (m model) renderRoute(width int) string {
	switch m.route {
	case routeHistory:
		return m.renderHistory(width)
	case routeSettings:
		return m.renderSettings(width)
	case routeActivity:
		return m.renderActivity(width)
	default:
		return m.renderHome(width)
	}
}

func (m model) renderHome(width int) string {
	var b strings.Builder
	b.WriteString(pageEyebrow.Render("ANIME, WITHOUT LEAVING THE TERMINAL") + "\n")
	b.WriteString(pageTitle.Render("What do you want to watch?") + "\n")
	b.WriteString(pageSubtitle.Render("ani-tui keeps ani-cli as the playback engine and replaces its prompts with one focused workspace.") + "\n\n")
	modeLabel := "SEARCH"
	if m.searchMode == opCountdown {
		modeLabel = "NEXT EPISODE COUNTDOWN"
	} else if m.pendingEpisode != "" {
		modeLabel = "EXACT EPISODE / RANGE: " + m.pendingEpisode
	} else if m.pendingIndex != "" {
		modeLabel = "SEARCH RESULT INDEX: " + m.pendingIndex
	}
	b.WriteString(sectionTitle.Render(modeLabel) + "\n")
	searchBox := searchBoxStyle.Width(min(width-4, 68)).Render(m.search.View())
	b.WriteString(searchBox + "\n")
	b.WriteString(hintStyle.Render("Enter to browse  •  c to continue  •  ctrl+p for every action") + "\n\n")
	cards := []struct{ key, title, sub string }{
		{"c", "Continue", "Resume from ani-cli history"},
		{"s", "Settings", "Player, quality, dub and downloads"},
		{"a", "Activity", "Backend output and errors"},
	}
	if width >= 72 {
		cardWidth := (min(width, 78) - 4) / 3
		var rendered []string
		for _, card := range cards {
			rendered = append(rendered, homeCard(cardWidth, card.key, card.title, card.sub))
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, rendered...))
	} else {
		for _, card := range cards {
			b.WriteString(homeCard(width-2, card.key, card.title, card.sub) + "\n")
		}
	}
	return b.String()
}

func homeCard(width int, key, title, sub string) string {
	content := keycapStyle.Render(key) + "  " + cardTitle.Render(title) + "\n" + hintStyle.Render(sub)
	return cardStyle.Width(max(16, width-3)).Height(3).Render(content)
}

func (m model) renderHistory(width int) string {
	var b strings.Builder
	b.WriteString(pageEyebrow.Render("LOCAL ANI-CLI STATE") + "\n")
	b.WriteString(pageTitle.Render("Watch history") + "\n")
	b.WriteString(pageSubtitle.Render("Enter searches the selected title. Press c for ani-cli's exact continue flow.") + "\n\n")
	if len(m.history) == 0 {
		b.WriteString(emptyStyle.Width(min(width-2, 68)).Render("No watch history yet.\n\nPlay an episode and ani-cli will record it here."))
		return b.String()
	}
	visible := max(3, m.height-13)
	start := clamp(m.historyCursor-visible/2, 0, max(0, len(m.history)-visible))
	end := min(len(m.history), start+visible)
	for i := start; i < end; i++ {
		entry := m.history[i]
		title := truncate(entry.Title, max(12, width-20))
		line := fmt.Sprintf("%-*s  ep %-6s", max(12, width-14), title, entry.Episode)
		if i == m.historyCursor {
			b.WriteString(listSelected.Width(min(width-2, 78)).Render("› " + line))
		} else {
			b.WriteString(listItem.Width(min(width-2, 78)).Render("  " + line))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + hintStyle.Render(fmt.Sprintf("%d titles  •  ↑↓ navigate  •  enter search  •  r refresh", len(m.history))))
	return b.String()
}

func (m model) renderSettings(width int) string {
	var b strings.Builder
	b.WriteString(pageEyebrow.Render("PERSISTED IN "+configPath()) + "\n")
	b.WriteString(pageTitle.Render("Playback settings") + "\n")
	b.WriteString(pageSubtitle.Render("These map directly to ani-cli flags and ANI_CLI_* environment variables.") + "\n\n")
	fields := settingsFields(m.cfg)
	visible := max(4, m.height-13)
	start := clamp(m.settingsCursor-visible/2, 0, max(0, len(fields)-visible))
	end := min(len(fields), start+visible)
	for i := start; i < end; i++ {
		field := fields[i]
		labelWidth := min(22, max(14, width/3))
		valueWidth := max(10, width-labelWidth-7)
		line := fmt.Sprintf("%-*s  %-*s", labelWidth, field.label, valueWidth, truncate(field.value, valueWidth))
		if i == m.settingsCursor {
			b.WriteString(listSelected.Width(min(width-2, 88)).Render("› " + line))
			b.WriteString("\n  " + hintStyle.Render(field.desc))
		} else {
			b.WriteString(listItem.Width(min(width-2, 88)).Render("  " + line))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + hintStyle.Render("↑↓ select  •  ←→ change  •  enter/e edit paths and flags"))
	return b.String()
}

func (m model) renderActivity(width int) string {
	var b strings.Builder
	b.WriteString(pageEyebrow.Render("ANI-CLI BACKEND") + "\n")
	title := "Activity"
	if m.running {
		title = m.spinner.View() + " Working"
	}
	b.WriteString(pageTitle.Render(title) + "\n")
	if m.command != "" {
		b.WriteString(commandStyle.Render(truncate(m.command, width-4)) + "\n")
	} else {
		b.WriteString(pageSubtitle.Render("Backend output, diagnostics, downloads and errors appear here.") + "\n")
	}
	b.WriteString("\n")
	visible := max(3, m.height-13)
	end := len(m.logs) - m.logOffset
	end = clamp(end, 0, len(m.logs))
	start := max(0, end-visible)
	for _, line := range m.logs[start:end] {
		style := logStyle
		prefix := "│ "
		if line.isErr {
			style = logErrorStyle
			prefix = "! "
		}
		b.WriteString(style.Render(prefix+truncate(line.text, max(12, width-3))) + "\n")
	}
	if len(m.logs) == 0 {
		b.WriteString(emptyStyle.Render("No activity yet."))
	}
	b.WriteString("\n" + hintStyle.Render("↑↓ scroll  •  G latest  •  x clear  •  ctrl+c cancel running task"))
	return b.String()
}

func (m model) renderDoctor(width, _ int) string {
	contentWidth := max(20, width-5)
	innerWidth := max(16, contentWidth-4)
	var content strings.Builder
	if m.doctor == nil {
		return ""
	}
	if m.doctor.phase == doctorScanning {
		content.WriteString(pageSubtitle.Render("Checking core tools and only the optional features active for this launch.") + "\n\n")
		content.WriteString(m.spinner.View() + " Inspecting executables and trusted package managers…")
		return renderInteractionPane("DEPENDENCY DOCTOR", "Checking this configuration", content.String(), contentWidth)
	}

	issue := m.doctor.current()
	if issue == nil {
		return renderInteractionPane("DEPENDENCY DOCTOR", "Finishing dependency check", m.spinner.View()+" Verifying the current configuration…", contentWidth)
	}
	queue := fmt.Sprintf("Missing requirement %d of %d", m.doctor.index+1, len(m.doctor.issues))
	content.WriteString(pageSubtitle.Render(queue) + "\n\n")
	content.WriteString(bodyStyle.Render(wrapDisplay(issue.Requirement.Reason, innerWidth)) + "\n\n")

	switch m.doctor.phase {
	case doctorOffering:
		if issue.Plan != nil {
			content.WriteString(sectionTitle.Render("EXACT COMMAND") + "\n")
			content.WriteString(commandStyle.Render(wrapDisplay(issue.Plan.DisplayCommand(), innerWidth-2)) + "\n\n")
			content.WriteString(sectionTitle.Render("VERIFIED OFFICIAL SOURCE") + "\n")
			content.WriteString(hintStyle.Render(wrapDisplay(issue.Plan.SourceURL, innerWidth)) + "\n\n")
			content.WriteString(keycapStyle.Render("y") + " install and verify   " + keycapStyle.Render("n") + " not now")
		} else {
			content.WriteString(lipgloss.NewStyle().Foreground(colorWarning).Bold(true).Render("Automatic installation is unavailable for this platform/source.") + "\n\n")
			content.WriteString(sectionTitle.Render("OFFICIAL MANUAL GUIDANCE") + "\n")
			content.WriteString(hintStyle.Render(wrapDisplay(issue.Requirement.ManualURL, innerWidth)) + "\n\n")
			content.WriteString(keycapStyle.Render("enter") + " acknowledge")
		}
	case doctorInstalling:
		content.WriteString(m.spinner.View() + fmt.Sprintf(" Installing with terminal handoff — attempt %d/%d", m.doctor.attempt, doctorMaxAttempts))
	case doctorBackoffWait:
		delay := doctorBackoff(m.doctor.attempt)
		content.WriteString(lipgloss.NewStyle().Foreground(colorWarning).Render(fmt.Sprintf("Transient failure. Retrying automatically after %s (attempt %d/%d next).", delay, m.doctor.attempt+1, doctorMaxAttempts)) + "\n\n")
		content.WriteString(hintStyle.Render(wrapDisplay(m.doctor.failure.Reason, innerWidth)) + "\n\n")
		content.WriteString(keycapStyle.Render("esc") + " cancel retry")
	case doctorFailed:
		label := "Installation stopped"
		if m.doctor.failure.Kind == installFailureTransient && m.doctor.attempt >= doctorMaxAttempts {
			label = "Five attempts exhausted"
		}
		content.WriteString(lipgloss.NewStyle().Foreground(colorDanger).Bold(true).Render(label) + "\n")
		content.WriteString(bodyStyle.Render(wrapDisplay(m.doctor.failure.Reason, innerWidth)) + "\n\n")
		content.WriteString(hintStyle.Render(wrapDisplay("Last output: "+m.doctor.lastOutput, innerWidth)) + "\n\n")
		content.WriteString(keycapStyle.Render("enter") + " close result")
	}
	return renderInteractionPane("DEPENDENCY DOCTOR", issue.Requirement.Name+" not found", content.String(), contentWidth)
}

func wrapDisplay(value string, width int) string {
	width = max(8, width)
	var lines []string
	for _, sourceLine := range strings.Split(value, "\n") {
		words := strings.Fields(sourceLine)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := ""
		for _, word := range words {
			for utf8.RuneCountInString(word) > width {
				if line != "" {
					lines = append(lines, line)
					line = ""
				}
				runes := []rune(word)
				lines = append(lines, string(runes[:width]))
				word = string(runes[width:])
			}
			if line == "" {
				line = word
			} else if utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= width {
				line += " " + word
			} else {
				lines = append(lines, line)
				line = word
			}
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func (m model) renderSelectionModal(width, height int) string {
	s := m.menu
	contentWidth := max(20, width-5)
	var b strings.Builder
	if s.inputMode {
		b.WriteString(pageSubtitle.Render("ani-cli needs one more value to continue this session.") + "\n\n")
		b.WriteString(searchBoxStyle.Width(max(16, contentWidth-4)).Render(s.filter.View()) + "\n\n")
		b.WriteString(hintStyle.Render("enter submit  •  esc cancel"))
		return renderInteractionPane("ANI-CLI REQUEST", emptyDefault(s.prompt, "Enter a value"), b.String(), contentWidth)
	}
	if s.filterActive || s.filter.Value() != "" {
		b.WriteString(searchBoxStyle.Width(max(16, contentWidth-4)).Render(s.filter.View()) + "\n")
	} else {
		label := fmt.Sprintf("%d choices from ani-cli", len(s.original))
		if s.multi {
			label += "  •  mark episode bounds with space"
		}
		if s.playback {
			label = "Episode controls are sent directly to ani-cli"
		}
		b.WriteString(hintStyle.Render(label) + "\n")
	}
	visible := clamp(height-10, 3, 16)
	s.offset = clamp(s.cursor-visible/2, 0, max(0, len(s.filtered)-visible))
	end := min(len(s.filtered), s.offset+visible)
	for i := s.offset; i < end; i++ {
		item := s.filtered[i]
		mark := "  "
		if s.multi && s.selected[s.originalIndex(item)] {
			mark = "● "
		}
		line := mark + truncate(s.displayLabel(item), max(12, contentWidth-8))
		if s.playback {
			shortcut, _ := playbackActionPresentation(item)
			if shortcut != "" {
				line = mark + keycapStyle.Render(shortcut) + "  " + truncate(s.displayLabel(item), max(12, contentWidth-13))
			}
		}
		if i == s.cursor {
			b.WriteString(listSelected.Width(max(16, contentWidth-4)).Render("› " + line))
		} else {
			b.WriteString(listItem.Width(max(16, contentWidth-4)).Render("  " + line))
		}
		b.WriteString("\n")
	}
	if len(s.filtered) == 0 {
		b.WriteString(emptyStyle.Width(max(16, contentWidth-4)).Render("No matching choices"))
	}
	title := "ANI-CLI REQUEST"
	hint := "↑↓ navigate  •  / filter  •  enter select  •  esc cancel"
	if s.playback {
		title = "PLAYBACK CONTROLS"
		hint = "n next  •  r replay  •  p previous  •  c quality  •  q quit"
	}
	b.WriteString("\n" + hintStyle.Render(hint))
	return renderInteractionPane(title, emptyDefault(s.prompt, "Select an option"), b.String(), contentWidth)
}

func (m model) renderPalette(width, height int) string {
	p := m.palette
	contentWidth := max(20, width-5)
	var b strings.Builder
	b.WriteString(pageSubtitle.Render("Jump anywhere without leaving the workspace.") + "\n")
	b.WriteString(searchBoxStyle.Width(max(16, contentWidth-4)).Render(p.filter.View()) + "\n")
	visible := clamp((height-11)/2, 2, 8)
	p.offset = clamp(p.cursor-visible/2, 0, max(0, len(p.filtered)-visible))
	end := min(len(p.filtered), p.offset+visible)
	for i := p.offset; i < end; i++ {
		item := p.filtered[i]
		line := cardTitle.Render(item.title) + "\n" + hintStyle.Render("   "+truncate(item.desc, max(12, contentWidth-8)))
		if i == p.cursor {
			b.WriteString(listSelected.Width(max(16, contentWidth-4)).Render("› " + line))
		} else {
			b.WriteString(listItem.Width(max(16, contentWidth-4)).Render("  " + line))
		}
		b.WriteString("\n")
	}
	if len(p.filtered) == 0 {
		b.WriteString(emptyStyle.Render("No commands match"))
	}
	b.WriteString("\n" + hintStyle.Render("↑↓ navigate  •  enter run  •  esc close"))
	return renderInteractionPane("COMMAND PALETTE", "Where do you want to go?", b.String(), contentWidth)
}

func (m model) renderConfirm(width, _ int) string {
	contentWidth := max(20, width-5)
	content := pageSubtitle.Render("Review this action before ani-cli continues.") + "\n\n" +
		bodyStyle.Width(max(16, contentWidth-4)).Render(m.confirm.body) + "\n\n" +
		keycapStyle.Render("y") + " confirm   " + keycapStyle.Render("n") + " cancel"
	return renderInteractionPane("CONFIRM ACTION", m.confirm.title, content, contentWidth)
}

func (m model) renderEdit(width, _ int) string {
	contentWidth := max(20, width-5)
	content := pageSubtitle.Render("Edit this value in place. The rest of your workspace stays visible.") + "\n\n" +
		searchBoxStyle.Width(max(16, contentWidth-4)).Render(m.edit.input.View()) + "\n\n" +
		hintStyle.Render("enter save  •  esc discard")
	return renderInteractionPane("EDIT VALUE", m.edit.label, content, contentWidth)
}

func (m model) renderHelp(width, _ int) string {
	contentWidth := max(20, width-5)
	col1 := []string{
		keyHelp("/", "focus anime search"),
		keyHelp("c", "continue watching"),
		keyHelp("1–4 / tab", "switch screen"),
		keyHelp("ctrl+p", "command palette"),
		keyHelp("?", "toggle this help"),
	}
	col2 := []string{
		keyHelp("↑↓ / j k", "move selection"),
		keyHelp("space", "mark episode range"),
		keyHelp("/", "filter an ani-cli request"),
		keyHelp("ctrl+c", "cancel active session"),
		keyHelp("q", "quit when input is blurred"),
	}
	left := strings.Join(col1, "\n")
	right := strings.Join(col2, "\n")
	columns := left + "\n" + right
	if contentWidth >= 58 {
		columns = lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width((contentWidth-4)/2).Render(left),
			lipgloss.NewStyle().Width((contentWidth-4)/2).Render(right),
		)
	}
	content := pageSubtitle.Render("Every interaction remains inside this full-screen workspace.") + "\n\n" +
		columns + "\n\n" + hintStyle.Render("Press ?, esc, enter, or q to return")
	return renderInteractionPane("KEYBOARD-FIRST", "Help & shortcuts", content, contentWidth)
}

func renderInteractionPane(eyebrow, title, content string, width int) string {
	heading := paneEyebrow.Render(eyebrow) + "\n" + paneTitle.Render(title) + "\n" + content
	return interactionPaneStyle.Width(max(16, width)).Render(heading)
}

func keyHelp(key, label string) string {
	return fmt.Sprintf("%-13s %s", keycapStyle.Render(key), bodyStyle.Render(label))
}

func smallTerminalView(width, height int) string {
	content := pageTitle.Render("ANI//TUI") + "\n\n" +
		bodyStyle.Render("Terminal is too small.") + "\n" +
		hintStyle.Render("Resize to at least 54 × 16.") + "\n\n" +
		fmt.Sprintf("Current: %d × %d", width, height)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

func writeBridgeResponse(conn net.Conn, response bridgeResponse) error {
	return encodeBridgeResponse(conn, response)
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func emptyLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(none)"
	}
	return value
}

func emptyDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func isOneOf(value string, items ...string) bool {
	for _, item := range items {
		if value == item {
			return true
		}
	}
	return false
}

func workspaceContentWidth(total int) int {
	if total >= 88 {
		contextWidth := clamp(total*31/100, 29, 38)
		return max(20, total-contextWidth-7)
	}
	return max(20, total-4)
}

func workspaceInputWidth(total int) int {
	return clamp(workspaceContentWidth(total)-9, 20, 72)
}

func clamp(value, low, high int) int {
	if high < low {
		return low
	}
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncate(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	if limit == 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}
