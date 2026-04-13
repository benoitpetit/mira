package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// LoadOrDefault loads configuration from file if it exists, otherwise returns defaults.
// It also honors the MIRA_DATA_PATH environment variable to override storage path.
func LoadOrDefault(path string) (*Config, error) {
	if path == "" {
		path = ResolveConfigPath("")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := Default()
		if dataPath := os.Getenv("MIRA_DATA_PATH"); dataPath != "" {
			cfg.Storage.Path = dataPath
		}
		return cfg, nil
	}
	return Load(path)
}

// ResolveConfigPath finds the best configuration file path using multi-platform fallbacks.
// Resolution order:
//   1. preferred argument (if non-empty)
//   2. MIRA_CONFIG environment variable
//   3. ./config.yaml in current working directory
//   4. $XDG_CONFIG_HOME/mira/config.yaml  (Linux)
//   5. $HOME/.config/mira/config.yaml      (Linux fallback)
//   6. ~/Library/Application Support/mira/config.yaml (macOS)
//   7. %APPDATA%/mira/config.yaml         (Windows)
func ResolveConfigPath(preferred string) string {
	if preferred != "" {
		return preferred
	}
	if envPath := os.Getenv("MIRA_CONFIG"); envPath != "" {
		if abs, err := filepath.Abs(envPath); err == nil {
			return abs
		}
		return envPath
	}
	if cwdPath, err := filepath.Abs("config.yaml"); err == nil {
		if _, err := os.Stat(cwdPath); err == nil {
			return cwdPath
		}
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = fallbackConfigDir()
	}
	return filepath.Join(configDir, "mira", "config.yaml")
}

// fallbackConfigDir returns a reasonable config directory when os.UserConfigDir fails.
func fallbackConfigDir() string {
	switch runtime.GOOS {
	case "windows":
		if home := os.Getenv("USERPROFILE"); home != "" {
			return filepath.Join(home, ".config")
		}
	case "darwin":
		if home := os.Getenv("HOME"); home != "" {
			return filepath.Join(home, "Library", "Application Support")
		}
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".config")
	}
	return "."
}
