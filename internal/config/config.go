// Package config provides the platform-appropriate config directory path
// for the gmail-mcp-server application.
package config

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
)

const appName = "gmail-mcp-server"

// xdgConfigDir returns $XDG_CONFIG_HOME, falling back to ~/.config.
func xdgConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return xdg
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config")
}

// AppDataDir returns the platform-appropriate config directory for the app.
//   - Linux/Windows: $XDG_CONFIG_HOME/gmail-mcp-server (or ~/.config/… / %AppData%/…)
//   - macOS: ~/Library/Application Support/gmail-mcp-server wins if it already
//     exists; otherwise falls back to ~/.config/gmail-mcp-server so CLI-oriented
//     setups can use XDG on all platforms.
func AppDataDir() string {
	var candidates []string

	if runtime.GOOS == "darwin" {
		// Native macOS path wins if it already exists (preserves existing installs).
		if nativeBase, err := os.UserConfigDir(); err == nil {
			candidates = append(candidates, filepath.Join(nativeBase, appName))
		}
		// XDG path is the default for new installs.
		if xdg := xdgConfigDir(); xdg != "" {
			candidates = append(candidates, filepath.Join(xdg, appName))
		}
	} else {
		if base, err := os.UserConfigDir(); err == nil {
			candidates = append(candidates, filepath.Join(base, appName))
		}
	}

	if len(candidates) == 0 {
		log.Printf("Warning: Could not determine config directory")
		return "."
	}

	// Use the first candidate that already exists; otherwise use the last one
	// (lowest priority / most portable) as the creation target.
	chosen := candidates[len(candidates)-1]
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			chosen = c
			break
		}
	}

	if err := os.MkdirAll(chosen, 0o755); err != nil {
		log.Printf("Warning: Could not create config directory %s: %v", chosen, err)
		return "."
	}
	return chosen
}

// AppFilePath returns an absolute path within the app data directory.
func AppFilePath(filename string) string {
	return filepath.Join(AppDataDir(), filename)
}
