package app

import (
	"claude-squad/config"
	"claude-squad/keys"
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/session/git"
	"claude-squad/ui"
	"claude-squad/ui/overlay"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const GlobalInstanceLimit = 10

// Run is the main entrypoint into the application.
func Run(ctx context.Context, program string, autoYes bool) error {
	p := tea.NewProgram(
		newHome(ctx, program, autoYes),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(), // Mouse scroll
	)
	_, err := p.Run()
	return err
}

type state int

const (
	stateDefault state = iota
	// stateNew is the state when the user is creating a new instance.
	stateNew
	// statePrompt is the state when the user is entering a prompt.
	statePrompt
	// stateHelp is the state when a help screen is displayed.
	stateHelp
	// stateConfirm is the state when a confirmation modal is displayed.
	stateConfirm
	// stateBranchSelect is the state when the user is selecting a branch.
	stateBranchSelect
	// stateErrorLog is the state when displaying the error log.
	stateErrorLog
	// statePRReview is the state when reviewing PR comments.
	statePRReview
	// stateBookmark is the state when creating a bookmark commit.
	stateBookmark
	// stateHistory is the state when displaying the history overlay.
	stateHistory
	// stateKeybindingEditor is the state when editing keybindings.
	stateKeybindingEditor
	// stateGitStatus is the state when displaying the git status overlay.
	stateGitStatus
	// stateCommentDetail is the state when displaying full PR comment content.
	stateCommentDetail
)

// Message types for tea.Cmd
type clearSuccessMsg struct{}
type startRebaseMsg struct{}
type instanceUpdatedMsg struct {
	instance *session.Instance
}
type rebaseUpdateMsg struct {
	err      error
	message  string
	complete bool
}

// Error messages
const (
	instancePausedError = "instance '%s' is paused - please resume it first"
	noPullRequestFoundError = "no pull request found for current branch: %w"
)

type home struct {
	ctx context.Context

	// -- Storage and Configuration --

	program string
	autoYes bool

	// storage is the interface for saving/loading data to/from the app's state
	storage *session.Storage
	// appConfig stores persistent application configuration
	appConfig *config.Config
	// appState stores persistent application state like seen help screens
	appState config.AppState
	// updateChecker checks for application updates
	updateChecker *UpdateChecker

	// -- State --

	// state is the current discrete state of the application
	state state
	// scrollLocked indicates if up/down keys should scroll in diff view without shift
	scrollLocked bool
	// newInstanceFinalizer is called when the state is stateNew and then you press enter.
	// It registers the new instance in the list after the instance has been started.
	newInstanceFinalizer func()

	// promptAfterName tracks if we should enter prompt mode after naming
	promptAfterName bool

	// keySent is used to manage underlining menu items
	keySent bool

	// Window dimensions
	windowWidth  int
	windowHeight int

	// pendingCmd stores a command to be executed after confirmation
	pendingCmd tea.Cmd

	// -- UI Components --

	// list displays the list of instances
	list *ui.List
	// menu displays the bottom menu
	menu *ui.Menu
	// tabbedWindow displays the tabbed window with AI, diff, and terminal panes
	tabbedWindow *ui.TabbedWindow
	// errBox displays error messages
	errBox *ui.ErrBox
	// global spinner instance. we plumb this down to where it's needed
	spinner spinner.Model
	// textInputOverlay handles text input with state
	textInputOverlay *overlay.TextInputOverlay
	// textOverlay displays text information
	textOverlay *overlay.TextOverlay
	// confirmationOverlay displays confirmation modals
	confirmationOverlay *overlay.ConfirmationOverlay
	// branchSelectorOverlay displays branch selection interface
	branchSelectorOverlay *overlay.BranchSelectorOverlay
	// prReviewOverlay handles PR comment review
	prReviewOverlay *ui.PRReviewModel
	// historyOverlay displays scrollable history content
	historyOverlay *overlay.HistoryOverlay
	// commentDetailOverlay displays full PR comment content
	commentDetailOverlay *overlay.CommentDetailOverlay
	// keybindingEditorOverlay displays keybinding editor interface
	keybindingEditorOverlay *overlay.KeybindingEditorOverlay
	// gitStatusOverlay displays git status information
	gitStatusOverlay *overlay.GitStatusOverlay

	// errorLog stores all error messages for display
	errorLog []string
	
	// pendingRebaseInstance stores the instance to rebase after confirmation
	pendingRebaseInstance *session.Instance
	
	// rebaseInProgress indicates if a rebase is currently in progress
	rebaseInProgress bool
	// rebaseInstance is the instance being rebased
	rebaseInstance *session.Instance
	// rebaseBranchName is the branch being rebased
	rebaseBranchName string
	// rebaseOriginalSHA is the commit SHA before rebase started
	rebaseOriginalSHA string
}

func newHome(ctx context.Context, program string, autoYes bool) *home {
	// Load application config
	appConfig := config.LoadConfig()

	// Load application state
	appState := config.LoadState()

	// Initialize custom keybindings
	if err := keys.InitializeCustomKeyBindings(); err != nil {
		// Log error but continue with defaults
		log.ErrorLog.Printf("Failed to load custom keybindings: %v", err)
	}

	// Initialize storage
	storage, err := session.NewStorage(appState)
	if err != nil {
		fmt.Printf("Failed to initialize storage: %v\n", err)
		os.Exit(1)
	}

	// Create update checker
	updateChecker := NewUpdateChecker()
	updateChecker.StartBackgroundCheck()

	menu := ui.NewMenu()
	menu.SetUpdateChecker(updateChecker)

	h := &home{
		ctx:           ctx,
		spinner:       spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		menu:          menu,
		tabbedWindow:  ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewTerminalPane(), ui.NewJestPane(appConfig)),
		errBox:        ui.NewErrBox(),
		storage:       storage,
		appConfig:     appConfig,
		program:       program,
		autoYes:       autoYes,
		state:         stateDefault,
		appState:      appState,
		updateChecker: updateChecker,
	}
	h.list = ui.NewList(&h.spinner, autoYes)

	// Load saved instances
	instances, err := storage.LoadInstances()
	if err != nil {
		h.errBox.Set("Failed to load instances: " + err.Error())
	} else {
		h.list.SetInstances(instances)
	}

	// Set initial update status from the checker
	menu.SetUpdateStatus(updateChecker.GetStatus())

	return h
}

// Init initializes the model.
func (m *home) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.tabbedWindow.Init(),
		m.instanceChanged(),
		m.checkForRunningInstances(),
		m.checkGitHooks(),
		// Check for update results immediately
		func() tea.Msg {
			// This will be handled in Update to refresh the update status
			return struct{}{}
		},
	)
}

// checkGitHooks checks if git hooks are installed and returns a message if they need to be installed
func (m *home) checkGitHooks() tea.Cmd {
	return func() tea.Msg {
		// Get the git root directory
		gitRoot, err := git.FindRepoRoot(".")
		if err != nil {
			// Not in a git repo, skip hook check
			return nil
		}

		// Check if the claude-squad git hooks are installed
		hookPath := filepath.Join(gitRoot, ".git", "hooks", "prepare-commit-msg")
		
		// Check if hook exists
		if _, err := os.Stat(hookPath); os.IsNotExist(err) {
			// Hook doesn't exist, suggest installation
			return fmt.Errorf("Git hooks not installed. Run 'claude-squad install-hooks' to enable commit message generation")
		}

		// Check if it's our hook by looking for our marker
		content, err := os.ReadFile(hookPath)
		if err != nil {
			return nil
		}

		if !strings.Contains(string(content), "claude-squad-hook") {
			return fmt.Errorf("Custom git hooks detected. Run 'claude-squad install-hooks --force' to replace with claude-squad hooks")
		}

		return nil
	}
}

// checkForRunningInstances checks if any instances are already running
func (m *home) checkForRunningInstances() tea.Cmd {
	return func() tea.Msg {
		// Check each instance to see if it's running
		for _, instance := range m.list.GetInstances() {
			if instance.TmuxAlive() && !instance.Started() {
				// Mark the instance as started since tmux session exists
				instance.SetStarted(true)
			}
		}
		return nil
	}
}

// confirmAction shows a confirmation modal and executes the given command if confirmed
func (m *home) confirmAction(message string, action func() tea.Cmd) tea.Cmd {
	m.state = stateConfirm
	m.confirmationOverlay = overlay.NewConfirmationOverlay(message)
	m.confirmationOverlay.OnConfirm = func() {
		m.state = stateDefault
		m.confirmationOverlay = nil
		// Execute the action to get a tea.Cmd
		m.pendingCmd = action()
	}
	m.confirmationOverlay.OnCancel = func() {
		m.state = stateDefault
		m.confirmationOverlay = nil
		m.pendingCmd = nil
	}
	return nil
}

func (m *home) resize(w, h int) {
	m.windowWidth = w
	m.windowHeight = h

	// Preserve menu height
	menuHeight := m.menu.Height()

	// Calculate heights
	errBoxHeight := 0
	if m.errBox.HasError() {
		errBoxHeight = 3 // Error box takes 3 lines when shown
	}

	// Update list width and calculate its actual height
	m.list.SetWidth(m.windowWidth)
	listHeight := m.list.CalculateHeight(m.windowHeight - menuHeight - errBoxHeight)
	m.list.SetHeight(listHeight)

	// The tabbed window gets everything else
	tabbedHeight := m.windowHeight - listHeight - menuHeight - errBoxHeight
	m.tabbedWindow.SetSize(m.windowWidth, tabbedHeight)

	// Update other components
	m.menu.SetWidth(m.windowWidth)
	m.errBox.SetSize(m.windowWidth, errBoxHeight)

	// Resize overlays
	if m.textInputOverlay != nil {
		m.textInputOverlay.SetSize(m.windowWidth, m.windowHeight)
	}
	if m.textOverlay != nil {
		m.textOverlay.SetSize(m.windowWidth, m.windowHeight)
	}
	if m.confirmationOverlay != nil {
		m.confirmationOverlay.SetSize(m.windowWidth, m.windowHeight)
	}
	if m.branchSelectorOverlay != nil {
		m.branchSelectorOverlay.SetSize(m.windowWidth, m.windowHeight)
	}
	if m.prReviewOverlay != nil {
		_, cmd := m.prReviewOverlay.Update(tea.WindowSizeMsg{Width: m.windowWidth, Height: m.windowHeight})
		if cmd != nil {
			// Handle any commands from resize if needed
		}
	}
	if m.historyOverlay != nil {
		m.historyOverlay.SetSize(m.windowWidth, m.windowHeight)
	}
	if m.commentDetailOverlay != nil {
		m.commentDetailOverlay.SetSize(m.windowWidth, m.windowHeight)
	}
	if m.keybindingEditorOverlay != nil {
		m.keybindingEditorOverlay.SetSize(m.windowWidth, m.windowHeight)
	}
	if m.gitStatusOverlay != nil {
		m.gitStatusOverlay.SetSize(m.windowWidth, m.windowHeight)
	}
}

// Update updates the model based on the message received.
func (m *home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Check for pending command execution
	if m.pendingCmd != nil {
		cmd := m.pendingCmd
		m.pendingCmd = nil
		return m, cmd
	}

	// Always update the update status
	m.menu.SetUpdateStatus(m.updateChecker.GetStatus())

	switch msg := msg.(type) {
	case startRebaseMsg:
		// Execute the actual rebase
		if m.pendingRebaseInstance != nil {
			cmd := m.startRebase(m.pendingRebaseInstance)
			m.pendingRebaseInstance = nil
			return m, cmd
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case ui.SelectedTabChangedMsg:
		// Update menu state based on active tab
		m.menu.SetInDiffTab(m.tabbedWindow.IsInDiffTab())
		return m, nil
	case tea.KeyMsg:
		// Handle special keys first based on state
		if m.state == stateConfirm && m.confirmationOverlay != nil {
			// Let the confirmation overlay handle the key
			handled := m.confirmationOverlay.HandleKey(msg)
			if handled {
				return m, nil
			}
		}

		// Convert tea.KeyMsg to our KeyName
		keyName, ok := keys.GetKeyName(msg.String())
		if !ok {
			// If key is not mapped, check for special navigation keys in different states
			if m.state == stateHistory && m.historyOverlay != nil {
				m.historyOverlay.Update(msg)
				return m, nil
			}
			// Pass through to components that might handle unmapped keys
			if m.state == stateNew && m.textInputOverlay != nil {
				m.textInputOverlay.Update(msg)
				return m, nil
			}
			if m.state == statePrompt && m.textInputOverlay != nil {
				m.textInputOverlay.Update(msg)
				return m, nil
			}
			if m.state == stateBookmark && m.textInputOverlay != nil {
				m.textInputOverlay.Update(msg)
				return m, nil
			}
			if m.state == stateBranchSelect && m.branchSelectorOverlay != nil {
				updated, cmd := m.branchSelectorOverlay.Update(msg)
				if u, ok := updated.(*overlay.BranchSelectorOverlay); ok {
					m.branchSelectorOverlay = u
				}
				return m, cmd
			}
			if m.state == statePRReview && m.prReviewOverlay != nil {
				// Handle PR review navigation
				updatedModel, cmd := m.prReviewOverlay.Update(msg)
				if u, ok := updatedModel.(*ui.PRReviewModel); ok {
					m.prReviewOverlay = u
				}
				return m, cmd
			}
			if m.state == stateCommentDetail && m.commentDetailOverlay != nil {
				m.commentDetailOverlay.Update(msg)
				return m, nil
			}
			if m.state == stateKeybindingEditor && m.keybindingEditorOverlay != nil {
				updated, cmd := m.keybindingEditorOverlay.Update(msg)
				if u, ok := updated.(*overlay.KeybindingEditorOverlay); ok {
					m.keybindingEditorOverlay = u
				}
				return m, cmd
			}
			if m.state == stateGitStatus && m.gitStatusOverlay != nil {
				m.gitStatusOverlay.Update(msg)
				return m, nil
			}
			return m, nil
		}

		// Handle key press
		return m.handleKeyPress(msg)

	case overlay.BranchSelectedMsg:
		m.state = stateDefault
		m.branchSelectorOverlay = nil
		// Create instance from the selected branch
		return m, m.createInstanceFromBranch(string(msg))

	case ui.OpenPRCommentDetailMsg:
		// Show the comment detail overlay
		m.state = stateCommentDetail
		m.commentDetailOverlay = overlay.NewCommentDetailOverlay(msg.Comment)
		m.commentDetailOverlay.OnDismiss = func() {
			m.state = statePRReview
			m.commentDetailOverlay = nil
		}
		return m, nil

	case ui.PRReviewCompleteMsg:
		// Return to default state
		m.state = stateDefault
		m.prReviewOverlay = nil
		return m, nil

	case ui.NavigateToDiffMsg:
		// Close PR review and navigate to diff
		m.state = stateDefault
		m.prReviewOverlay = nil
		
		// Switch to diff tab
		m.tabbedWindow.ActivateDiffTab()
		m.menu.SetInDiffTab(true)
		
		// Navigate to the file and line
		return m, m.tabbedWindow.NavigateToFileAndLine(msg.FilePath, msg.LineNumber)

	case ui.OpenFileInIDEMsg:
		// Open the file in IDE at the specific line
		selected := m.list.GetSelectedInstance()
		if selected != nil {
			return m, m.openFileInIDEAtLine(selected, msg.FilePath, msg.LineNumber)
		}
		return m, nil

	case error:
		// Don't add errors to the log while showing error log
		if m.state != stateErrorLog {
			m.errorLog = append(m.errorLog, msg.Error())
		}
		m.errBox.Set(msg.Error())
		return m, nil
	case string:
		// Success message - show in error box with different styling
		m.errBox.SetSuccess(msg)
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return clearSuccessMsg{}
		})
	case clearSuccessMsg:
		// Clear success message after timeout
		m.errBox.Clear()
		return m, nil
	case instanceUpdatedMsg:
		// Update the instance in the list
		m.list.UpdateInstance(msg.instance)
		// Save instances to disk
		if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
			log.ErrorLog.Printf("Failed to save instances: %v", err)
		}
		// Update displays
		return m, m.instanceChanged()
	case rebaseUpdateMsg:
		// Handle rebase progress updates
		if msg.err != nil {
			m.rebaseInProgress = false
			m.rebaseInstance = nil
			m.rebaseBranchName = ""
			m.rebaseOriginalSHA = ""
			return m, m.handleError(msg.err)
		}
		
		if msg.message != "" {
			// Show progress message
			m.errBox.SetSuccess(msg.message)
		}
		
		if msg.complete {
			m.rebaseInProgress = false
			m.rebaseInstance = nil
			m.rebaseBranchName = ""
			m.rebaseOriginalSHA = ""
			m.errBox.SetSuccess("Rebase completed successfully!")
			
			// Clear success message after a delay
			return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
				return clearSuccessMsg{}
			})
		}
		
		return m, nil
	}

	// Update spinner
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)

	// Update the list (which contains instances)
	m.list, cmd = m.list.Update(msg)
	cmds = append(cmds, cmd)

	// Update the tabbed window
	m.tabbedWindow, cmd = m.tabbedWindow.Update(msg)
	cmds = append(cmds, cmd)

	// Update overlays based on state
	switch m.state {
	case stateNew, statePrompt, stateBookmark:
		if m.textInputOverlay != nil {
			var overlayCmd tea.Cmd
			m.textInputOverlay, overlayCmd = m.textInputOverlay.Update(msg)
			cmds = append(cmds, overlayCmd)
		}
	case stateHelp:
		if m.textOverlay != nil {
			m.textOverlay.Update(msg)
		}
	case stateErrorLog:
		if m.textOverlay != nil {
			m.textOverlay.Update(msg)
		}
	case stateHistory:
		if m.historyOverlay != nil {
			m.historyOverlay.Update(msg)
		}
	case stateKeybindingEditor:
		if m.keybindingEditorOverlay != nil {
			updated, overlayCmd := m.keybindingEditorOverlay.Update(msg)
			if u, ok := updated.(*overlay.KeybindingEditorOverlay); ok {
				m.keybindingEditorOverlay = u
			}
			cmds = append(cmds, overlayCmd)
		}
	case stateGitStatus:
		if m.gitStatusOverlay != nil {
			m.gitStatusOverlay.Update(msg)
		}
	}

	return m, tea.Batch(cmds...)
}

// View renders the UI.
func (m *home) View() string {
	var sections []string

	// Add the list
	sections = append(sections, m.list.View())

	// Add the tabbed window
	sections = append(sections, m.tabbedWindow.View())

	// Add error box if there's an error
	if m.errBox.HasError() {
		sections = append(sections, m.errBox.View())
	}

	// Add the menu
	sections = append(sections, m.menu.View())

	// Render overlays based on state
	content := strings.Join(sections, "\n")

	switch m.state {
	case stateNew, statePrompt, stateBookmark:
		if m.textInputOverlay != nil {
			return m.textInputOverlay.View(content)
		}
	case stateHelp, stateErrorLog:
		if m.textOverlay != nil {
			return m.textOverlay.View(content)
		}
	case stateConfirm:
		if m.confirmationOverlay != nil {
			return m.confirmationOverlay.View(content)
		}
	case stateBranchSelect:
		if m.branchSelectorOverlay != nil {
			return m.branchSelectorOverlay.View()
		}
	case statePRReview:
		if m.prReviewOverlay != nil {
			return m.prReviewOverlay.View()
		}
	case stateHistory:
		if m.historyOverlay != nil {
			return m.historyOverlay.View(content)
		}
	case stateCommentDetail:
		if m.commentDetailOverlay != nil {
			return m.commentDetailOverlay.View(content)
		}
	case stateKeybindingEditor:
		if m.keybindingEditorOverlay != nil {
			return m.keybindingEditorOverlay.View()
		}
	case stateGitStatus:
		if m.gitStatusOverlay != nil {
			return m.gitStatusOverlay.View(content)
		}
	}

	return content
}

// -- Handlers --

// handleMenuHighlighting handles menu highlighting based on key presses
func (m *home) handleMenuHighlighting(msg tea.KeyMsg) (tea.Cmd, bool) {
	// Reset the keySent flag on any key press
	m.keySent = false

	// Convert tea.KeyMsg to our KeyName
	name, ok := keys.GetKeyName(msg.String())
	if !ok {
		// Key is not mapped, don't highlight anything
		return nil, false
	}

	// Skip the menu highlighting if we're using scrolling keys
	if name == keys.KeyShiftDown || name == keys.KeyShiftUp {
		return nil, false
	}

	// Skip the menu highlighting if the key is not in the map or we are using the shift up and down keys.
	// TODO: cleanup: when you press enter on stateNew, we use keys.KeySubmitName. We should unify the keymap.
	if name == keys.KeyEnter && m.state == stateNew {
		name = keys.KeySubmitName
	}

	m.keySent = true
	return tea.Batch(
		func() tea.Msg { return msg },
		m.keydownCallback(name)), true
}

func (m *home) handleKeyPress(msg tea.KeyMsg) (mod tea.Model, cmd tea.Cmd) {
	cmd, returnEarly := m.handleMenuHighlighting(msg)
	if returnEarly {
		return m, cmd
	}

	if m.state == stateHelp {
		return m.handleHelpState(msg)
	}

	if m.state == stateErrorLog {
		return m.handleErrorLogState(msg)
	}

	if m.state == stateHistory {
		return m.handleHistoryState(msg)
	}

	if m.state == stateKeybindingEditor {
		return m.handleKeybindingEditorState(msg)
	}

	if m.state == stateGitStatus {
		return m.handleGitStatusState(msg)
	}

	if m.state == stateCommentDetail {
		return m.handleCommentDetailState(msg)
	}

	if m.state == stateNew {
		return m.handleNewState(msg)
	}

	if m.state == statePrompt {
		return m.handlePromptState(msg)
	}

	if m.state == stateBookmark {
		return m.handleBookmarkState(msg)
	}

	if m.state == stateBranchSelect {
		return m.handleBranchSelectState(msg)
	}

	if m.state == statePRReview {
		return m.handlePRReviewState(msg)
	}

	// Convert tea.KeyMsg to our KeyName
	keyName, ok := keys.GetKeyName(msg.String())
	if !ok {
		return m, nil
	}

	// Check if we should handle keys in diff view specially
	if m.tabbedWindow.IsInDiffTab() && m.scrollLocked {
		// When scroll lock is on, up/down keys scroll without shift
		switch keyName {
		case keys.KeyUp:
			keyName = keys.KeyShiftUp
		case keys.KeyDown:
			keyName = keys.KeyShiftDown
		}
	}

	switch keyName {
	case keys.KeyShiftUp, keys.KeyShiftDown, keys.KeyHome, keys.KeyEnd,
		keys.KeyPageUp, keys.KeyPageDown, keys.KeyAltUp, keys.KeyAltDown,
		keys.KeyDiffAll, keys.KeyDiffLastCommit, keys.KeyLeft, keys.KeyRight:
		// These keys are only for diff navigation
		if m.tabbedWindow.IsInDiffTab() {
			return m, m.handleDiffNavigation(keyName)
		}
		return m, nil
	case keys.KeyScrollLock:
		// Toggle scroll lock
		m.scrollLocked = !m.scrollLocked
		statusMsg := "Scroll lock OFF"
		if m.scrollLocked {
			statusMsg = "Scroll lock ON"
		}
		return m, m.handleError(fmt.Errorf(statusMsg))
	case keys.KeyOpenInIDE:
		// Open file in IDE (only in diff view)
		if m.tabbedWindow.IsInDiffTab() {
			selected := m.list.GetSelectedInstance()
			if selected != nil {
				filePath := m.tabbedWindow.GetCurrentFile()
				if filePath != "" {
					return m, m.openFileInIDE(selected, filePath)
				}
			}
		}
		return m, nil
	case keys.KeyExternalDiff:
		// Open file in external diff tool (only in diff view)
		if m.tabbedWindow.IsInDiffTab() {
			selected := m.list.GetSelectedInstance()
			if selected != nil {
				filePath := m.tabbedWindow.GetCurrentFile()
				if filePath != "" {
					return m, m.openFileInExternalDiff(selected, filePath)
				}
			}
		}
		return m, nil
	}

	// Handle navigation keys that work in both views
	switch keyName {
	case keys.KeyUp:
		if m.list.NumInstances() == 0 {
			return m, nil
		}
		m.list.Previous()
		return m, m.instanceChanged()
	case keys.KeyDown:
		if m.list.NumInstances() == 0 {
			return m, nil
		}
		m.list.Next()
		return m, m.instanceChanged()
	case keys.KeyTab:
		return m.handleTabSwitch(false)
	case keys.KeyShiftTab:
		return m.handleTabSwitch(true)
	case keys.KeyQuit:
		return m, tea.Quit
	case keys.KeyHelp:
		// Show help screen
		m.showHelpScreen(helpTypeGeneral{}, nil)
		return m, nil
	case keys.KeyEditKeybindings:
		// Show keybinding editor
		m.state = stateKeybindingEditor
		m.keybindingEditorOverlay = overlay.NewKeybindingEditorOverlay()
		m.keybindingEditorOverlay.OnSave = func(config *keys.KeyBindingsConfig) {
			// Save the keybindings
			if err := config.Save(); err != nil {
				m.handleError(fmt.Errorf("failed to save keybindings: %w", err))
			} else {
				// Reinitialize keybindings
				if err := keys.InitializeCustomKeyBindings(); err != nil {
					m.handleError(fmt.Errorf("failed to reload keybindings: %w", err))
				} else {
					m.handleError(fmt.Errorf("keybindings saved successfully"))
				}
			}
			m.state = stateDefault
			m.keybindingEditorOverlay = nil
		}
		m.keybindingEditorOverlay.OnCancel = func() {
			m.state = stateDefault
			m.keybindingEditorOverlay = nil
		}
		return m, nil
	case keys.KeyErrorLog:
		// Show error log
		m.showErrorLog()
		return m, nil
	case keys.KeyNew:
		// Show help screen before creating new instance
		m.showHelpScreen(helpTypeInstanceCreation{}, func() {
			m.startNewInstance(false)
		})
		return m, nil
	case keys.KeyPrompt:
		// Show help screen before creating new instance with prompt
		m.showHelpScreen(helpTypeInstanceCreation{}, func() {
			m.startNewInstance(true)
		})
		return m, nil
	case keys.KeyExistingBranch:
		// Show branch selector
		return m, m.showBranchSelector()
	case keys.KeyKill:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		// Create the kill action
		killAction := func() tea.Cmd {
			return func() tea.Msg {
				if err := m.storage.DeleteInstance(selected); err != nil {
					return err
				}
				if err := selected.Kill(); err != nil {
					return err
				}
				m.list.RemoveSelectedInstance()
				return nil
			}
		}
		// Show confirmation modal
		message := fmt.Sprintf("[!] Kill session '%s'?", selected.Title)
		return m, m.confirmAction(message, killAction)
	case keys.KeySubmit:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		// Create the push action as a tea.Cmd
		pushAction := func() tea.Msg {
			// Default commit message with timestamp
			commitMsg := fmt.Sprintf("[claudesquad] update from '%s' on %s", selected.Title, time.Now().Format(time.RFC822))
			worktree, err := selected.GetGitWorktree()
			if err != nil {
				return err
			}
			if err = worktree.PushChanges(commitMsg, true); err != nil {
				return err
			}
			return nil
		}
		// Show confirmation modal
		message := fmt.Sprintf("[!] Push changes from session '%s'?", selected.Title)
		return m, m.confirmAction(message, pushAction)
	case keys.KeyCheckout:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		// Show help screen before pausing
		m.showHelpScreen(helpTypeInstanceCheckout{}, func() {
			if err := selected.Pause(); err != nil {
				m.handleError(err)
			}
			m.instanceChanged()
		})
		return m, nil
	case keys.KeyResume:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		if err := selected.Resume(); err != nil {
			return m, m.handleError(err)
		}
		return m, m.instanceChanged()
	case keys.KeyOpenIDE:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		// Open IDE at the instance's path and connect Claude
		cmd := m.openIDE(selected)
		return m, cmd
	case keys.KeyRebase:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		
		// Check if instance is paused
		if selected.Paused() {
			return m, m.handleError(fmt.Errorf(instancePausedError, selected.Title))
		}
		
		// Get the worktree
		worktree, err := selected.GetGitWorktree()
		if err != nil {
			return m, m.handleError(fmt.Errorf("failed to get git worktree: %w", err))
		}
		
		// Get main branch name
		mainBranch, err := worktree.GetMainBranch()
		if err != nil {
			return m, m.handleError(fmt.Errorf("failed to determine main branch: %w", err))
		}
		
		// Show confirmation modal
		message := fmt.Sprintf("[!] Rebase session '%s' with %s?", selected.Title, mainBranch)
		
		// Store the selected instance for the rebase
		m.pendingRebaseInstance = selected
		
		// Create a simple action that just returns a message to trigger the actual rebase
		rebaseAction := func() tea.Cmd {
			return func() tea.Msg {
				return startRebaseMsg{}
			}
		}
		
		return m, m.confirmAction(message, rebaseAction)
	case keys.KeyPRReview:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Check if instance is started
		if !selected.Started() {
			return m, m.handleError(fmt.Errorf("instance '%s' is not started", selected.Title))
		}

		// Check if instance is paused
		if selected.Paused() {
			return m, m.handleError(fmt.Errorf(instancePausedError, selected.Title))
		}

		// Get the worktree for the selected instance
		worktree, err := selected.GetGitWorktree()
		if err != nil {
			return m, m.handleError(fmt.Errorf("failed to get git worktree: %w", err))
		}

		// Get the worktree path
		worktreePath := worktree.GetWorktreePath()

		// Get current PR info from the worktree (always fresh)
		pr, err := git.GetCurrentPR(worktreePath)
		if err != nil {
			return m, m.handleError(fmt.Errorf(noPullRequestFoundError, err))
		}

		// Fetch PR comments (always fresh - includes resolved status detection)
		if err := pr.FetchComments(worktreePath); err != nil {
			return m, m.handleError(fmt.Errorf("failed to fetch PR comments: %w", err))
		}

		// Preprocess comments for better performance
		pr.PreprocessComments()

		// Show PR review UI
		m.state = statePRReview
		prReviewModel := ui.NewPRReviewModel(pr)
		m.prReviewOverlay = &prReviewModel

		// Initialize the PR review model
		initCmd := prReviewModel.Init()
		return m, initCmd
	case keys.KeyBookmark:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		// Show the bookmark creation state
		m.state = stateBookmark
		m.menu.SetState(ui.StateBookmark)
		m.textInputOverlay = overlay.NewTextInputOverlay("Enter bookmark message (or leave empty for auto-generated)", "")
		return m, nil
	case keys.KeyHistory:
		return m, m.showHistoryView()
	case keys.KeyTest:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		// Run Jest tests in the web directory
		cmd := m.runJestTests(selected)
		return m, cmd
	case keys.KeyGitStatus:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		// Show git status overlay
		return m, m.showGitStatusOverlay(selected)
	case keys.KeyGitStatusBookmark:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		// Show git status overlay in bookmark mode
		return m, m.showGitStatusOverlayBookmarkMode(selected)
	case keys.KeyCheckUpdate:
		// Trigger an immediate update check
		m.updateChecker.CheckNow()
		// For now, we'll just return without showing a message
		// The update indicator will appear in the menu when the check completes
		return m, nil
	case keys.KeyReset:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		// Check if instance is paused
		if selected.Paused() {
			return m, m.handleError(fmt.Errorf(instancePausedError, selected.Title))
		}
		// Get the worktree
		worktree, err := selected.GetGitWorktree()
		if err != nil {
			return m, m.handleError(fmt.Errorf("failed to get git worktree: %w", err))
		}
		// Get current branch name
		branchName, err := worktree.GetCurrentBranch()
		if err != nil {
			return m, m.handleError(fmt.Errorf("failed to get current branch: %w", err))
		}
		// Show confirmation modal
		message := fmt.Sprintf("[!] Reset branch '%s' to remote head? This will discard all local commits.", branchName)
		resetAction := func() tea.Cmd {
			return m.resetBranchToRemote(selected)
		}
		return m, m.confirmAction(message, resetAction)
	case keys.KeyEnter:
		if m.list.NumInstances() == 0 {
			return m, nil
		}
		selected := m.list.GetSelectedInstance()
		if selected == nil || selected.Paused() || !selected.TmuxAlive() {
			return m, nil
		}
		// Show help screen before attaching
		m.showHelpScreen(helpTypeInstanceAttach{}, func() {
			var ch chan struct{}
			var err error

			// Determine which pane to attach to based on active tab
			if m.tabbedWindow.IsInTerminalTab() {
				// If terminal tab is active, attach to terminal pane (pane 0)
				ch, err = m.list.AttachToPane(0)
			} else {
				// Otherwise, attach to AI pane (pane 1)
				ch, err = m.list.AttachToPane(1)
			}

			if err != nil {
				m.handleError(err)
				return
			}

			// Store selected instance for reload handling
			selected := m.list.GetSelectedInstance()

			<-ch
			m.state = stateDefault

			// Check if reload was requested (set by the tmux reload handler)
			if selected != nil && selected.NeedsReload() {
				selected.SetNeedsReload(false)
				// Reload the session
				if err := selected.ReloadSession(); err != nil {
					m.handleError(err)
					return
				}
				// Show a message that reload completed
				fmt.Fprintf(os.Stderr, "\n\033[32mSession reloaded. Press Enter to re-attach.\033[0m\n")
			}
		})
		return m, nil
	default:
		return m, nil
	}
}

// handleTabSwitch handles tab switching in both forward and reverse directions
func (m *home) handleTabSwitch(reverse bool) (tea.Model, tea.Cmd) {
	if reverse {
		m.tabbedWindow.ToggleReverse()
	} else {
		m.tabbedWindow.Toggle()
	}
	m.menu.SetInDiffTab(m.tabbedWindow.IsInDiffTab())
	return m, m.instanceChanged()
}

// instanceChanged updates the AI pane, menu, diff pane, and terminal pane based on the selected instance. It returns an error
// Cmd if there was any error.
func (m *home) openIDE(instance *session.Instance) tea.Cmd {
	return func() tea.Msg {
		// Get the git worktree to access the worktree path
		gitWorktree, err := instance.GetGitWorktree()
		if err != nil {
			return fmt.Errorf("failed to get git worktree: %w", err)
		}

		// Open IDE at the worktree path (not the git root)
		worktreePath := gitWorktree.GetWorktreePath()

		// Get the IDE command from configuration
		globalConfig := m.appConfig
		ideCommand := config.GetEffectiveIdeCommand(worktreePath, globalConfig)

		cmd := exec.Command(ideCommand, worktreePath)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to open IDE (%s): %w", ideCommand, err)
		}

		return nil
	}
}

func (m *home) openFileInIDE(instance *session.Instance, filePath string) tea.Cmd {
	return func() tea.Msg {
		// Get the git worktree to access the worktree path
		gitWorktree, err := instance.GetGitWorktree()
		if err != nil {
			return fmt.Errorf("failed to get git worktree: %w", err)
		}

		// Construct the full path to the file using the worktree path
		worktreePath := gitWorktree.GetWorktreePath()
		fullPath := filepath.Join(worktreePath, filePath)

		// Get the IDE command from configuration
		globalConfig := m.appConfig
		ideCommand := config.GetEffectiveIdeCommand(worktreePath, globalConfig)

		// Open IDE with the specific file
		cmd := exec.Command(ideCommand, fullPath)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to open file in IDE (%s): %w", ideCommand, err)
		}

		return nil
	}
}

func (m *home) openFileInExternalDiff(instance *session.Instance, filePath string) tea.Cmd {
	return func() tea.Msg {
		// Get the git worktree to access the worktree path
		gitWorktree, err := instance.GetGitWorktree()
		if err != nil {
			return fmt.Errorf("failed to get git worktree: %w", err)
		}

		// Get the diff command from configuration
		worktreePath := gitWorktree.GetWorktreePath()
		globalConfig := m.appConfig
		diffCommand := config.GetEffectiveDiffCommand(worktreePath, globalConfig)

		if diffCommand == "" {
			return fmt.Errorf("no diff command configured")
		}

		// Split the diff command to handle cases like "code --diff"
		parts := strings.Fields(diffCommand)
		if len(parts) == 0 {
			return fmt.Errorf("invalid diff command")
		}

		// Get the file at HEAD for comparison
		fullPath := filepath.Join(worktreePath, filePath)
		
		// Create a temporary file for the HEAD version
		headContent, err := gitWorktree.GetFileAtHead(filePath)
		if err != nil {
			// If file doesn't exist at HEAD, just open the current file
			cmd := exec.Command(parts[0], append(parts[1:], fullPath)...)
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to open file in diff tool: %w", err)
			}
			return nil
		}

		// Write HEAD content to temp file
		tempFile, err := os.CreateTemp("", "claude-squad-*."+filepath.Base(filePath))
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		defer tempFile.Close()

		if _, err := tempFile.Write([]byte(headContent)); err != nil {
			return fmt.Errorf("failed to write temp file: %w", err)
		}

		// Build command arguments
		args := append(parts[1:], tempFile.Name(), fullPath)
		
		// Open diff tool with both files
		cmd := exec.Command(parts[0], args...)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to open external diff tool: %w", err)
		}

		// Clean up temp file after a delay (give the tool time to open)
		go func() {
			time.Sleep(5 * time.Second)
			os.Remove(tempFile.Name())
		}()

		return nil
	}
}

func (m *home) openFileInIDEAtLine(instance *session.Instance, filePath string, lineNumber int) tea.Cmd {
	return func() tea.Msg {
		// Get the git worktree to access the worktree path
		gitWorktree, err := instance.GetGitWorktree()
		if err != nil {
			return fmt.Errorf("failed to get git worktree: %w", err)
		}

		// Construct the full path to the file using the worktree path
		worktreePath := gitWorktree.GetWorktreePath()
		fullPath := filepath.Join(worktreePath, filePath)

		// Get the IDE command from configuration
		globalConfig := m.appConfig
		ideCommand := config.GetEffectiveIdeCommand(worktreePath, globalConfig)

		// Different IDEs have different ways to open at a specific line
		// VS Code: code file:line
		// WebStorm/IntelliJ: idea file:line
		// Sublime: subl file:line
		// Vim: vim +line file
		
		var cmd *exec.Cmd
		
		// Check which IDE we're using
		if strings.Contains(ideCommand, "vim") || strings.Contains(ideCommand, "nvim") {
			// Vim-style: +line file
			cmd = exec.Command(ideCommand, fmt.Sprintf("+%d", lineNumber), fullPath)
		} else {
			// Most modern IDEs support file:line format
			cmd = exec.Command(ideCommand, fmt.Sprintf("%s:%d", fullPath, lineNumber))
		}

		if err := cmd.Start(); err != nil {
			// Fallback to just opening the file without line number
			cmd = exec.Command(ideCommand, fullPath)
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to open file in IDE (%s): %w", ideCommand, err)
			}
		}

		return nil
	}
}

// runJestTests runs Jest tests for the selected instance
func (m *home) runJestTests(instance *session.Instance) tea.Cmd {
	return func() tea.Msg {
		// Get the git worktree to access the worktree path
		gitWorktree, err := instance.GetGitWorktree()
		if err != nil {
			return fmt.Errorf("failed to get git worktree: %w", err)
		}

		// Get the worktree path
		worktreePath := gitWorktree.GetWorktreePath()

		// Check if web directory exists
		webDir := filepath.Join(worktreePath, "web")
		if _, err := os.Stat(webDir); os.IsNotExist(err) {
			return fmt.Errorf("no 'web' directory found in worktree")
		}

		// Clear previous Jest results
		m.tabbedWindow.ClearJestResults()

		// Run npm test in the web directory
		cmd := exec.Command("npm", "test", "--", "--json", "--outputFile=jest-results.json")
		cmd.Dir = webDir

		// Capture output
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Jest returns non-zero exit code when tests fail, which is expected
			// Check if we at least got the results file
			resultsPath := filepath.Join(webDir, "jest-results.json")
			if _, statErr := os.Stat(resultsPath); statErr != nil {
				// No results file, this is a real error
				return fmt.Errorf("jest command failed: %w\nOutput: %s", err, string(output))
			}
		}

		// Read the Jest results
		resultsPath := filepath.Join(webDir, "jest-results.json")
		resultsData, err := os.ReadFile(resultsPath)
		if err != nil {
			return fmt.Errorf("failed to read jest results: %w", err)
		}

		// Update the Jest pane with results
		if err := m.tabbedWindow.UpdateJestResults(resultsData); err != nil {
			return fmt.Errorf("failed to parse jest results: %w", err)
		}

		// Switch to Jest tab
		m.tabbedWindow.ActivateJestTab()
		m.menu.SetInDiffTab(false)

		// Clean up the results file
		os.Remove(resultsPath)

		return "Jest tests completed"
	}
}

// handleDiffNavigation handles navigation keys specific to the diff view
func (m *home) handleDiffNavigation(keyName keys.KeyName) tea.Cmd {
	switch keyName {
	case keys.KeyShiftUp:
		m.tabbedWindow.ScrollUp()
	case keys.KeyShiftDown:
		m.tabbedWindow.ScrollDown()
	case keys.KeyHome:
		m.tabbedWindow.ScrollToTop()
	case keys.KeyEnd:
		m.tabbedWindow.ScrollToBottom()
	case keys.KeyPageUp:
		m.tabbedWindow.PageUp()
	case keys.KeyPageDown:
		m.tabbedWindow.PageDown()
	case keys.KeyAltUp:
		m.tabbedWindow.PreviousFile()
	case keys.KeyAltDown:
		m.tabbedWindow.NextFile()
	case keys.KeyDiffAll:
		return m.showAllChanges()
	case keys.KeyDiffLastCommit:
		return m.showLastCommitDiff()
	case keys.KeyLeft:
		m.tabbedWindow.PreviousCommit()
		return m.updateDiffForCommit()
	case keys.KeyRight:
		m.tabbedWindow.NextCommit()
		return m.updateDiffForCommit()
	}
	return nil
}

// showAllChanges updates the diff pane to show all changes
func (m *home) showAllChanges() tea.Cmd {
	selected := m.list.GetSelectedInstance()
	if selected == nil {
		return nil
	}

	return func() tea.Msg {
		worktree, err := selected.GetGitWorktree()
		if err != nil {
			return err
		}

		diff, err := worktree.GetDiff("")
		if err != nil {
			return err
		}

		m.tabbedWindow.SetDiffContent(diff, "All Changes")
		m.tabbedWindow.SetDiffMode(ui.DiffModeAll)
		return nil
	}
}

// showLastCommitDiff updates the diff pane to show the last commit
func (m *home) showLastCommitDiff() tea.Cmd {
	selected := m.list.GetSelectedInstance()
	if selected == nil {
		return nil
	}

	return func() tea.Msg {
		worktree, err := selected.GetGitWorktree()
		if err != nil {
			return err
		}

		// Get the last commit SHA
		lastCommit, err := worktree.GetLastCommitSHA()
		if err != nil {
			return err
		}

		// Get the diff for the last commit
		diff, err := worktree.GetCommitDiff(lastCommit)
		if err != nil {
			return err
		}

		// Get commit info
		info, err := worktree.GetCommitInfo(lastCommit)
		if err != nil {
			return err
		}

		title := fmt.Sprintf("Commit: %s - %s", lastCommit[:7], info.Subject)
		m.tabbedWindow.SetDiffContent(diff, title)
		m.tabbedWindow.SetDiffMode(ui.DiffModeCommit)
		
		// Load commit history
		history, err := worktree.GetCommitHistory(20)
		if err != nil {
			return err
		}
		m.tabbedWindow.SetCommitHistory(history, 0)
		
		return nil
	}
}

// updateDiffForCommit updates the diff view for the currently selected commit
func (m *home) updateDiffForCommit() tea.Cmd {
	selected := m.list.GetSelectedInstance()
	if selected == nil {
		return nil
	}

	commitIndex := m.tabbedWindow.GetCurrentCommitIndex()
	if commitIndex < 0 {
		return nil
	}

	return func() tea.Msg {
		worktree, err := selected.GetGitWorktree()
		if err != nil {
			return err
		}

		// Get commit history
		history, err := worktree.GetCommitHistory(20)
		if err != nil {
			return err
		}

		if commitIndex >= len(history) {
			return nil
		}

		commit := history[commitIndex]
		
		// Get the diff for this commit
		diff, err := worktree.GetCommitDiff(commit.SHA)
		if err != nil {
			return err
		}

		title := fmt.Sprintf("Commit: %s - %s", commit.SHA[:7], commit.Subject)
		m.tabbedWindow.SetDiffContent(diff, title)
		
		return nil
	}
}

// handleNewState handles key presses when in the new instance state
func (m *home) handleNewState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyName, ok := keys.GetKeyName(msg.String())
	if !ok {
		// Pass through to text input
		updated, cmd := m.textInputOverlay.Update(msg)
		m.textInputOverlay = updated.(*overlay.TextInputOverlay)
		return m, cmd
	}

	switch keyName {
	case keys.KeySubmitName:
		value := m.textInputOverlay.Value()
		if value == "" {
			return m, nil
		}
		
		// Check if we should go to prompt mode after naming
		if m.promptAfterName {
			// Reset the flag
			m.promptAfterName = false
			// Store the name and switch to prompt mode
			m.textInputOverlay = overlay.NewTextInputOverlay("Enter a prompt for the AI (or press Escape to skip)", "")
			m.state = statePrompt
			m.menu.SetState(ui.StatePrompt)
			// Store the instance name for later
			m.textInputOverlay.SetMetadata("instanceName", value)
			return m, nil
		}
		
		// Normal flow - create instance immediately
		m.state = stateDefault
		m.menu.SetState(ui.StateDefault)
		m.textInputOverlay = nil
		return m, m.createInstance(value, "")
	case keys.KeyQuit:
		m.state = stateDefault
		m.menu.SetState(ui.StateDefault)
		m.textInputOverlay = nil
		return m, nil
	default:
		// Pass through to text input
		updated, cmd := m.textInputOverlay.Update(msg)
		m.textInputOverlay = updated.(*overlay.TextInputOverlay)
		return m, cmd
	}
}

// handlePromptState handles key presses when in the prompt state
func (m *home) handlePromptState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyName, ok := keys.GetKeyName(msg.String())
	if !ok {
		// Pass through to text input
		updated, cmd := m.textInputOverlay.Update(msg)
		m.textInputOverlay = updated.(*overlay.TextInputOverlay)
		return m, cmd
	}

	switch keyName {
	case keys.KeyEnter:
		prompt := m.textInputOverlay.Value()
		instanceName := m.textInputOverlay.GetMetadata("instanceName")
		
		m.state = stateDefault
		m.menu.SetState(ui.StateDefault)
		m.textInputOverlay = nil
		
		// Create instance with prompt (even if empty)
		return m, m.createInstance(instanceName, prompt)
	case keys.KeyQuit:
		// If escape is pressed, create instance without prompt
		instanceName := m.textInputOverlay.GetMetadata("instanceName")
		
		m.state = stateDefault
		m.menu.SetState(ui.StateDefault)
		m.textInputOverlay = nil
		
		// Create instance without prompt
		return m, m.createInstance(instanceName, "")
	default:
		// Pass through to text input
		updated, cmd := m.textInputOverlay.Update(msg)
		m.textInputOverlay = updated.(*overlay.TextInputOverlay)
		return m, cmd
	}
}

// handleBookmarkState handles key presses when in the bookmark state
func (m *home) handleBookmarkState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyName, ok := keys.GetKeyName(msg.String())
	if !ok {
		// Pass through to text input
		updated, cmd := m.textInputOverlay.Update(msg)
		m.textInputOverlay = updated.(*overlay.TextInputOverlay)
		return m, cmd
	}

	switch keyName {
	case keys.KeyEnter:
		message := m.textInputOverlay.Value()
		m.state = stateDefault
		m.menu.SetState(ui.StateDefault)
		m.textInputOverlay = nil
		
		// Create bookmark commit
		selected := m.list.GetSelectedInstance()
		if selected != nil {
			return m, m.createBookmark(selected, message)
		}
		return m, nil
	case keys.KeyQuit:
		m.state = stateDefault
		m.menu.SetState(ui.StateDefault)
		m.textInputOverlay = nil
		return m, nil
	default:
		// Pass through to text input
		updated, cmd := m.textInputOverlay.Update(msg)
		m.textInputOverlay = updated.(*overlay.TextInputOverlay)
		return m, cmd
	}
}

// handleBranchSelectState handles key presses when in the branch select state
func (m *home) handleBranchSelectState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyName, ok := keys.GetKeyName(msg.String())
	if ok && keyName == keys.KeyQuit {
		m.state = stateDefault
		m.branchSelectorOverlay = nil
		return m, nil
	}

	// Pass through to branch selector
	updated, cmd := m.branchSelectorOverlay.Update(msg)
	if u, ok := updated.(*overlay.BranchSelectorOverlay); ok {
		m.branchSelectorOverlay = u
	}
	return m, cmd
}

// handlePRReviewState handles key presses when in the PR review state
func (m *home) handlePRReviewState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyName, ok := keys.GetKeyName(msg.String())
	if ok && keyName == keys.KeyQuit {
		m.state = stateDefault
		m.prReviewOverlay = nil
		return m, nil
	}

	// Pass through to PR review overlay
	updatedModel, cmd := m.prReviewOverlay.Update(msg)
	if u, ok := updatedModel.(*ui.PRReviewModel); ok {
		m.prReviewOverlay = u
	}
	return m, cmd
}

// handleHelpState handles key presses when in the help state
func (m *home) handleHelpState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyName, _ := keys.GetKeyName(msg.String())
	if keyName == keys.KeyQuit || keyName == keys.KeyHelp || keyName == keys.KeyEnter {
		m.state = stateDefault
		m.textOverlay = nil
		// Execute any pending callback
		if callback := m.textOverlay.GetCallback(); callback != nil {
			callback()
		}
		return m, nil
	}
	return m, nil
}

// handleErrorLogState handles key presses when in the error log state
func (m *home) handleErrorLogState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyName, _ := keys.GetKeyName(msg.String())
	if keyName == keys.KeyQuit || keyName == keys.KeyErrorLog || keyName == keys.KeyEnter {
		m.state = stateDefault
		m.textOverlay = nil
		return m, nil
	}
	// Allow scrolling in error log
	if m.textOverlay != nil {
		m.textOverlay.Update(msg)
	}
	return m, nil
}

// handleHistoryState handles key presses when in the history state
func (m *home) handleHistoryState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyName, _ := keys.GetKeyName(msg.String())
	if keyName == keys.KeyQuit || keyName == keys.KeyHistory || keyName == keys.KeyEnter {
		m.state = stateDefault
		m.menu.SetState(ui.StateDefault)
		m.historyOverlay = nil
		return m, nil
	}
	// Pass through to history overlay for scrolling
	if m.historyOverlay != nil {
		m.historyOverlay.Update(msg)
	}
	return m, nil
}

// handleKeybindingEditorState handles key presses when in the keybinding editor state
func (m *home) handleKeybindingEditorState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// The keybinding editor handles all its own keys
	updated, cmd := m.keybindingEditorOverlay.Update(msg)
	if u, ok := updated.(*overlay.KeybindingEditorOverlay); ok {
		m.keybindingEditorOverlay = u
	}
	return m, cmd
}

// handleGitStatusState handles key presses when in the git status state
func (m *home) handleGitStatusState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyName, _ := keys.GetKeyName(msg.String())
	if keyName == keys.KeyQuit || keyName == keys.KeyGitStatus || keyName == keys.KeyEnter {
		m.state = stateDefault
		m.gitStatusOverlay = nil
		return m, nil
	}
	// Pass through to git status overlay for navigation
	if m.gitStatusOverlay != nil {
		m.gitStatusOverlay.Update(msg)
	}
	return m, nil
}

// handleCommentDetailState handles key presses when in the comment detail state
func (m *home) handleCommentDetailState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyName, _ := keys.GetKeyName(msg.String())
	if keyName == keys.KeyQuit || keyName == keys.KeyEnter {
		// Return to PR review state
		m.state = statePRReview
		m.commentDetailOverlay = nil
		return m, nil
	}
	// Pass through to comment detail overlay for scrolling
	if m.commentDetailOverlay != nil {
		m.commentDetailOverlay.Update(msg)
	}
	return m, nil
}

// startNewInstance starts the process of creating a new instance
func (m *home) startNewInstance(withPrompt bool) {
	if m.list.NumInstances() >= GlobalInstanceLimit {
		m.handleError(fmt.Errorf("session limit reached (%d)", GlobalInstanceLimit))
		return
	}
	
	m.state = stateNew
	m.menu.SetState(ui.StateNew)
	m.textInputOverlay = overlay.NewTextInputOverlay("Enter instance name", "")
	m.promptAfterName = withPrompt
}

// showBranchSelector shows the branch selector overlay
func (m *home) showBranchSelector() tea.Cmd {
	return func() tea.Msg {
		// Get the git root directory
		gitRoot, err := git.FindRepoRoot(".")
		if err != nil {
			return fmt.Errorf("not in a git repository")
		}

		// Get list of branches
		branches, err := git.ListBranches(gitRoot)
		if err != nil {
			return err
		}

		if len(branches) == 0 {
			return fmt.Errorf("no branches found")
		}

		// Get current branch to filter it out
		currentBranch, err := git.GetCurrentBranch(gitRoot)
		if err != nil {
			return err
		}

		// Filter out current branch
		var availableBranches []string
		for _, branch := range branches {
			if branch != currentBranch {
				availableBranches = append(availableBranches, branch)
			}
		}

		if len(availableBranches) == 0 {
			return fmt.Errorf("no other branches available")
		}

		// Show branch selector
		m.state = stateBranchSelect
		m.branchSelectorOverlay = overlay.NewBranchSelectorOverlay(availableBranches)
		return nil
	}
}

// createInstanceFromBranch creates a new instance from an existing branch
func (m *home) createInstanceFromBranch(branchName string) tea.Cmd {
	// Generate instance name from branch name
	instanceName := branchNameToInstanceName(branchName)
	
	// Create the instance with the branch
	return m.createInstanceWithBranch(instanceName, "", branchName)
}

// branchNameToInstanceName converts a branch name to a suitable instance name
func branchNameToInstanceName(branchName string) string {
	// Remove common prefixes
	name := branchName
	prefixes := []string{"feature/", "bugfix/", "hotfix/", "release/", "feat/", "fix/"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			name = strings.TrimPrefix(name, prefix)
			break
		}
	}
	
	// Replace special characters with underscores
	name = regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(name, "_")
	
	// Trim underscores from start and end
	name = strings.Trim(name, "_")
	
	// Limit length
	if len(name) > 30 {
		name = name[:30]
	}
	
	return name
}

// createInstance creates a new instance with the given name and optional prompt
func (m *home) createInstance(name string, prompt string) tea.Cmd {
	return m.createInstanceWithBranch(name, prompt, "")
}

// createInstanceWithBranch creates a new instance with the given name, prompt, and optional branch
func (m *home) createInstanceWithBranch(name string, prompt string, branchName string) tea.Cmd {
	return func() tea.Msg {
		// Find git root
		gitRoot, err := git.FindRepoRoot(".")
		if err != nil {
			return err
		}

		// Get the current branch name if not specified
		if branchName == "" {
			currentBranch, err := git.GetCurrentBranch(gitRoot)
			if err != nil {
				return err
			}
			branchName = currentBranch
		}

		// Generate a unique branch name for the instance
		timestamp := time.Now().Format("20060102_150405")
		instanceBranchName := fmt.Sprintf("%s/%s_%s", m.storage.BranchPrefix, name, timestamp)

		// Create the instance
		instance, err := session.CreateAndStartInstance(name, gitRoot, m.storage.BasePath, instanceBranchName, branchName, m.program)
		if err != nil {
			return err
		}

		// Save the instance
		if err := m.storage.SaveInstance(instance); err != nil {
			instance.Kill()
			return err
		}

		// Add to the list
		m.list.AddInstance(instance)

		// If a prompt was provided, send it to the AI pane
		if prompt != "" {
			// Wait a bit for the instance to fully start
			time.Sleep(500 * time.Millisecond)
			
			// Send the prompt to the AI pane
			if err := instance.SendToAIPane(prompt); err != nil {
				log.ErrorLog.Printf("Failed to send prompt to AI pane: %v", err)
			}
		}

		return instanceUpdatedMsg{instance: instance}
	}
}

// createBookmark creates a bookmark commit for the instance
func (m *home) createBookmark(instance *session.Instance, message string) tea.Cmd {
	return func() tea.Msg {
		worktree, err := instance.GetGitWorktree()
		if err != nil {
			return fmt.Errorf("failed to get git worktree: %w", err)
		}

		// Create bookmark commit
		if err := worktree.CreateBookmark(message); err != nil {
			return fmt.Errorf("failed to create bookmark: %w", err)
		}

		return "Bookmark created successfully"
	}
}

// showHelpScreen shows a help screen and optionally executes a callback when dismissed
func (m *home) showHelpScreen(helpType interface{}, callback func()) {
	// Check if this help screen has been seen before
	helpKey := getHelpKey(helpType)
	if m.appState.HasSeenHelp(helpKey) && !m.autoYes {
		// User has seen this help before, execute callback immediately
		if callback != nil {
			callback()
		}
		return
	}

	// Mark this help as seen
	m.appState.MarkHelpSeen(helpKey)
	config.SaveState(m.appState)

	// Show the help screen
	m.state = stateHelp
	helpContent := getHelpContent(helpType)
	m.textOverlay = overlay.NewTextOverlay("Help", helpContent, callback)
}

// showErrorLog shows the error log overlay
func (m *home) showErrorLog() {
	if len(m.errorLog) == 0 {
		m.handleError(fmt.Errorf("no errors logged"))
		return
	}

	// Format error log with timestamps
	var content strings.Builder
	content.WriteString("Error Log (newest first):\n\n")
	
	// Show errors in reverse order (newest first)
	for i := len(m.errorLog) - 1; i >= 0; i-- {
		content.WriteString(fmt.Sprintf("[%d] %s\n\n", len(m.errorLog)-i, m.errorLog[i]))
	}

	m.state = stateErrorLog
	m.textOverlay = overlay.NewTextOverlay("Error Log", content.String(), nil)
}

// showHistoryView shows the history overlay for the current pane
func (m *home) showHistoryView() tea.Cmd {
	selected := m.list.GetSelectedInstance()
	if selected == nil {
		return nil
	}

	// Check if instance is paused
	if selected.Paused() {
		return m.handleError(fmt.Errorf(instancePausedError, selected.Title))
	}

	var content string
	var err error
	var title string

	// Get history based on current tab
	if m.tabbedWindow.IsInTerminalTab() {
		// Get terminal history
		content, err = selected.GetTerminalFullHistory()
		if err != nil {
			return m.handleError(fmt.Errorf("failed to get terminal history: %v", err))
		}
		title = fmt.Sprintf("Terminal History - %s", selected.Title)
	} else {
		// Default to AI pane if we're in diff view
		content, err = selected.GetAIFullHistory()
		if err != nil {
			return m.handleError(fmt.Errorf("failed to get AI history: %v", err))
		}
		title = fmt.Sprintf("AI History - %s", selected.Title)
	}

	// Create the history overlay
	m.historyOverlay = overlay.NewHistoryOverlay(title, content)
	m.historyOverlay.OnDismiss = func() {
		m.state = stateDefault
		m.menu.SetState(ui.StateDefault)
		m.historyOverlay = nil
	}

	// Set state to history
	m.state = stateHistory
	m.menu.SetState(ui.StateDefault)

	return tea.WindowSize()
}

// showGitStatusOverlay displays the git status overlay for the current instance
func (m *home) showGitStatusOverlay(instance *session.Instance) tea.Cmd {
	// Get the git worktree for the instance
	worktree, err := instance.GetGitWorktree()
	if err != nil {
		return m.handleError(fmt.Errorf("failed to get git worktree: %w", err))
	}

	// Get changed files for the branch
	files, err := worktree.GetChangedFilesForBranch()
	if err != nil {
		return m.handleError(fmt.Errorf("failed to get changed files: %w", err))
	}

	// Get the current branch name
	branchName, err := worktree.GetCurrentBranch()
	if err != nil {
		return m.handleError(fmt.Errorf("failed to get current branch: %w", err))
	}

	// Create the git status overlay
	m.gitStatusOverlay = overlay.NewGitStatusOverlay(branchName, files)
	m.gitStatusOverlay.OnDismiss = func() {
		m.state = stateDefault
		m.gitStatusOverlay = nil
	}

	// Set state to git status
	m.state = stateGitStatus

	return tea.WindowSize()
}

// showGitStatusOverlayBookmarkMode displays the git status overlay in bookmark mode
func (m *home) showGitStatusOverlayBookmarkMode(instance *session.Instance) tea.Cmd {
	// Get the git worktree for the instance
	worktree, err := instance.GetGitWorktree()
	if err != nil {
		return m.handleError(fmt.Errorf("failed to get git worktree: %w", err))
	}

	// Get the current branch name
	branchName, err := worktree.GetCurrentBranch()
	if err != nil {
		return m.handleError(fmt.Errorf("failed to get current branch: %w", err))
	}

	// Create the git status overlay in bookmark mode
	gitStatusOverlay, err := overlay.NewGitStatusOverlayBookmarkMode(branchName, worktree)
	if err != nil {
		return m.handleError(fmt.Errorf("failed to create bookmark git status overlay: %w", err))
	}

	m.gitStatusOverlay = gitStatusOverlay
	m.gitStatusOverlay.OnDismiss = func() {
		m.state = stateDefault
		m.gitStatusOverlay = nil
	}

	// Set state to git status
	m.state = stateGitStatus

	return tea.WindowSize()
}

// resetBranchToRemote resets the current branch to its remote head
func (m *home) resetBranchToRemote(instance *session.Instance) tea.Cmd {
	return func() tea.Msg {
		// Get the git worktree
		worktree, err := instance.GetGitWorktree()
		if err != nil {
			return fmt.Errorf("failed to get git worktree: %w", err)
		}

		// Get current branch name
		branchName, err := worktree.GetCurrentBranch()
		if err != nil {
			return fmt.Errorf("failed to get current branch: %w", err)
		}

		// Get the remote name (usually "origin")
		remote, err := worktree.GetRemoteName()
		if err != nil {
			return fmt.Errorf("failed to get remote name: %w", err)
		}

		// Fetch latest from remote
		if err := worktree.FetchFromRemote(); err != nil {
			return fmt.Errorf("failed to fetch from remote: %w", err)
		}

		// Reset to remote head
		if err := worktree.ResetToRemote(remote, branchName); err != nil {
			return fmt.Errorf("failed to reset branch '%s' to remote: %w", branchName, err)
		}

		return fmt.Sprintf("Branch '%s' reset to %s/%s", branchName, remote, branchName)
	}
}

// handleError converts an error into a tea.Msg
func (m *home) handleError(err error) tea.Cmd {
	return func() tea.Msg {
		return err
	}
}

// instanceChanged updates the UI when the selected instance changes
func (m *home) instanceChanged() tea.Cmd {
	selected := m.list.GetSelectedInstance()
	if selected == nil {
		// Clear all panes
		m.tabbedWindow.ClearAll()
		return nil
	}

	// Update the diff pane
	return m.updateDiffPane(selected)
}

// updateDiffPane updates the diff pane for the selected instance
func (m *home) updateDiffPane(instance *session.Instance) tea.Cmd {
	return func() tea.Msg {
		worktree, err := instance.GetGitWorktree()
		if err != nil {
			return err
		}

		diff, err := worktree.GetDiff("")
		if err != nil {
			return err
		}

		m.tabbedWindow.SetDiffContent(diff, "All Changes")
		return nil
	}
}

// keydownCallback creates a callback for key press handling
func (m *home) keydownCallback(keyName keys.KeyName) tea.Cmd {
	return func() tea.Msg {
		// Handle menu item highlighting
		m.menu.SetActiveKey(keyName)
		return nil
	}
}

// startRebase initiates a rebase operation for the given instance
func (m *home) startRebase(instance *session.Instance) tea.Cmd {
	return func() tea.Msg {
		// Get the worktree
		worktree, err := instance.GetGitWorktree()
		if err != nil {
			return rebaseUpdateMsg{err: fmt.Errorf("failed to get git worktree: %w", err)}
		}

		// Get main branch name
		mainBranch, err := worktree.GetMainBranch()
		if err != nil {
			return rebaseUpdateMsg{err: fmt.Errorf("failed to determine main branch: %w", err)}
		}

		// Mark rebase as in progress
		m.rebaseInProgress = true
		m.rebaseInstance = instance
		
		// Get current branch name
		branchName, err := worktree.GetCurrentBranch()
		if err != nil {
			return rebaseUpdateMsg{err: fmt.Errorf("failed to get current branch: %w", err)}
		}
		m.rebaseBranchName = branchName

		// Get current SHA before rebase
		sha, err := worktree.GetCurrentCommitSHA()
		if err != nil {
			return rebaseUpdateMsg{err: fmt.Errorf("failed to get current commit SHA: %w", err)}
		}
		m.rebaseOriginalSHA = sha

		// Perform the rebase
		if err := worktree.RebaseWithMain(); err != nil {
			return rebaseUpdateMsg{err: err}
		}

		return rebaseUpdateMsg{complete: true}
	}
}