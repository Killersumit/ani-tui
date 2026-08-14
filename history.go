package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type historyEntry struct {
	Episode string
	ID      string
	Title   string
}

func historyPath(cfg Config) string {
	root := strings.TrimSpace(cfg.HistoryDir)
	if root == "" {
		root = os.Getenv("ANI_CLI_HIST_DIR")
	}
	if root == "" {
		root = os.Getenv("XDG_STATE_HOME")
		if root == "" {
			home, err := os.UserHomeDir()
			if err == nil {
				root = filepath.Join(home, ".local", "state")
			} else {
				root = "."
			}
		}
		root = filepath.Join(root, "ani-cli")
	}
	return filepath.Join(root, "ani-hsts")
}

func loadHistory(cfg Config) ([]historyEntry, error) {
	file, err := os.Open(historyPath(cfg))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var result []historyEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "\t", 3)
		if len(parts) != 3 {
			continue
		}
		result = append(result, historyEntry{Episode: parts[0], ID: parts[1], Title: parts[2]})
	}
	return result, scanner.Err()
}
