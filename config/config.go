package config

import (
	"claude-squad/log"
	"claude-squad/util"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// Pre-compiled regex patterns for parsing CLAUDE.md
	claudeSquadSectionRe = regexp.MustCompile(`(?i)\[claude-squad\]([\s\S]*?)(?:\n\[|$)`)
	ideCommandRe         = regexp.MustCompile(`(?m)^ide_command\s*[:=]\s*(.+)$`)
	diffCommandRe        = regexp.MustCompile(`(?m)^diff_command\s*[:=]\s*(.+)$`)
)

const (
	ConfigFileName = "config.json"
	defaultProgram = "claude"
)

// GetConfigDir returns the path to the application's configuration directory
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config home directory: %w", err)
	}
	return filepath.Join(homeDir, ".claude-squad"), nil
}

// Config represents the application configuration
type Config struct {
	// DefaultProgram is the default program to run in new instances
	DefaultProgram string `json:"default_program"`
	// AutoYes is a flag to automatically accept all prompts.
	AutoYes bool `json:"auto_yes"`
	// DaemonPollInterval is the interval (ms) at which the daemon polls sessions for autoyes mode.
	DaemonPollInterval int `json:"daemon_poll_interval"`
	// BranchPrefix is the prefix used for git branches created by the application.
	BranchPrefix string `json:"branch_prefix"`
	// DefaultIdeCommand is the default IDE command to use when none is configured per-repo
	DefaultIdeCommand string `json:"default_ide_command"`
	// DefaultDiffCommand is the default external diff command to use when none is configured per-repo
	DefaultDiffCommand string `json:"default_diff_command"`
}

// RepoConfig represents per-repository configuration
type RepoConfig struct {
	// IdeCommand is the IDE command to use for this repository
	IdeCommand string `json:"ide_command,omitempty"`
	// DiffCommand is the external diff command to use for this repository  
	DiffCommand string `json:"diff_command,omitempty"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	program, err := GetClaudeCommand()
	if err != nil {
		log.ErrorLog.Printf("failed to get claude command: %v", err)
		program = defaultProgram
	}

	return &Config{
		DefaultProgram:     program,
		AutoYes:            false,
		DaemonPollInterval: 1000,
		BranchPrefix: func() string {
			user, err := user.Current()
			if err != nil || user == nil || user.Username == "" {
				log.ErrorLog.Printf("failed to get current user: %v", err)
				return "session/"
			}
			return fmt.Sprintf("%s/", strings.ToLower(user.Username))
		}(),
		DefaultIdeCommand:  "webstorm",
		DefaultDiffCommand: "",
	}
}

// GetClaudeCommand attempts to find the "claude" command in the user's shell
// It checks in the following order:
// 1. Shell alias resolution: using "which" command
// 2. PATH lookup
//
// If both fail, it returns an error.
func GetClaudeCommand() (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash" // Default to bash if SHELL is not set
	}

	// Force the shell to load the user's profile and then run the command
	// For zsh, source .zshrc; for bash, source .bashrc
	var shellCmd string
	if strings.Contains(shell, "zsh") {
		shellCmd = "source ~/.zshrc &>/dev/null || true; which claude"
	} else if strings.Contains(shell, "bash") {
		shellCmd = "source ~/.bashrc &>/dev/null || true; which claude"
	} else {
		shellCmd = "which claude"
	}

	cmd := util.Command("Config.detectCodeEditor", shell, "-c", shellCmd)
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		path := strings.TrimSpace(string(output))
		if path != "" {
			// Check if the output is an alias definition and extract the actual path
			// Handle formats like "claude: aliased to /path/to/claude" or other shell-specific formats
			aliasRegex := regexp.MustCompile(`(?:aliased to|->|=)\s*([^\s]+)`)
			matches := aliasRegex.FindStringSubmatch(path)
			if len(matches) > 1 {
				path = matches[1]
			}
			return path, nil
		}
	}

	// Otherwise, try to find in PATH directly
	claudePath, err := exec.LookPath("claude")
	if err == nil {
		return claudePath, nil
	}

	return "", fmt.Errorf("claude command not found in aliases or PATH")
}

func LoadConfig() *Config {
	configDir, err := GetConfigDir()
	if err != nil {
		log.ErrorLog.Printf("failed to get config directory: %v", err)
		return DefaultConfig()
	}

	configPath := filepath.Join(configDir, ConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create and save default config if file doesn't exist
			defaultCfg := DefaultConfig()
			if saveErr := saveConfig(defaultCfg); saveErr != nil {
				log.WarningLog.Printf("failed to save default config: %v", saveErr)
			}
			return defaultCfg
		}

		log.WarningLog.Printf("failed to get config file: %v", err)
		return DefaultConfig()
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		log.ErrorLog.Printf("failed to parse config file: %v", err)
		return DefaultConfig()
	}

	// Merge with defaults for missing fields to handle config file migration
	defaults := DefaultConfig()
	if config.DefaultIdeCommand == "" {
		config.DefaultIdeCommand = defaults.DefaultIdeCommand
	}
	if config.DefaultDiffCommand == "" {
		config.DefaultDiffCommand = defaults.DefaultDiffCommand
	}

	return &config
}

// saveConfig saves the configuration to disk
func saveConfig(config *Config) error {
	configDir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, ConfigFileName)
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
}

// SaveConfig exports the saveConfig function for use by other packages
func SaveConfig(config *Config) error {
	return saveConfig(config)
}

// LoadRepoConfig loads per-repository configuration from CLAUDE.md or .claude-squad/config.json
// It searches for configuration in the following order:
// 1. .claude-squad/config.json in the repository root
// 2. [claude-squad] section in CLAUDE.md in the repository root
// 3. Returns empty RepoConfig if no configuration found
func LoadRepoConfig(repoPath string) *RepoConfig {
	if repoPath == "" {
		return &RepoConfig{}
	}

	// Try .claude-squad/config.json first
	if config := loadRepoConfigFromJSON(repoPath); config != nil {
		return config
	}

	// Try CLAUDE.md second  
	if config := loadRepoConfigFromCLAUDEMD(repoPath); config != nil {
		return config
	}

	return &RepoConfig{}
}

// loadRepoConfigFromJSON loads configuration from .claude-squad/config.json in repo root
func loadRepoConfigFromJSON(repoPath string) *RepoConfig {
	configPath := filepath.Join(repoPath, ".claude-squad", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var config RepoConfig
	if err := json.Unmarshal(data, &config); err != nil {
		log.WarningLog.Printf("failed to parse repo config at %s: %v", configPath, err)
		return nil
	}

	return &config
}

// loadRepoConfigFromCLAUDEMD loads configuration from [claude-squad] section in CLAUDE.md
func loadRepoConfigFromCLAUDEMD(repoPath string) *RepoConfig {
	claudePath := filepath.Join(repoPath, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if err != nil {
		return nil
	}

	content := string(data)
	
	// Look for [claude-squad] section
	matches := claudeSquadSectionRe.FindStringSubmatch(content)
	if len(matches) < 2 {
		return nil
	}

	configSection := matches[1]
	config := &RepoConfig{}

	// Parse ide_command
	if ideMatches := ideCommandRe.FindStringSubmatch(configSection); len(ideMatches) > 1 {
		config.IdeCommand = strings.TrimSpace(ideMatches[1])
	}

	// Parse diff_command  
	if diffMatches := diffCommandRe.FindStringSubmatch(configSection); len(diffMatches) > 1 {
		config.DiffCommand = strings.TrimSpace(diffMatches[1])
	}

	return config
}

// GetEffectiveIdeCommand returns the IDE command to use, checking repo config first, then global config
func GetEffectiveIdeCommand(repoPath string, globalConfig *Config) string {
	repoConfig := LoadRepoConfig(repoPath)
	if repoConfig.IdeCommand != "" {
		return repoConfig.IdeCommand
	}
	if globalConfig != nil && globalConfig.DefaultIdeCommand != "" {
		return globalConfig.DefaultIdeCommand
	}
	return "webstorm" // fallback
}

// GetEffectiveDiffCommand returns the diff command to use, checking repo config first, then global config  
func GetEffectiveDiffCommand(repoPath string, globalConfig *Config) string {
	repoConfig := LoadRepoConfig(repoPath)
	if repoConfig.DiffCommand != "" {
		return repoConfig.DiffCommand
	}
	if globalConfig != nil && globalConfig.DefaultDiffCommand != "" {
		return globalConfig.DefaultDiffCommand
	}
	return "" // empty means use built-in diff viewer
}
