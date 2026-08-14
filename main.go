package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// Release workflows replace these values with -ldflags. The defaults make it
// clear when a binary was built from a working tree rather than an official
// release artifact.
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	if os.Getenv("ANI_TUI_MENU_HELPER") == "1" {
		os.Exit(runMenuHelper())
	}

	flag.Usage = func() {
		output := flag.CommandLine.Output()
		fmt.Fprintln(output, "ani-tui — a full-screen terminal interface for ani-cli")
		fmt.Fprintln(output)
		fmt.Fprintln(output, "Usage: ani-tui [options]")
		fmt.Fprintln(output)
		fmt.Fprintln(output, "Options:")
		flag.PrintDefaults()
		fmt.Fprintln(output)
		fmt.Fprintln(output, "Project: https://github.com/Killersumit/ani-tui")
	}

	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "print version and build information")
	flag.BoolVar(&showVersion, "v", false, "print version and build information (shorthand)")
	showConfig := flag.Bool("print-config", false, "print the configuration file path")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "ani-tui: unexpected argument %q\n\n", flag.Arg(0))
		flag.Usage()
		os.Exit(2)
	}
	if showVersion {
		fmt.Println(versionString())
		return
	}
	if *showConfig {
		fmt.Println(configPath())
		return
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal("load configuration", err)
	}
	bridge, err := newBridgeServer()
	if err != nil {
		fatal("start menu bridge", err)
	}
	defer bridge.close()

	self, err := os.Executable()
	if err != nil {
		fatal("find ani-tui executable", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(self); resolveErr == nil {
		self = resolved
	}

	pm := newProcessManager()
	initial := newModel(cfg, bridge, pm, self)
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if cfg.Mouse {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	program := tea.NewProgram(initial, opts...)
	if _, err := program.Run(); err != nil {
		pm.cancel()
		fatal("run terminal interface", err)
	}
	pm.cancel()
}

func versionString() string {
	result := fmt.Sprintf("ani-tui %s", version)
	if commit != "" && commit != "unknown" {
		result += fmt.Sprintf(" (commit %s", commit)
		if buildDate != "" && buildDate != "unknown" {
			result += fmt.Sprintf(", built %s", buildDate)
		}
		result += ")"
	}
	return result
}

func fatal(context string, err error) {
	fmt.Fprintf(os.Stderr, "ani-tui: %s: %v\n", context, err)
	os.Exit(1)
}
