package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/austinamoruso/configuration/keys"
)

// KeyBinding represents a custom keybinding configuration
type KeyBinding struct {
	Command string   `json:"command"` // The command name (e.g., "up", "down", "new")
	Keys    []string `json:"keys"`    // The key combinations (e.g., ["k", "up"])
	Help    string   `json:"help"`    // Help text to display
}

// KeyBindingsConfig stores all custom keybindings
type KeyBindingsConfig struct {
	Version  string       `json:"version"`  // Config version for future migrations
	Bindings []KeyBinding `json:"bindings"` // List of custom keybindings
}

// DefaultKeyBindings returns the default keybindings configuration
func DefaultKeyBindings() *KeyBindingsConfig {
	return &KeyBindingsConfig{
		Version: "1.0",
		Bindings: []KeyBinding{
			// Navigation
			{Command: "up", Keys: []string{"up", "k"}, Help: "↑/k"},
			{Command: "down", Keys: []string{"down", "j"}, Help: "↓/j"},
			{Command: "home", Keys: []string{"home", "ctrl+a", "ctrl+home"}, Help: "home/ctrl+a"},
			{Command: "end", Keys: []string{"end", "ctrl+e", "ctrl+end"}, Help: "end/ctrl+e"},
			{Command: "page_up", Keys: []string{"pgup"}, Help: "pgup"},
			{Command: "page_down", Keys: []string{"pgdown"}, Help: "pgdn"},
			
			// Instance management
			{Command: "new", Keys: []string{"n"}, Help: "n"},
			{Command: "new_with_prompt", Keys: []string{"N"}, Help: "N"},
			{Command: "existing_branch", Keys: []string{"e"}, Help: "e"},
			{Command: "kill", Keys: []string{"D"}, Help: "D"},
			{Command: "checkout", Keys: []string{"c"}, Help: "c"},
			{Command: "resume", Keys: []string{"r"}, Help: "r"},
			{Command: "push", Keys: []string{"p"}, Help: "p"},
			{Command: "rebase", Keys: []string{"b"}, Help: "b"},
			
			// Diff view
			{Command: "scroll_up", Keys: []string{"shift+up"}, Help: "shift+↑"},
			{Command: "scroll_down", Keys: []string{"shift+down"}, Help: "shift+↓"},
			{Command: "prev_file", Keys: []string{"alt+up"}, Help: "alt+↑"},
			{Command: "next_file", Keys: []string{"alt+down"}, Help: "alt+↓"},
			{Command: "diff_all", Keys: []string{"a"}, Help: "a"},
			{Command: "diff_last_commit", Keys: []string{"d"}, Help: "d"},
			{Command: "prev_commit", Keys: []string{"left"}, Help: "←"},
			{Command: "next_commit", Keys: []string{"right"}, Help: "→"},
			{Command: "scroll_lock", Keys: []string{"s"}, Help: "s"},
			
			// Actions
			{Command: "enter", Keys: []string{"enter", "o"}, Help: "↵/o"},
			{Command: "tab", Keys: []string{"tab"}, Help: "tab"},
			{Command: "help", Keys: []string{"?"}, Help: "?"},
			{Command: "quit", Keys: []string{"q"}, Help: "q"},
			{Command: "error_log", Keys: []string{"l"}, Help: "l"},
			{Command: "webstorm", Keys: []string{"w"}, Help: "w"},
			{Command: "open_in_ide", Keys: []string{"i"}, Help: "i"},
		},
	}
}

// LoadKeyBindings loads keybindings from the config file
func LoadKeyBindings() (*KeyBindingsConfig, error) {
	configPath := filepath.Join(os.Getenv("HOME"), ".claude-squad", "keybindings.json")
	
	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Return defaults if file doesn't exist
		return DefaultKeyBindings(), nil
	}
	
	// Read the file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	
	// Parse JSON
	var config KeyBindingsConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	
	// If no bindings are defined, use defaults
	if len(config.Bindings) == 0 {
		return DefaultKeyBindings(), nil
	}
	
	return &config, nil
}

// SaveKeyBindings saves keybindings to the config file
func (k *KeyBindingsConfig) Save() error {
	configPath := filepath.Join(os.Getenv("HOME"), ".claude-squad", "keybindings.json")
	
	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	
	// Marshal to JSON with pretty printing
	data, err := json.MarshalIndent(k, "", "  ")
	if err != nil {
		return err
	}
	
	// Write to file
	return os.WriteFile(configPath, data, 0644)
}

// ToKeyMap converts the keybindings config to a map for easy lookup
func (k *KeyBindingsConfig) ToKeyMap() map[string]keys.KeyName {
	keyMap := make(map[string]keys.KeyName)
	
	// Map command names to KeyName constants
	commandToKeyName := map[string]keys.KeyName{
		"up":               keys.KeyUp,
		"down":             keys.KeyDown,
		"enter":            keys.KeyEnter,
		"new":              keys.KeyNew,
		"new_with_prompt":  keys.KeyPrompt,
		"existing_branch":  keys.KeyExistingBranch,
		"kill":             keys.KeyKill,
		"quit":             keys.KeyQuit,
		"push":             keys.KeySubmit,
		"checkout":         keys.KeyCheckout,
		"resume":           keys.KeyResume,
		"help":             keys.KeyHelp,
		"error_log":        keys.KeyErrorLog,
		"webstorm":         keys.KeyWebStorm,
		"rebase":           keys.KeyRebase,
		"tab":              keys.KeyTab,
		"scroll_up":        keys.KeyShiftUp,
		"scroll_down":      keys.KeyShiftDown,
		"home":             keys.KeyHome,
		"end":              keys.KeyEnd,
		"page_up":          keys.KeyPageUp,
		"page_down":        keys.KeyPageDown,
		"prev_file":        keys.KeyAltUp,
		"next_file":        keys.KeyAltDown,
		"diff_all":         keys.KeyDiffAll,
		"diff_last_commit": keys.KeyDiffLastCommit,
		"prev_commit":      keys.KeyLeft,
		"next_commit":      keys.KeyRight,
		"scroll_lock":      keys.KeyScrollLock,
		"open_in_ide":      keys.KeyOpenInIDE,
	}
	
	// Build the key map
	for _, binding := range k.Bindings {
		if keyName, ok := commandToKeyName[binding.Command]; ok {
			for _, key := range binding.Keys {
				keyMap[key] = keyName
			}
		}
	}
	
	return keyMap
}

// GetBinding returns the keybinding for a specific command
func (k *KeyBindingsConfig) GetBinding(command string) *KeyBinding {
	for _, binding := range k.Bindings {
		if binding.Command == command {
			return &binding
		}
	}
	return nil
}

// SetBinding updates or adds a keybinding for a command
func (k *KeyBindingsConfig) SetBinding(command string, keys []string, help string) {
	for i, binding := range k.Bindings {
		if binding.Command == command {
			k.Bindings[i].Keys = keys
			k.Bindings[i].Help = help
			return
		}
	}
	
	// Add new binding if not found
	k.Bindings = append(k.Bindings, KeyBinding{
		Command: command,
		Keys:    keys,
		Help:    help,
	})
}

// ValidateBindings checks for conflicts in keybindings
func (k *KeyBindingsConfig) ValidateBindings() map[string][]string {
	conflicts := make(map[string][]string)
	keyToCommands := make(map[string][]string)
	
	// Build map of keys to commands
	for _, binding := range k.Bindings {
		for _, key := range binding.Keys {
			keyToCommands[key] = append(keyToCommands[key], binding.Command)
		}
	}
	
	// Find conflicts
	for key, commands := range keyToCommands {
		if len(commands) > 1 {
			conflicts[key] = commands
		}
	}
	
	return conflicts
}