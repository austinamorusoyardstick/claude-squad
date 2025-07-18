package keys

import (
	"github.com/charmbracelet/bubbles/key"
)

type KeyName int

const (
	KeyUp KeyName = iota
	KeyDown
	KeyEnter
	KeyNew
	KeyKill
	KeyQuit
	KeyReview
	KeyPush
	KeySubmit
	KeyPRReview

	KeyTab        // Tab is a special keybinding for switching between panes.
	KeySubmitName // SubmitName is a special keybinding for submitting the name of a new instance.

	KeyCheckout
	KeyResume
	KeyPrompt         // New key for entering a prompt
	KeyHelp           // Key for showing help screen
	KeyExistingBranch // Key for creating instance from existing branch
	KeyErrorLog       // Key for showing error log
	KeyWebStorm       // Key for opening WebStorm
	KeyRebase         // Key for rebasing with main branch

	// Diff keybindings
	KeyShiftUp
	KeyShiftDown
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyAltUp
	KeyAltDown
	KeyDiffAll
	KeyDiffLastCommit
	KeyLeft
	KeyRight
	KeyScrollLock
	KeyOpenInIDE
)

// GlobalKeyStringsMap is a global, immutable map string to keybinding.
var GlobalKeyStringsMap = map[string]KeyName{
	"up":         KeyUp,
	"k":          KeyUp,
	"down":       KeyDown,
	"j":          KeyDown,
	"shift+up":   KeyShiftUp,
	"shift+down": KeyShiftDown,
	"home":       KeyHome,
	"end":        KeyEnd,
	"ctrl+a":     KeyHome,
	"ctrl+e":     KeyEnd,
	"ctrl+home":  KeyHome,
	"ctrl+end":   KeyEnd,
	"pgup":       KeyPageUp,
	"pgdown":     KeyPageDown,
	"alt+up":     KeyAltUp,
	"alt+down":   KeyAltDown,
	"a":          KeyDiffAll,
	"d":          KeyDiffLastCommit,
	"left":       KeyLeft,
	"right":      KeyRight,
	"s":          KeyScrollLock,
	"N":          KeyPrompt,
	"enter":      KeyEnter,
	"o":          KeyEnter,
	"n":          KeyNew,
	"e":          KeyExistingBranch,
	"D":          KeyKill,
	"q":          KeyQuit,
	"tab":        KeyTab,
	"c":          KeyCheckout,
	"r":          KeyResume,
	"p":          KeySubmit,
	"?":          KeyHelp,
	"l":          KeyErrorLog,
	"w":          KeyWebStorm,
	"i":          KeyOpenInIDE,
	"b":          KeyRebase,
	"R":          KeyPRReview,
}

// GlobalkeyBindings is a global, immutable map of KeyName tot keybinding.
var GlobalkeyBindings = map[KeyName]key.Binding{
	KeyUp: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	KeyDown: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	KeyShiftUp: key.NewBinding(
		key.WithKeys("shift+up"),
		key.WithHelp("shift+↑", "scroll"),
	),
	KeyShiftDown: key.NewBinding(
		key.WithKeys("shift+down"),
		key.WithHelp("shift+↓", "scroll"),
	),
	KeyHome: key.NewBinding(
		key.WithKeys("home", "ctrl+a", "ctrl+home"),
		key.WithHelp("home/ctrl+a", "scroll to top"),
	),
	KeyEnd: key.NewBinding(
		key.WithKeys("end", "ctrl+e", "ctrl+end"),
		key.WithHelp("end/ctrl+e", "scroll to bottom"),
	),
	KeyPageUp: key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("pgup", "page up"),
	),
	KeyPageDown: key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("pgdn", "page down"),
	),
	KeyAltUp: key.NewBinding(
		key.WithKeys("alt+up"),
		key.WithHelp("alt+↑", "prev file"),
	),
	KeyAltDown: key.NewBinding(
		key.WithKeys("alt+down"),
		key.WithHelp("alt+↓", "next file"),
	),
	KeyDiffAll: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "all changes"),
	),
	KeyDiffLastCommit: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "last commit diff"),
	),
	KeyLeft: key.NewBinding(
		key.WithKeys("left"),
		key.WithHelp("←", "prev commit"),
	),
	KeyRight: key.NewBinding(
		key.WithKeys("right"),
		key.WithHelp("→", "next commit"),
	),
	KeyScrollLock: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "toggle scroll lock"),
	),
	KeyOpenInIDE: key.NewBinding(
		key.WithKeys("i"),
		key.WithHelp("i", "open in IDE"),
	),
	KeyEnter: key.NewBinding(
		key.WithKeys("enter", "o"),
		key.WithHelp("↵/o", "open"),
	),
	KeyNew: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new"),
	),
	KeyExistingBranch: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "existing branch"),
	),
	KeyKill: key.NewBinding(
		key.WithKeys("D"),
		key.WithHelp("D", "kill"),
	),
	KeyHelp: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	KeyQuit: key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "quit"),
	),
	KeySubmit: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "push branch"),
	),
	KeyPrompt: key.NewBinding(
		key.WithKeys("N"),
		key.WithHelp("N", "new with prompt"),
	),
	KeyCheckout: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "checkout"),
	),
	KeyTab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch tab"),
	),
	KeyResume: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "resume"),
	),
	KeyErrorLog: key.NewBinding(
		key.WithKeys("l"),
		key.WithHelp("l", "error log"),
	),
	KeyWebStorm: key.NewBinding(
		key.WithKeys("w"),
		key.WithHelp("w", "open WebStorm"),
	),
	KeyRebase: key.NewBinding(
		key.WithKeys("b"),
		key.WithHelp("b", "rebase"),
	),
	KeyPRReview: key.NewBinding(
		key.WithKeys("R"),
		key.WithHelp("R", "review PR comments"),
	),

	// -- Special keybindings --

	KeySubmitName: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "submit name"),
	),
}
