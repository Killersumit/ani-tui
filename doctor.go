package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	doctorMaxAttempts   = 5
	doctorCaptureLimit  = 128 * 1024
	aniCLIInstallURL    = "https://github.com/pystardust/ani-cli#install"
	aniSkipInstallURL   = "https://github.com/synacktraa/ani-skip"
	syncplayInstallURL  = "https://syncplay.pl/guide/install/"
	cattInstallURL      = "https://github.com/skorokithakis/catt"
	curlInstallURL      = "https://curl.se/download.html"
	coreutilsInstallURL = "https://github.com/pystardust/ani-cli#dependencies"
)

type dependencyID string

const (
	depAniCLI  dependencyID = "ani-cli"
	depCurl    dependencyID = "curl"
	depSed     dependencyID = "sed"
	depGrep    dependencyID = "grep"
	depPatch   dependencyID = "patch"
	depMPV     dependencyID = "mpv"
	depVLC     dependencyID = "vlc"
	depIINA    dependencyID = "iina"
	depSync    dependencyID = "syncplay"
	depYTDLP   dependencyID = "yt-dlp"
	depFFmpeg  dependencyID = "ffmpeg"
	depAniSkip dependencyID = "ani-skip"
	depCatt    dependencyID = "catt"
	depFlatpak dependencyID = "flatpak"
	depLog     dependencyID = "log-viewer"
)

type dependencyRequirement struct {
	ID             dependencyID
	Name           string
	Reason         string
	Alternatives   []string
	InstallTargets []dependencyID
	ManualURL      string
}

type dependencyIssue struct {
	Requirement dependencyRequirement
	Plan        *installPlan
}

type packageManager struct {
	ID        string
	Binary    string
	SourceURL string
}

type distroInfo struct {
	ID        string
	IDLike    []string
	VersionID string
	Codename  string
}

type installStep struct {
	Binary             string
	Args               []string
	AllowAlreadyExists bool
}

type installPlan struct {
	Manager   packageManager
	Package   string
	Prep      []installStep
	Binary    string
	Args      []string
	SourceURL string
}

func (p installPlan) DisplayCommand() string {
	commands := make([]string, 0, len(p.Prep)+1)
	for _, step := range p.Prep {
		commands = append(commands, shellDisplay(step.Binary, step.Args))
	}
	commands = append(commands, shellDisplay(p.Binary, p.Args))
	return strings.Join(commands, " && ")
}

type doctorEnvironment struct {
	GOOS       string
	LookPath   func(string) (string, error)
	PathExists func(string) bool
	ReadFile   func(string) ([]byte, error)
	IsRoot     bool
}

func systemDoctorEnvironment() doctorEnvironment {
	isRoot := false
	if current, err := user.Current(); err == nil {
		isRoot = current.Uid == "0"
	}
	return doctorEnvironment{
		GOOS:     runtime.GOOS,
		LookPath: exec.LookPath,
		PathExists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
		ReadFile: os.ReadFile,
		IsRoot:   isRoot,
	}
}

func normalizeDoctorEnvironment(env doctorEnvironment) doctorEnvironment {
	system := systemDoctorEnvironment()
	if env.GOOS == "" {
		env.GOOS = system.GOOS
	}
	if env.LookPath == nil {
		env.LookPath = system.LookPath
	}
	if env.PathExists == nil {
		env.PathExists = system.PathExists
	}
	if env.ReadFile == nil {
		env.ReadFile = system.ReadFile
	}
	return env
}

func requirementSatisfied(req dependencyRequirement, env doctorEnvironment) bool {
	env = normalizeDoctorEnvironment(env)
	for _, candidate := range req.Alternatives {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if strings.ContainsAny(candidate, `/\\`) || filepath.IsAbs(candidate) {
			if _, err := env.LookPath(candidate); err == nil {
				return true
			}
			// ani-cli uses these directories as sentinels for an installed mpv
			// Flatpak and invokes `flatpak run` later. Every other path must be
			// executable; mere file existence is not sufficient.
			if req.ID == depMPV && strings.Contains(filepath.ToSlash(candidate), "/flatpak/app/io.mpv.Mpv") && env.PathExists(candidate) {
				return true
			}
			continue
		}
		if _, err := env.LookPath(candidate); err == nil {
			return true
		}
	}
	return false
}

func requiredDependencies(cfg Config, spec commandSpec, env doctorEnvironment) []dependencyRequirement {
	env = normalizeDoctorEnvironment(env)
	regular := spec.Operation == opSearch || spec.Operation == opContinue || spec.Operation == opCountdown || spec.Operation == ""
	requirements := []dependencyRequirement{
		{
			ID:             depAniCLI,
			Name:           "ani-cli",
			Reason:         "ani-tui delegates browsing and playback to ani-cli",
			Alternatives:   []string{cfg.AniCLIPath},
			InstallTargets: []dependencyID{depAniCLI},
			ManualURL:      aniCLIInstallURL,
		},
		{
			ID:             depCurl,
			Name:           "curl-compatible client",
			Reason:         "ani-cli needs curl for catalog and stream requests",
			Alternatives:   []string{"curl_firefox135", "curl_chrome136", "curl_chrome116", "curl_ff117", "curl"},
			InstallTargets: []dependencyID{depCurl},
			ManualURL:      curlInstallURL,
		},
	}

	if regular || spec.Operation == opUpdate {
		requirements = append(requirements,
			dependencyRequirement{ID: depSed, Name: "sed", Reason: "ani-cli parses catalog responses with sed", Alternatives: []string{"sed"}, InstallTargets: []dependencyID{depSed}, ManualURL: coreutilsInstallURL},
			dependencyRequirement{ID: depGrep, Name: "grep", Reason: "ani-cli filters catalog responses with grep", Alternatives: []string{"grep"}, InstallTargets: []dependencyID{depGrep}, ManualURL: coreutilsInstallURL},
		)
	}

	if spec.Operation == opUpdate {
		requirements = append(requirements, dependencyRequirement{
			ID: depPatch, Name: "patch", Reason: "ani-cli's self-update applies a unified diff with patch",
			Alternatives: []string{"patch"}, InstallTargets: []dependencyID{depPatch}, ManualURL: coreutilsInstallURL,
		})
	}
	if spec.Operation == opLogs {
		viewer := "journalctl"
		if env.GOOS == "darwin" {
			viewer = "log"
		}
		requirements = append(requirements, dependencyRequirement{
			ID: depLog, Name: viewer, Reason: "ani-cli uses the platform log viewer for this operation",
			Alternatives: []string{viewer}, ManualURL: coreutilsInstallURL,
		})
	}

	requirements = append(requirements, playerRequirements(cfg, spec, env)...)

	if regular {
		if cfg.Action == "download" {
			requirements = append(requirements, dependencyRequirement{
				ID: depYTDLP, Name: "yt-dlp or ffmpeg", Reason: "download mode needs one supported downloader",
				Alternatives: []string{"yt-dlp", "ffmpeg"}, InstallTargets: []dependencyID{depYTDLP, depFFmpeg},
				ManualURL: "https://github.com/yt-dlp/yt-dlp#installation",
			})
		}
		if cfg.SkipIntro {
			reason := "intro skipping is enabled and ani-cli invokes ani-skip"
			if env.GOOS == "windows" {
				reason += " (upstream currently marks this unsupported on Windows)"
			}
			requirements = append(requirements, dependencyRequirement{
				ID: depAniSkip, Name: "ani-skip", Reason: reason,
				Alternatives: []string{"ani-skip"}, InstallTargets: []dependencyID{depAniSkip}, ManualURL: aniSkipInstallURL,
			})
		}
	}

	// ani-cli's Linux auto-player can select a Flatpak installation directory,
	// but playback then still requires the flatpak executable.
	if env.GOOS == "linux" && cfg.Player == "auto" && cfg.Action != "download" && (spec.Operation == opSearch || spec.Operation == opContinue || spec.Operation == "") {
		flatpakMPV := firstExisting(env, "$HOME/.local/share/flatpak/app/io.mpv.Mpv/", "/var/lib/flatpak/app/io.mpv.Mpv/")
		if flatpakMPV != "" && !commandAvailable("mpv", env) {
			requirements = append(requirements, dependencyRequirement{
				ID: depFlatpak, Name: "flatpak", Reason: "ani-cli selected an installed Flatpak mpv",
				Alternatives: []string{"flatpak"}, InstallTargets: []dependencyID{depFlatpak}, ManualURL: "https://flatpak.org/setup/",
			})
		}
	}
	return requirements
}

func playerRequirements(cfg Config, spec commandSpec, env doctorEnvironment) []dependencyRequirement {
	regularFinalCheck := (spec.Operation == opSearch || spec.Operation == opContinue || spec.Operation == opCountdown || spec.Operation == "") && cfg.Action != "download"
	var requirements []dependencyRequirement

	// With no ANI_CLI_PLAYER override, Linux and macOS probe for a default
	// player before option parsing. VLC and Syncplay are expressed as later
	// flags, so they still trigger this initial probe. Windows merely assigns
	// mpv.exe at this stage and does not validate it until the final check.
	usesPlatformAutoProbe := cfg.Player == "auto" || cfg.Player == "vlc" || cfg.Player == "syncplay"
	if usesPlatformAutoProbe && env.GOOS != "windows" {
		requirements = append(requirements, autoPlayerRequirement(env.GOOS))
	}

	if !regularFinalCheck {
		return requirements
	}
	switch cfg.Player {
	case "debug":
		return requirements
	case "mpv":
		requirements = append(requirements, dependencyRequirement{ID: depMPV, Name: "mpv", Reason: "the selected player is mpv", Alternatives: executableNames(env.GOOS, "mpv"), InstallTargets: []dependencyID{depMPV}, ManualURL: "https://mpv.io/installation/"})
	case "catt":
		requirements = append(requirements, dependencyRequirement{ID: depCatt, Name: "catt", Reason: "the selected cast player is catt", Alternatives: []string{"catt"}, InstallTargets: []dependencyID{depCatt}, ManualURL: cattInstallURL})
	case "custom":
		requirements = append(requirements, dependencyRequirement{ID: dependencyID("custom-player"), Name: emptyDefault(strings.TrimSpace(cfg.CustomPlayer), "custom player"), Reason: "the selected custom player must be executable", Alternatives: []string{cfg.CustomPlayer}, ManualURL: "https://github.com/pystardust/ani-cli#dependencies"})
	case "vlc":
		requirements = append(requirements, dependencyRequirement{ID: depVLC, Name: "VLC", Reason: "the selected player is VLC", Alternatives: executableNames(env.GOOS, "vlc"), InstallTargets: []dependencyID{depVLC}, ManualURL: "https://www.videolan.org/vlc/"})
	case "syncplay":
		syncCandidates := []string{"syncplay"}
		if env.GOOS == "darwin" {
			syncCandidates = []string{"/Applications/Syncplay.app/Contents/MacOS/syncplay", "syncplay"}
		} else if env.GOOS == "windows" {
			syncCandidates = []string{"syncplay.exe"}
		}
		requirements = append(requirements, dependencyRequirement{ID: depSync, Name: "Syncplay", Reason: "the selected player is Syncplay", Alternatives: syncCandidates, InstallTargets: []dependencyID{depSync}, ManualURL: syncplayInstallURL})
	case "auto":
		if env.GOOS == "windows" {
			requirements = append(requirements, autoPlayerRequirement(env.GOOS))
		}
	}
	return requirements
}

func autoPlayerRequirement(goos string) dependencyRequirement {
	switch goos {
	case "darwin":
		return dependencyRequirement{
			ID: depIINA, Name: "IINA, mpv, or VLC", Reason: "ani-cli selects an available macOS player before parsing options",
			Alternatives:   []string{"iina", "/Applications/IINA.app/Contents/MacOS/iina-cli", "mpv", "vlc"},
			InstallTargets: []dependencyID{depIINA, depMPV, depVLC}, ManualURL: "https://github.com/pystardust/ani-cli#macos",
		}
	case "windows":
		return dependencyRequirement{ID: depMPV, Name: "mpv", Reason: "ani-cli's Windows player is mpv.exe", Alternatives: []string{"mpv.exe"}, InstallTargets: []dependencyID{depMPV}, ManualURL: "https://github.com/pystardust/ani-cli#windows"}
	default:
		return dependencyRequirement{
			ID: depMPV, Name: "mpv or VLC", Reason: "ani-cli selects an available Linux player before parsing options",
			Alternatives:   []string{"mpv", expandHome("$HOME/.local/share/flatpak/app/io.mpv.Mpv/"), "/var/lib/flatpak/app/io.mpv.Mpv/", "vlc"},
			InstallTargets: []dependencyID{depMPV, depVLC}, ManualURL: "https://github.com/pystardust/ani-cli#linux",
		}
	}
}

func executableNames(goos, name string) []string {
	if goos == "windows" {
		return []string{name + ".exe", name}
	}
	return []string{name}
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "$HOME/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "$HOME/"))
		}
	}
	return path
}

func firstExisting(env doctorEnvironment, paths ...string) string {
	for _, path := range paths {
		path = expandHome(path)
		if env.PathExists(path) {
			return path
		}
	}
	return ""
}

func commandAvailable(name string, env doctorEnvironment) bool {
	_, err := env.LookPath(name)
	return err == nil
}

func scanDependencies(cfg Config, spec commandSpec, env doctorEnvironment) []dependencyIssue {
	env = normalizeDoctorEnvironment(env)
	distro := readDistroInfo(env)
	manager, _ := detectPackageManager(env, distro)
	var issues []dependencyIssue
	for _, requirement := range requiredDependencies(cfg, spec, env) {
		if requirementSatisfied(requirement, env) {
			continue
		}
		issue := dependencyIssue{Requirement: requirement}
		if manager.ID != "" {
			for _, target := range requirement.InstallTargets {
				if plan, ok := makeInstallPlan(manager, target, cfg, distro, env); ok {
					issue.Plan = &plan
					break
				}
			}
		}
		issues = append(issues, issue)
	}
	return issues
}

func parseOSRelease(data []byte) distroInfo {
	values := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		values[strings.TrimSpace(key)] = value
	}
	return distroInfo{
		ID: strings.ToLower(values["ID"]), IDLike: strings.Fields(strings.ToLower(values["ID_LIKE"])),
		VersionID: values["VERSION_ID"], Codename: strings.ToLower(emptyDefault(values["VERSION_CODENAME"], values["DEBIAN_CODENAME"])),
	}
}

func readDistroInfo(env doctorEnvironment) distroInfo {
	if env.GOOS != "linux" {
		return distroInfo{}
	}
	data, err := env.ReadFile("/etc/os-release")
	if err != nil {
		return distroInfo{}
	}
	return parseOSRelease(data)
}

func detectPackageManager(env doctorEnvironment, distro distroInfo) (packageManager, bool) {
	env = normalizeDoctorEnvironment(env)
	var candidates []string
	switch env.GOOS {
	case "darwin":
		candidates = []string{"brew"}
	case "windows":
		candidates = []string{"scoop", "winget"}
	default:
		family := append([]string{distro.ID}, distro.IDLike...)
		switch {
		case containsAny(family, "debian", "ubuntu", "linuxmint", "pop"):
			candidates = []string{"apt-get", "dnf", "pacman", "zypper", "apk"}
		case containsAny(family, "fedora", "rhel", "centos"):
			candidates = []string{"dnf", "apt-get", "pacman", "zypper", "apk"}
		case containsAny(family, "arch", "manjaro"):
			candidates = []string{"pacman", "apt-get", "dnf", "zypper", "apk"}
		case containsAny(family, "suse", "opensuse"):
			candidates = []string{"zypper", "apt-get", "dnf", "pacman", "apk"}
		case containsAny(family, "alpine"):
			candidates = []string{"apk", "apt-get", "dnf", "pacman", "zypper"}
		default:
			candidates = []string{"apt-get", "dnf", "pacman", "zypper", "apk", "brew"}
		}
	}
	for _, name := range candidates {
		if path, err := env.LookPath(name); err == nil {
			return packageManager{ID: managerID(name), Binary: path, SourceURL: managerSourceURL(managerID(name))}, true
		}
	}
	return packageManager{}, false
}

func managerID(binary string) string {
	if binary == "apt" || binary == "apt-get" {
		return "apt"
	}
	return binary
}

func managerSourceURL(id string) string {
	switch id {
	case "apt":
		return "https://www.debian.org/doc/manuals/apt-guide/ch2.en.html"
	case "dnf":
		return "https://dnf.readthedocs.io/en/latest/command_ref.html"
	case "pacman":
		return "https://wiki.archlinux.org/title/Pacman"
	case "zypper":
		return "https://documentation.suse.com/smart/systems-management/html/reference-zypper-install/index.html"
	case "apk":
		return "https://wiki.alpinelinux.org/wiki/Alpine_Package_Keeper"
	case "brew":
		return "https://docs.brew.sh/Manpage"
	case "scoop":
		return "https://scoop.sh/"
	case "winget":
		return "https://learn.microsoft.com/windows/package-manager/winget/install"
	}
	return ""
}

func containsAny(values []string, candidates ...string) bool {
	for _, value := range values {
		for _, candidate := range candidates {
			if value == candidate {
				return true
			}
		}
	}
	return false
}

type packageMapping struct {
	Name string
	Cask bool
	URL  string
}

func packageFor(manager string, dependency dependencyID, cfg Config, distro distroInfo) (packageMapping, bool) {
	common := map[dependencyID]string{
		depCurl: "curl", depSed: "sed", depGrep: "grep", depPatch: "patch", depMPV: "mpv", depVLC: "vlc",
		depSync: "syncplay", depYTDLP: "yt-dlp", depFFmpeg: "ffmpeg", depFlatpak: "flatpak",
	}
	name := common[dependency]
	switch manager {
	case "apt":
		if dependency == depAniCLI {
			if !debianHasAniCLIPackage(distro) || !isDefaultAniCLIPath(cfg.AniCLIPath) {
				return packageMapping{}, false
			}
			name = "ani-cli"
		}
		if name == "" {
			return packageMapping{}, false
		}
		return packageMapping{Name: name}, true
	case "dnf":
		if dependency == depFFmpeg {
			name = "ffmpeg-free"
		}
		if dependency == depAniCLI || dependency == depSync || name == "" {
			return packageMapping{}, false
		}
		return packageMapping{Name: name}, true
	case "pacman":
		if dependency == depAniCLI || name == "" {
			return packageMapping{}, false
		}
		return packageMapping{Name: name}, true
	case "zypper":
		// Upstream notes that distro mpv/VLC builds miss features, and yt-dlp
		// availability differs between Leap and Tumbleweed. Do not invent a safe
		// one-command route for those optional tools.
		if dependency == depAniCLI || dependency == depMPV || dependency == depVLC || dependency == depSync || dependency == depYTDLP || dependency == depFFmpeg || name == "" {
			return packageMapping{}, false
		}
		return packageMapping{Name: name}, true
	case "apk":
		if dependency == depAniCLI || name == "" {
			return packageMapping{}, false
		}
		return packageMapping{Name: name}, true
	case "brew":
		switch dependency {
		case depMPV:
			return packageMapping{Name: "mpv", URL: "https://formulae.brew.sh/formula/mpv"}, true
		case depVLC:
			return packageMapping{Name: "vlc", Cask: true, URL: "https://formulae.brew.sh/cask/vlc"}, true
		case depIINA:
			return packageMapping{Name: "iina", Cask: true, URL: "https://formulae.brew.sh/cask/iina"}, true
		case depSync:
			return packageMapping{Name: "syncplay", Cask: true, URL: "https://formulae.brew.sh/cask/syncplay"}, true
		case depYTDLP:
			return packageMapping{Name: "yt-dlp", URL: "https://formulae.brew.sh/formula/yt-dlp"}, true
		case depFFmpeg:
			return packageMapping{Name: "ffmpeg", URL: "https://formulae.brew.sh/formula/ffmpeg"}, true
		}
		return packageMapping{}, false
	case "scoop":
		extrasURL := func(name string) string {
			return "https://github.com/ScoopInstaller/Extras/blob/master/bucket/" + name + ".json"
		}
		mainURL := func(name string) string {
			return "https://github.com/ScoopInstaller/Main/blob/master/bucket/" + name + ".json"
		}
		switch dependency {
		case depAniCLI:
			if !isDefaultAniCLIPath(cfg.AniCLIPath) {
				return packageMapping{}, false
			}
			return packageMapping{Name: "extras/ani-cli", URL: extrasURL("ani-cli")}, true
		case depMPV:
			return packageMapping{Name: "extras/mpv", URL: extrasURL("mpv")}, true
		case depVLC:
			return packageMapping{Name: "extras/vlc", URL: extrasURL("vlc")}, true
		case depSync:
			return packageMapping{Name: "extras/syncplay", URL: extrasURL("syncplay")}, true
		case depYTDLP:
			return packageMapping{Name: "yt-dlp", URL: mainURL("yt-dlp")}, true
		case depFFmpeg:
			return packageMapping{Name: "ffmpeg", URL: mainURL("ffmpeg")}, true
		case depCurl:
			return packageMapping{Name: "curl", URL: mainURL("curl")}, true
		case depPatch:
			return packageMapping{Name: "patch", URL: mainURL("patch")}, true
		}
		return packageMapping{}, false
	case "winget":
		switch dependency {
		case depVLC:
			return packageMapping{Name: "VideoLAN.VLC", URL: "https://github.com/microsoft/winget-pkgs/tree/master/manifests/v/VideoLAN/VLC"}, true
		case depYTDLP:
			return packageMapping{Name: "yt-dlp.yt-dlp", URL: "https://github.com/microsoft/winget-pkgs/tree/master/manifests/y/yt-dlp/yt-dlp"}, true
		}
		return packageMapping{}, false
	}
	return packageMapping{}, false
}

func isDefaultAniCLIPath(path string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(path)))
	return base == "ani-cli" || base == "ani-cli.exe"
}

func debianHasAniCLIPackage(distro distroInfo) bool {
	if distro.ID != "debian" {
		return false
	}
	if containsAny([]string{distro.Codename}, "trixie", "forky", "sid") {
		return true
	}
	majorText := strings.SplitN(distro.VersionID, ".", 2)[0]
	major, err := strconv.Atoi(majorText)
	return err == nil && major >= 13
}

func makeInstallPlan(manager packageManager, dependency dependencyID, cfg Config, distro distroInfo, env doctorEnvironment) (installPlan, bool) {
	pkg, ok := packageFor(manager.ID, dependency, cfg, distro)
	if !ok {
		return installPlan{}, false
	}
	plan := installPlan{Manager: manager, Package: pkg.Name, Binary: manager.Binary, SourceURL: emptyDefault(pkg.URL, manager.SourceURL)}
	switch manager.ID {
	case "apt":
		plan.Args = []string{"install", "-y", pkg.Name}
	case "dnf":
		plan.Args = []string{"install", "-y", pkg.Name}
	case "pacman":
		plan.Args = []string{"-S", "--needed", pkg.Name}
	case "zypper":
		plan.Args = []string{"install", pkg.Name}
	case "apk":
		plan.Args = []string{"add", pkg.Name}
	case "brew":
		plan.Args = []string{"install"}
		if pkg.Cask {
			plan.Args = append(plan.Args, "--cask")
		}
		plan.Args = append(plan.Args, pkg.Name)
	case "scoop":
		packageName := pkg.Name
		if bucket, name, qualified := strings.Cut(pkg.Name, "/"); qualified {
			// Scoop requires non-main buckets to be added first. These are two
			// direct argv executions, never a shell command; an existing bucket is
			// accepted so a transient retry can safely repeat the plan.
			plan.Prep = []installStep{{Binary: plan.Binary, Args: []string{"bucket", "add", bucket}, AllowAlreadyExists: true}}
			packageName = name
		}
		plan.Args = []string{"install", packageName}
	case "winget":
		plan.Args = []string{"install", "--id", pkg.Name, "-e", "--source", "winget", "--accept-package-agreements", "--accept-source-agreements"}
	}
	if env.GOOS == "linux" && !env.IsRoot && manager.ID != "brew" {
		sudo, err := env.LookPath("sudo")
		if err != nil {
			// Do not offer a command that is expected to fail halfway through
			// for lack of privilege. The issue remains visible with its verified
			// manual guidance instead.
			return installPlan{}, false
		}
		plan.Args = append([]string{plan.Binary}, plan.Args...)
		plan.Binary = sudo
	}
	return plan, true
}

type installFailureKind string

const (
	installFailureNone      installFailureKind = "none"
	installFailureTransient installFailureKind = "transient"
	installFailurePermanent installFailureKind = "permanent"
	installFailureUnknown   installFailureKind = "unknown"
)

type installFailure struct {
	Kind   installFailureKind
	Reason string
}

var (
	httpStatusPattern    = regexp.MustCompile(`(?i)(?:http(?: status)?[ =:/-]*)([45][0-9]{2})\b`)
	transientHTTPPattern = regexp.MustCompile(`(?i)\b(429|5[0-9]{2}) (?:too many|internal|bad gateway|service unavailable|gateway timeout)`)
)

func classifyInstallFailure(output string, err error) installFailure {
	if err == nil {
		return installFailure{Kind: installFailureNone}
	}
	text := strings.ToLower(stripANSI(output + "\n" + err.Error()))
	permanent := []struct{ pattern, reason string }{
		{"unable to locate package", "package is unavailable from the configured repositories"},
		{"no package ", "package is unavailable from the configured repositories"},
		{"package not found", "package is unavailable from the configured source"},
		{"no matches found", "package is unavailable from the configured source"},
		{"manifest not found", "package manifest was not found"},
		{"404 not found", "the requested package or source does not exist"},
		{"status 404", "the requested package or source does not exist"},
		{"permission denied", "permission was denied"},
		{"not in the sudoers", "sudo authorization was denied"},
		{"authentication failed", "authentication failed"},
		{"incorrect password", "authentication failed"},
		{"access is denied", "permission was denied"},
		{"unknown option", "the package-manager command is unsupported"},
		{"unrecognized option", "the package-manager command is unsupported"},
		{"conflicting dependencies", "package dependencies conflict"},
		{"breaks dependency", "package dependencies conflict"},
		{"no space left on device", "the device is out of space"},
		{"insufficient disk space", "the device is out of space"},
		{"checksum mismatch", "package integrity verification failed"},
		{"signature verification failed", "package signature verification failed"},
		{"invalid signature", "package signature verification failed"},
	}
	for _, item := range permanent {
		if strings.Contains(text, item.pattern) {
			return installFailure{Kind: installFailurePermanent, Reason: item.reason}
		}
	}
	if match := httpStatusPattern.FindStringSubmatch(text); len(match) == 2 {
		code, _ := strconv.Atoi(match[1])
		if code == 408 || code == 429 || code >= 500 {
			return installFailure{Kind: installFailureTransient, Reason: "the package service returned a retryable HTTP response"}
		}
		return installFailure{Kind: installFailurePermanent, Reason: "the package service rejected the request with a permanent HTTP response"}
	}
	transient := []struct{ pattern, reason string }{
		{"timed out", "the network request timed out"}, {"timeout", "the network request timed out"},
		{"temporary failure", "a temporary network failure occurred"}, {"temporarily unavailable", "the service is temporarily unavailable"},
		{"connection reset", "the network connection was reset"}, {"connection refused", "the network connection was refused"},
		{"could not resolve", "DNS resolution failed"}, {"name resolution", "DNS resolution failed"},
		{"network is unreachable", "the network is unreachable"}, {"no route to host", "the network is unreachable"},
		{"tls handshake timeout", "the TLS handshake timed out"},
		{"too many requests", "the service rate-limited the request"},
	}
	for _, item := range transient {
		if strings.Contains(text, item.pattern) {
			return installFailure{Kind: installFailureTransient, Reason: item.reason}
		}
	}
	if transientHTTPPattern.MatchString(text) {
		return installFailure{Kind: installFailureTransient, Reason: "the package service returned a retryable HTTP response"}
	}
	return installFailure{Kind: installFailureUnknown, Reason: "the installer failed for an unrecognized reason"}
}

func doctorBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	return time.Duration(1<<min(attempt-1, 3)) * time.Second
}

type boundedCapture struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func newBoundedCapture(limit int) *boundedCapture {
	return &boundedCapture{limit: max(1, limit)}
}

func (b *boundedCapture) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(p) >= b.limit {
		b.data = append(b.data[:0], p[len(p)-b.limit:]...)
		return len(p), nil
	}
	overflow := len(b.data) + len(p) - b.limit
	if overflow > 0 {
		copy(b.data, b.data[overflow:])
		b.data = b.data[:len(b.data)-overflow]
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *boundedCapture) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(append([]byte(nil), b.data...))
}

type teeExecCommand struct {
	plan    installPlan
	capture *boundedCapture
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func newTeeExecCommand(plan installPlan) *teeExecCommand {
	return &teeExecCommand{plan: plan, capture: newBoundedCapture(doctorCaptureLimit)}
}

func (c *teeExecCommand) SetStdin(reader io.Reader)  { c.stdin = reader }
func (c *teeExecCommand) SetStdout(writer io.Writer) { c.stdout = writer }
func (c *teeExecCommand) SetStderr(writer io.Writer) { c.stderr = writer }

func (c *teeExecCommand) Run() error {
	steps := append([]installStep{}, c.plan.Prep...)
	steps = append(steps, installStep{Binary: c.plan.Binary, Args: c.plan.Args})
	for _, step := range steps {
		cmd := exec.Command(step.Binary, step.Args...)
		cmd.Stdin = c.stdin
		if c.stdout != nil {
			cmd.Stdout = io.MultiWriter(c.stdout, c.capture)
		} else {
			cmd.Stdout = c.capture
		}
		if c.stderr != nil {
			cmd.Stderr = io.MultiWriter(c.stderr, c.capture)
		} else {
			cmd.Stderr = c.capture
		}
		if err := cmd.Run(); err != nil {
			if step.AllowAlreadyExists && strings.Contains(strings.ToLower(c.capture.String()), "already exists") {
				continue
			}
			return err
		}
	}
	return nil
}

type doctorPhase int

const (
	doctorScanning doctorPhase = iota
	doctorOffering
	doctorInstalling
	doctorBackoffWait
	doctorFailed
)

type doctorState struct {
	active     bool
	blocking   bool
	token      int
	phase      doctorPhase
	issues     []dependencyIssue
	index      int
	attempt    int
	declined   bool
	pending    *commandSpec
	failure    installFailure
	lastOutput string
}

func (d *doctorState) current() *dependencyIssue {
	if d == nil || d.index < 0 || d.index >= len(d.issues) {
		return nil
	}
	return &d.issues[d.index]
}

type doctorScanMsg struct {
	Token    int
	Blocking bool
	Spec     commandSpec
	Issues   []dependencyIssue
}

type doctorInstallMsg struct {
	Token   int
	Index   int
	Attempt int
	Output  string
	Err     error
}

type doctorRetryMsg struct {
	Token int
	Index int
}

func doctorScanCmd(cfg Config, spec commandSpec, env doctorEnvironment, token int, blocking bool) tea.Cmd {
	return func() tea.Msg {
		return doctorScanMsg{Token: token, Blocking: blocking, Spec: spec, Issues: scanDependencies(cfg, spec, env)}
	}
}

func doctorInstallCmd(plan installPlan, token, index, attempt int) tea.Cmd {
	command := newTeeExecCommand(plan)
	return tea.Exec(command, func(err error) tea.Msg {
		return doctorInstallMsg{Token: token, Index: index, Attempt: attempt, Output: command.capture.String(), Err: err}
	})
}

func doctorRetryCmd(token, index, attempt int) tea.Cmd {
	return tea.Tick(doctorBackoff(attempt), func(time.Time) tea.Msg { return doctorRetryMsg{Token: token, Index: index} })
}

func conciseInstallerOutput(output string) string {
	lines := strings.FieldsFunc(stripANSI(output), func(r rune) bool { return r == '\n' || r == '\r' })
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return "No installer output was captured."
}

func validateDoctorPlan(plan installPlan) error {
	if strings.TrimSpace(plan.Binary) == "" || len(plan.Args) == 0 {
		return errors.New("invalid empty installation plan")
	}
	for _, step := range plan.Prep {
		if strings.TrimSpace(step.Binary) == "" || len(step.Args) == 0 {
			return errors.New("invalid empty installation preparation step")
		}
	}
	return nil
}
