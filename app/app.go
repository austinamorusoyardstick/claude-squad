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
	"encoding/json"
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
	// keybindingEditorOverlay displays keybinding editor interface
	keybindingEditorOverlay *overlay.KeybindingEditorOverlay

	// errorLog stores all error messages for display
	errorLog []string
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

	h := &home{
		ctx:          ctx,
		spinner:      spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewTerminalPane()),
		errBox:       ui.NewErrBox(),
		storage:      storage,
		appConfig:    appConfig,
		program:      program,
		autoYes:      autoYes,
		state:        stateDefault,
		appState:     appState,
	}
	h.list = ui.NewList(&h.spinner, autoYes)

	// Load saved instances
	instances, err := storage.LoadInstances()
	if err != nil {
		fmt.Printf("Failed to load instances: %v\n", err)
		os.Exit(1)
	}

	// Add loaded instances to the list
	for _, instance := range instances {
		// Call the finalizer immediately.
		h.list.AddInstance(instance)()
		if autoYes {
			instance.AutoYes = true
		}
	}

	return h
}

// updateHandleWindowSizeEvent sets the sizes of the components.
// The components will try to render inside their bounds.
func (m *home) updateHandleWindowSizeEvent(msg tea.WindowSizeMsg) {
	// List takes 30% of width, preview takes 70%
	listWidth := int(float32(msg.Width) * 0.3)
	tabsWidth := msg.Width - listWidth

	// Menu takes 10% of height, list and window take 90%
	contentHeight := int(float32(msg.Height) * 0.9)
	menuHeight := msg.Height - contentHeight - 1     // minus 1 for error box
	m.errBox.SetSize(int(float32(msg.Width)*0.9), 1) // error box takes 1 row

	m.tabbedWindow.SetSize(tabsWidth, contentHeight)
	m.list.SetSize(listWidth, contentHeight)

	if m.textInputOverlay != nil {
		m.textInputOverlay.SetSize(int(float32(msg.Width)*0.6), int(float32(msg.Height)*0.4))
	}
	if m.textOverlay != nil {
		m.textOverlay.SetWidth(int(float32(msg.Width) * 0.6))
	}
	if m.historyOverlay != nil {
		m.historyOverlay.SetSize(int(float32(msg.Width)*0.9), int(float32(msg.Height)*0.9))
	}

	previewWidth, previewHeight := m.tabbedWindow.GetPreviewSize()
	if err := m.list.SetSessionPreviewSize(previewWidth, previewHeight); err != nil {
		log.ErrorLog.Print(err)
	}
	m.menu.SetSize(msg.Width, menuHeight)
}

func (m *home) Init() tea.Cmd {
	// Upon starting, we want to start the spinner. Whenever we get a spinner.TickMsg, we
	// update the spinner, which sends a new spinner.TickMsg. I think this lasts forever lol.
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			time.Sleep(100 * time.Millisecond)
			return previewTickMsg{}
		},
		tickUpdateMetadataCmd,
	)
}

func (m *home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle branch selector updates when in that state
	if m.state == stateBranchSelect && m.branchSelectorOverlay != nil {
		if _, ok := msg.(tea.KeyMsg); ok {
			// Update the branch selector
			_, cmd := m.branchSelectorOverlay.Update(msg)

			// Check if selection is complete
			if m.branchSelectorOverlay.IsSelected() {
				selectedBranch := m.branchSelectorOverlay.SelectedBranch()
				if selectedBranch == "" {
					// User cancelled
					m.state = stateDefault
					m.menu.SetState(ui.StateDefault)
					m.branchSelectorOverlay = nil
					return m, nil
				}

				// Create instance with selected branch
				return m.createInstanceWithBranch(selectedBranch)
			}

			return m, cmd
		}
	}

	// Handle PR review updates when in that state
	if m.state == statePRReview && m.prReviewOverlay != nil {
		// Always pass window size messages to ensure the overlay initializes
		if _, ok := msg.(tea.WindowSizeMsg); ok {
			updatedModel, cmd := m.prReviewOverlay.Update(msg)
			*m.prReviewOverlay = updatedModel
			return m, cmd
		}

		updatedModel, cmd := m.prReviewOverlay.Update(msg)
		*m.prReviewOverlay = updatedModel

		// Check for completion or cancellation messages
		switch msg.(type) {
		case ui.PRReviewCompleteMsg:
			// Handle accepted comments
			acceptedComments := msg.(ui.PRReviewCompleteMsg).AcceptedComments
			m.state = stateDefault
			m.prReviewOverlay = nil

			// Process accepted comments with Claude
			if len(acceptedComments) > 0 {
				return m, m.processAcceptedComments(acceptedComments)
			}
			return m, nil
		case ui.PRReviewCancelMsg:
			// User cancelled
			m.state = stateDefault
			m.prReviewOverlay = nil
			return m, nil
		}

		return m, cmd
	}

	switch msg := msg.(type) {
	case hideErrMsg:
		m.errBox.Clear()
	case previewTickMsg:
		cmd := m.instanceChanged()
		return m, tea.Batch(
			cmd,
			func() tea.Msg {
				time.Sleep(100 * time.Millisecond)
				return previewTickMsg{}
			},
		)
	case keyupMsg:
		m.menu.ClearKeydown()
		return m, nil
	case tickUpdateMetadataMessage:
		for _, instance := range m.list.GetInstances() {
			if !instance.Started() || instance.Paused() {
				continue
			}
			updated, prompt := instance.HasUpdated()
			if updated {
				instance.SetStatus(session.Running)
			} else {
				if prompt {
					instance.TapEnter()
				} else {
					instance.SetStatus(session.Ready)
				}
			}
			if err := instance.UpdateDiffStats(); err != nil {
				log.WarningLog.Printf("could not update diff stats: %v", err)
			}
		}
		return m, tickUpdateMetadataCmd
	case tea.MouseMsg:
		// Handle mouse wheel events for scrolling the diff/preview pane
		if msg.Action == tea.MouseActionPress {
			if msg.Button == tea.MouseButtonWheelDown || msg.Button == tea.MouseButtonWheelUp {
				selected := m.list.GetSelectedInstance()
				if selected == nil || selected.Status == session.Paused {
					return m, nil
				}

				switch msg.Button {
				case tea.MouseButtonWheelUp:
					m.tabbedWindow.ScrollUp()
				case tea.MouseButtonWheelDown:
					m.tabbedWindow.ScrollDown()
				}
				return m, nil
			}
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	case tea.WindowSizeMsg:
		m.updateHandleWindowSizeEvent(msg)

		// Also update PR review overlay if it's active
		if m.state == statePRReview && m.prReviewOverlay != nil {
			updatedModel, _ := m.prReviewOverlay.Update(msg)
			*m.prReviewOverlay = updatedModel
		}

		return m, nil
	case error:
		// Handle errors from confirmation actions
		return m, m.handleError(msg)
	case instanceChangedMsg:
		// Handle instance changed after confirmation action
		return m, m.instanceChanged()
	case instanceCreatedMsg:
		// Handle instance creation completion
		if msg.err != nil {
			// Remove the instance on error
			m.list.Kill()
			return m, m.handleError(msg.err)
		}
		// Show help screen on successful creation
		m.showHelpScreen(helpStart(msg.instance), nil)
		return m, m.instanceChanged()
	case instanceDeletedMsg:
		// Handle instance deletion completion
		if msg.err != nil {
			return m, m.handleError(msg.err)
		}
		return m, m.instanceChanged()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case allCommentsProcessedMsg:
		// Comments have been processed, return to default state
		m.state = stateDefault
		m.textOverlay = nil

		// Show success message
		// Note: Using error box for now to show success message
		successErr := fmt.Errorf("✓ PR comments processed successfully!")
		m.errBox.SetError(successErr)
		return m, func() tea.Msg {
			time.Sleep(3 * time.Second)
			return hideErrMsg{}
		}
	case testStartedMsg:
		// Show non-obtrusive message that tests are running
		m.errBox.SetError(fmt.Errorf("Running Jest tests..."))
		return m, nil
	case testProgressMsg:
		// Update test progress
		var status string
		if msg.running {
			status = fmt.Sprintf("Running tests: %d/%d passed, %d failed", msg.passed, msg.total, msg.failed)
		} else {
			status = fmt.Sprintf("Tests complete: %d/%d passed, %d failed", msg.passed, msg.total, msg.failed)
		}
		m.errBox.SetError(fmt.Errorf(status))
		return m, nil
	case testResultsMsg:
		// Handle test results
		if msg.err != nil {
			return m, m.handleError(msg.err)
		}

		// Parse final stats from output
		finalStats := parseJestFinalStats(msg.output)

		// Open failed test files in WebStorm if any
		if len(msg.failedFiles) > 0 {
			for _, file := range msg.failedFiles {
				cmd := exec.Command("webstorm", file)
				cmd.Start()
			}
			// Show brief status about failed tests with counts
			m.errBox.SetError(fmt.Errorf("Tests completed: %d/%d passed, %d failed. Opening failed files in WebStorm",
				finalStats.passed, finalStats.total, finalStats.failed))
		} else {
			// All tests passed
			m.errBox.SetError(fmt.Errorf("All tests passed! %d/%d test suites completed",
				finalStats.passed, finalStats.total))
		}

		// Auto-hide the message after 5 seconds (give more time to read the stats)
		return m, func() tea.Msg {
			select {
			case <-time.After(5 * time.Second):
			}
			return hideErrMsg{}
		}
	}
	return m, nil
}

func (m *home) handleQuit() (tea.Model, tea.Cmd) {
	if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
		return m, m.handleError(err)
	}
	return m, tea.Quit
}

func (m *home) handleMenuHighlighting(msg tea.KeyMsg) (cmd tea.Cmd, returnEarly bool) {
	// Handle menu highlighting when you press a button. We intercept it here and immediately return to
	// update the ui while re-sending the keypress. Then, on the next call to this, we actually handle the keypress.
	if m.keySent {
		m.keySent = false
		return nil, false
	}
	if m.state == statePrompt || m.state == stateHelp || m.state == stateConfirm {
		return nil, false
	}
	// If it's in the global keymap, we should try to highlight it.
	name, ok := keys.GetKeyName(msg.String())
	if !ok {
		return nil, false
	}

	if m.list.GetSelectedInstance() != nil && m.list.GetSelectedInstance().Paused() && name == keys.KeyEnter {
		return nil, false
	}
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

	if m.state == stateNew {
		// Handle quit commands first. Don't handle q because the user might want to type that.
		if msg.String() == "ctrl+c" {
			m.state = stateDefault
			m.promptAfterName = false
			m.list.Kill()
			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
			)
		}

		instance := m.list.GetInstances()[m.list.NumInstances()-1]
		switch msg.Type {
		// Start the instance (enable previews etc) and go back to the main menu state.
		case tea.KeyEnter:
			if len(instance.Title) == 0 {
				return m, m.handleError(fmt.Errorf("title cannot be empty"))
			}

			// Start the instance asynchronously
			cmd := m.startInstanceAsync(instance)

			// Save after adding new instance
			if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
				return m, m.handleError(err)
			}

			// Instance added successfully, call the finalizer.
			m.newInstanceFinalizer()
			if m.autoYes {
				instance.AutoYes = true
			}

			m.state = stateDefault
			if m.promptAfterName {
				m.state = statePrompt
				m.menu.SetState(ui.StatePrompt)
				// Initialize the text input overlay
				m.textInputOverlay = overlay.NewTextInputOverlay("Enter prompt", "")
				m.promptAfterName = false
			} else {
				m.menu.SetState(ui.StateDefault)
			}

			return m, tea.Batch(tea.WindowSize(), m.instanceChanged(), cmd)
		case tea.KeyRunes:
			if len(instance.Title) >= 32 {
				return m, m.handleError(fmt.Errorf("title cannot be longer than 32 characters"))
			}
			if err := instance.SetTitle(instance.Title + string(msg.Runes)); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeyBackspace:
			if len(instance.Title) == 0 {
				return m, nil
			}
			if err := instance.SetTitle(instance.Title[:len(instance.Title)-1]); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeySpace:
			if err := instance.SetTitle(instance.Title + " "); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeyEsc:
			m.list.Kill()
			m.state = stateDefault
			m.instanceChanged()

			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
			)
		default:
		}
		return m, nil
	} else if m.state == statePrompt {
		// Use the new TextInputOverlay component to handle all key events
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)

		// Check if the form was submitted or canceled
		if shouldClose {
			selected := m.list.GetSelectedInstance()
			// TODO: this should never happen since we set the instance in the previous state.
			if selected == nil {
				return m, nil
			}
			if m.textInputOverlay.IsSubmitted() {
				if err := selected.SendPrompt(m.textInputOverlay.GetValue()); err != nil {
					// TODO: we probably end up in a bad state here.
					return m, m.handleError(err)
				}
			}

			// Close the overlay and reset state
			m.textInputOverlay = nil
			m.state = stateDefault
			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					m.showHelpScreen(helpStart(selected), nil)
					return nil
				},
			)
		}

		return m, nil
	} else if m.state == stateBookmark {
		// Handle bookmark state
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)

		if shouldClose {
			selected := m.list.GetSelectedInstance()
			if selected == nil {
				return m, nil
			}

			var finalCmd tea.Cmd = tea.WindowSize()
			if m.textInputOverlay.IsSubmitted() {
				// Create bookmark commit
				commitMsg := m.textInputOverlay.GetValue()
				cmd := m.createBookmarkCommit(selected, commitMsg)
				finalCmd = tea.Batch(tea.WindowSize(), cmd)
			}

			// Common state reset logic
			m.textInputOverlay = nil
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)

			return m, finalCmd
		}

		return m, nil
	}

	// Handle confirmation state
	if m.state == stateConfirm {
		shouldClose := m.confirmationOverlay.HandleKeyPress(msg)
		if shouldClose {
			// Capture confirmation state before clearing overlay
			wasConfirmed := m.confirmationOverlay.IsConfirmed()
			m.state = stateDefault
			m.confirmationOverlay = nil

			// Execute pending command if confirmed
			if wasConfirmed && m.pendingCmd != nil {
				cmd := m.pendingCmd
				m.pendingCmd = nil
				// Execute the action and get the result
				result := cmd()
				// If result is a tea.Cmd, return it to be executed
				if resultCmd, ok := result.(tea.Cmd); ok {
					return m, resultCmd
				}
				// Otherwise handle as a message
				return m.Update(result)
			}
			m.pendingCmd = nil
			return m, nil
		}
		return m, nil
	}

	// Exit scrolling mode when ESC is pressed and preview or terminal pane is in scrolling mode
	// Check if Escape key was pressed
	// Always check for escape key first to ensure it doesn't get intercepted elsewhere
	if msg.Type == tea.KeyEsc {
		// Use the selected instance from the list
		selected := m.list.GetSelectedInstance()

		// If in preview tab and in scroll mode, exit scroll mode
		if !m.tabbedWindow.IsInDiffTab() && !m.tabbedWindow.IsInTerminalTab() && m.tabbedWindow.IsPreviewInScrollMode() {
			err := m.tabbedWindow.ResetPreviewToNormalMode(selected)
			if err != nil {
				return m, m.handleError(err)
			}
			return m, m.instanceChanged()
		}

		// If in terminal tab and in scroll mode, exit scroll mode
		if m.tabbedWindow.IsInTerminalTab() && m.tabbedWindow.IsTerminalInScrollMode() {
			err := m.tabbedWindow.ResetTerminalToNormalMode(selected)
			if err != nil {
				return m, m.handleError(err)
			}
			return m, m.instanceChanged()
		}
	}

	// Handle quit commands first
	if msg.String() == "ctrl+c" || msg.String() == "q" {
		return m.handleQuit()
	}

	name, ok := keys.GetKeyName(msg.String())
	if !ok {
		return m, nil
	}

	switch name {
	case keys.KeyHelp:
		return m.showHelpScreen(helpTypeGeneral{}, nil)
	case keys.KeyErrorLog:
		return m.showErrorLog()
	case keys.KeyEditKeybindings:
		m.state = stateKeybindingEditor
		m.keybindingEditorOverlay = overlay.NewKeybindingEditorOverlay()
		return m, nil
	case keys.KeyPrompt:
		if m.list.NumInstances() >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
		}
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:   "",
			Path:    ".",
			Program: m.program,
		})
		if err != nil {
			return m, m.handleError(err)
		}

		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)
		m.promptAfterName = true

		return m, nil
	case keys.KeyNew:
		if m.list.NumInstances() >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
		}
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:   "",
			Path:    ".",
			Program: m.program,
		})
		if err != nil {
			return m, m.handleError(err)
		}

		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)

		return m, nil
	case keys.KeyExistingBranch:
		if m.list.NumInstances() >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
		}

		// Show branch selector
		m.state = stateBranchSelect
		m.menu.SetState(ui.StateNewInstance)

		// Get list of remote branches
		branches, err := git.ListRemoteBranchesFromRepo(".")
		if err != nil {
			return m, m.handleError(fmt.Errorf("failed to list remote branches: %w", err))
		}

		// Check if there are any branches
		if len(branches) == 0 {
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, m.handleError(fmt.Errorf("no remote branches found"))
		}

		// Create branch selector overlay
		m.branchSelectorOverlay = overlay.NewBranchSelectorOverlay(branches)

		// Initialize the branch selector
		return m, m.branchSelectorOverlay.Init()
	case keys.KeyUp:
		if m.scrollLocked && m.tabbedWindow.IsInDiffTab() {
			m.tabbedWindow.ScrollUp()
		} else {
			m.list.Up()
		}
		return m, m.instanceChanged()
	case keys.KeyDown:
		if m.scrollLocked && m.tabbedWindow.IsInDiffTab() {
			m.tabbedWindow.ScrollDown()
		} else {
			m.list.Down()
		}
		return m, m.instanceChanged()
	case keys.KeyShiftUp:
		m.tabbedWindow.ScrollUp()
		return m, nil
	case keys.KeyShiftDown:
		m.tabbedWindow.ScrollDown()
		return m, nil
	case keys.KeyHome:
		if m.tabbedWindow.IsInDiffTab() {
			m.tabbedWindow.ScrollToTop()
		}
		return m, m.instanceChanged()
	case keys.KeyEnd:
		if m.tabbedWindow.IsInDiffTab() {
			m.tabbedWindow.ScrollToBottom()
		}
		return m, m.instanceChanged()
	case keys.KeyPageUp:
		if m.tabbedWindow.IsInDiffTab() {
			m.tabbedWindow.PageUp()
		}
		return m, m.instanceChanged()
	case keys.KeyPageDown:
		if m.tabbedWindow.IsInDiffTab() {
			m.tabbedWindow.PageDown()
		}
		return m, m.instanceChanged()
	case keys.KeyAltUp:
		if m.tabbedWindow.IsInDiffTab() {
			m.tabbedWindow.JumpToPrevFile()
		}
		return m, m.instanceChanged()
	case keys.KeyAltDown:
		if m.tabbedWindow.IsInDiffTab() {
			m.tabbedWindow.JumpToNextFile()
		}
		return m, m.instanceChanged()
	case keys.KeyTab:
		m.tabbedWindow.Toggle()
		m.menu.SetInDiffTab(m.tabbedWindow.IsInDiffTab())
		return m, m.instanceChanged()
	case keys.KeyDiffAll:
		if m.tabbedWindow.IsInDiffTab() {
			m.tabbedWindow.SetDiffModeAll()
		}
		return m, m.instanceChanged()
	case keys.KeyDiffLastCommit:
		if m.tabbedWindow.IsInDiffTab() {
			m.tabbedWindow.SetDiffModeLastCommit()
		}
		return m, m.instanceChanged()
	case keys.KeyLeft:
		if m.tabbedWindow.IsInDiffTab() {
			m.tabbedWindow.NavigateToPrevCommit()
		}
		return m, m.instanceChanged()
	case keys.KeyRight:
		if m.tabbedWindow.IsInDiffTab() {
			m.tabbedWindow.NavigateToNextCommit()
		}
		return m, m.instanceChanged()
	case keys.KeyScrollLock:
		if m.tabbedWindow.IsInDiffTab() {
			m.scrollLocked = !m.scrollLocked
			m.menu.SetScrollLocked(m.scrollLocked)
		}
		return m, nil
	case keys.KeyOpenInIDE:
		// Only handle 'i' when in diff view
		if m.tabbedWindow.IsInDiffTab() {
			selected := m.list.GetSelectedInstance()
			if selected == nil {
				return m, nil
			}
			// Get the current file from diff view
			currentFile := m.tabbedWindow.GetCurrentDiffFile()
			if currentFile == "" {
				return m, m.handleError(fmt.Errorf("no file selected in diff view"))
			}
			// Open the file in WebStorm
			cmd := m.openFileInWebStorm(selected, currentFile)
			return m, cmd
		}
		return m, nil
	case keys.KeyKill:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Create the kill action as a tea.Cmd
		killAction := func() tea.Msg {
			// Delete from storage first
			if err := m.storage.DeleteInstance(selected.Title); err != nil {
				return err
			}

			// Start async kill and return a command
			// The kill logic will handle checked out branches
			return m.killInstanceAsync(selected)
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
		return m, tea.WindowSize()
	case keys.KeyWebStorm:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		// Open WebStorm at the instance's path and connect Claude
		cmd := m.openWebStorm(selected)
		return m, cmd
	case keys.KeyRebase:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Create the rebase action as a tea.Cmd
		rebaseAction := func() tea.Msg {
			worktree, err := selected.GetGitWorktree()
			if err != nil {
				return err
			}

			// Check if there are uncommitted changes
			isDirty, err := worktree.IsDirty()
			if err != nil {
				return err
			}

			if isDirty {
				return fmt.Errorf("cannot rebase: you have uncommitted changes. Please commit or stash them first")
			}

			// Perform the rebase
			if err := worktree.RebaseWithMain(); err != nil {
				return err
			}

			return instanceChangedMsg{}
		}

		// Show confirmation modal
		message := fmt.Sprintf("[!] Rebase session '%s' with main branch?", selected.Title)
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
			return m, m.handleError(fmt.Errorf("instance '%s' is paused - please resume it first", selected.Title))
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
			return m, m.handleError(fmt.Errorf("no pull request found for branch in %s: %w", worktreePath, err))
		}

		// Fetch PR comments (always fresh - includes resolved status detection)
		if err := pr.FetchComments(worktreePath); err != nil {
			return m, m.handleError(fmt.Errorf("failed to fetch PR comments: %w", err))
		}

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

// instanceChanged updates the AI pane, menu, diff pane, and terminal pane based on the selected instance. It returns an error
// Cmd if there was any error.
func (m *home) openWebStorm(instance *session.Instance) tea.Cmd {
	return func() tea.Msg {
		// Get the git worktree to access the worktree path
		gitWorktree, err := instance.GetGitWorktree()
		if err != nil {
			return fmt.Errorf("failed to get git worktree: %w", err)
		}

		// Open WebStorm at the worktree path (not the git root)
		worktreePath := gitWorktree.GetWorktreePath()
		cmd := exec.Command("webstorm", worktreePath)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to open WebStorm: %w", err)
		}

		return nil
	}
}

func (m *home) openFileInWebStorm(instance *session.Instance, filePath string) tea.Cmd {
	return func() tea.Msg {
		// Get the git worktree to access the worktree path
		gitWorktree, err := instance.GetGitWorktree()
		if err != nil {
			return fmt.Errorf("failed to get git worktree: %w", err)
		}

		// Construct the full path to the file using the worktree path
		worktreePath := gitWorktree.GetWorktreePath()
		fullPath := filepath.Join(worktreePath, filePath)

		// Open WebStorm with the specific file
		cmd := exec.Command("webstorm", fullPath)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to open file in WebStorm: %w", err)
		}

		return nil
	}
}

const (
	// maxBookmarkSummaryLen is the maximum length for auto-generated bookmark commit message summaries
	maxBookmarkSummaryLen = 100
)

func (m *home) createBookmarkCommit(instance *session.Instance, userMessage string) tea.Cmd {
	return func() tea.Msg {
		worktree, err := instance.GetGitWorktree()
		if err != nil {
			return fmt.Errorf("failed to get git worktree: %w", err)
		}

		// Get current branch name
		currentBranch, err := worktree.GetCurrentBranch()
		if err != nil {
			return fmt.Errorf("failed to get current branch: %w", err)
		}

		var commitMessage string
		if userMessage != "" {
			// Use user-provided message
			commitMessage = fmt.Sprintf("[BOOKMARK] %s", userMessage)
		} else {
			// Generate message from commits since last bookmark
			lastBookmarkSHA, err := worktree.FindLastBookmarkCommit(currentBranch)
			if err != nil {
				return fmt.Errorf("failed to find last bookmark: %w", err)
			}

			// Get commit messages since last bookmark
			messages, err := worktree.GetCommitMessagesSince(lastBookmarkSHA, currentBranch)
			if err != nil {
				return fmt.Errorf("failed to get commit messages: %w", err)
			}

			if len(messages) == 0 {
				commitMessage = "[BOOKMARK] No changes since last bookmark"
			} else {
				// Generate a summary by concatenating the commit messages
				summary := strings.Join(messages, "; ")
				if len(summary) > maxBookmarkSummaryLen {
					summary = summary[:maxBookmarkSummaryLen-len("...")] + "..."
				}
				commitMessage = fmt.Sprintf("[BOOKMARK] %s", summary)
			}
		}

		// Create the bookmark commit (allow empty)
		if err := worktree.CreateBookmarkCommit(commitMessage); err != nil {
			return fmt.Errorf("failed to create bookmark commit: %w", err)
		}

		return instanceChangedMsg{}
	}
}

func (m *home) runJestTests(instance *session.Instance) tea.Cmd {
	return tea.Sequence(
		// First, send a message that tests have started
		func() tea.Msg {
			return testStartedMsg{}
		},
		// Then run the tests with progress tracking
		m.runJestTestsWithProgress(instance),
	)
}

func (m *home) runJestTestsWithProgress(instance *session.Instance) tea.Cmd {
	return func() tea.Msg {
		// Get the git worktree to access the worktree path
		gitWorktree, err := instance.GetGitWorktree()
		if err != nil {
			return testResultsMsg{err: fmt.Errorf("failed to get git worktree: %w", err)}
		}

		// Construct the path to the web directory
		worktreePath := gitWorktree.GetWorktreePath()

		// Run npm test without watch mode
		cmd := exec.Command("yarn", "test", "--watchAll=false", "--json", "--outputFile=test-results.json")
		cmd.Dir = worktreePath
		// Capture output
		output, _ := cmd.CombinedOutput()

		// Parse failed test files from output
		failedFiles := parseFailedTestFiles(string(output), worktreePath)
		// Also try to read the JSON output file for more reliable parsing
		jsonPath := filepath.Join(worktreePath, "test-results.json")
		if jsonData, err := os.ReadFile(jsonPath); err == nil {
			// Parse JSON for failed files if available
			if jsonFailedFiles := parseJestJSON(jsonData, worktreePath); len(jsonFailedFiles) > 0 {
				failedFiles = jsonFailedFiles
			}
			// Clean up the JSON file
			os.Remove(jsonPath)
		}

		return testResultsMsg{
			output:      string(output),
			failedFiles: failedFiles,
			err:         nil,
		}
	}
}

// parseFailedTestFiles parses Jest output to find failed test file paths
func parseFailedTestFiles(output string, webPath string) []string {
	var failedFiles []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		// Look for FAIL lines which contain the test file path
		if strings.HasPrefix(strings.TrimSpace(line), "FAIL") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// The file path is usually the second part after FAIL
				testFile := parts[1]
				// Convert relative path to absolute
				if !filepath.IsAbs(testFile) {
					testFile = filepath.Join(webPath, testFile)
				}
				// Check if file exists
				if _, err := os.Stat(testFile); err == nil {
					failedFiles = append(failedFiles, testFile)
				}
			}
		}
	}

	return failedFiles
}

// parseJestJSON parses Jest JSON output to find failed test files
func parseJestJSON(jsonData []byte, webPath string) []string {
	var failedFiles []string

	// Simple JSON parsing for test results
	// Jest JSON format includes testResults array with status and name fields
	type TestResult struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}

	type JestResults struct {
		TestResults []TestResult `json:"testResults"`
	}

	var results JestResults
	if err := json.Unmarshal(jsonData, &results); err == nil {
		for _, result := range results.TestResults {
			if result.Status == "failed" {
				testFile := result.Name
				// Convert relative path to absolute
				if !filepath.IsAbs(testFile) {
					testFile = filepath.Join(webPath, testFile)
				}
				// Check if file exists
				if _, err := os.Stat(testFile); err == nil {
					failedFiles = append(failedFiles, testFile)
				}
			}
		}
	}

	return failedFiles
}

// testStats holds test statistics
type testStats struct {
	passed int
	failed int
	total  int
}

// parseJestFinalStats parses the final test summary from Jest output
func parseJestFinalStats(output string) testStats {
	stats := testStats{}

	// Look for the test suites summary line
	// Example: "Test Suites: 1 passed, 1 failed, 2 total"
	re := regexp.MustCompile(`Test Suites:\s*(\d+)\s*passed(?:,\s*(\d+)\s*failed)?.*?,\s*(\d+)\s*total`)
	matches := re.FindStringSubmatch(output)

	if len(matches) >= 4 {
		if passed, err := strconv.Atoi(matches[1]); err == nil {
			stats.passed = passed
		}
		if len(matches) > 2 && matches[2] != "" {
			if failed, err := strconv.Atoi(matches[2]); err == nil {
				stats.failed = failed
			}
		}
		if total, err := strconv.Atoi(matches[3]); err == nil {
			stats.total = total
		}
	}

	// If no failed count was found, calculate it
	if stats.failed == 0 && stats.total > stats.passed {
		stats.failed = stats.total - stats.passed
	}

	return stats
}

func (m *home) instanceChanged() tea.Cmd {
	// selected may be nil
	selected := m.list.GetSelectedInstance()

	m.tabbedWindow.UpdateDiff(selected)
	m.tabbedWindow.UpdateTerminal(selected)
	// Update menu with current instance
	m.menu.SetInstance(selected)

	// If there's no selected instance, we don't need to update the preview.
	if err := m.tabbedWindow.UpdatePreview(selected); err != nil {
		return m.handleError(err)
	}
	return nil
}

type keyupMsg struct{}

// keydownCallback clears the menu option highlighting after 500ms.
func (m *home) keydownCallback(name keys.KeyName) tea.Cmd {
	m.menu.Keydown(name)
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(500 * time.Millisecond):
		}

		return keyupMsg{}
	}
}

// hideErrMsg implements tea.Msg and clears the error text from the screen.
type hideErrMsg struct{}

// previewTickMsg implements tea.Msg and triggers a preview update
type previewTickMsg struct{}

type tickUpdateMetadataMessage struct{}

type instanceChangedMsg struct{}

// instanceCreatedMsg is sent when an instance has been created successfully
type instanceCreatedMsg struct {
	instance *session.Instance
	err      error
}

// instanceDeletedMsg is sent when an instance has been deleted successfully
type instanceDeletedMsg struct {
	title string
	err   error
}

// testResultsMsg is sent when test results are available
type testResultsMsg struct {
	output      string
	failedFiles []string
	err         error
}

// testStartedMsg is sent when tests start running
type testStartedMsg struct{}

// testProgressMsg is sent with test progress updates
type testProgressMsg struct {
	passed  int
	failed  int
	total   int
	running bool
}

// tickUpdateMetadataCmd is the callback to update the metadata of the instances every 500ms. Note that we iterate
// overall the instances and capture their output. It's a pretty expensive operation. Let's do it 2x a second only.
var tickUpdateMetadataCmd = func() tea.Msg {
	time.Sleep(500 * time.Millisecond)
	return tickUpdateMetadataMessage{}
}

// startInstanceAsync starts an instance asynchronously and returns a tea.Cmd
func (m *home) startInstanceAsync(instance *session.Instance) tea.Cmd {
	return func() tea.Msg {
		var resultErr error
		done := make(chan struct{})

		instance.StartAsync(true, func(err error) {
			resultErr = err
			close(done)
		})

		// Wait for completion
		<-done

		return instanceCreatedMsg{
			instance: instance,
			err:      resultErr,
		}
	}
}

// killInstanceAsync kills an instance asynchronously and returns a tea.Cmd
func (m *home) killInstanceAsync(instance *session.Instance) tea.Cmd {
	return func() tea.Msg {
		var resultErr error
		done := make(chan struct{})
		title := instance.Title

		instance.KillAsync(func(err error) {
			if err != nil {
				// If normal kill fails, try force kill
				log.InfoLog.Printf("Normal kill failed for %s: %v. Attempting force kill...", title, err)
				forceDone := make(chan struct{})
				var forceErr error

				instance.ForceKillAsync(func(err error) {
					forceErr = err
					close(forceDone)
				})

				<-forceDone

				if forceErr != nil {
					// Log the error but don't fail - we still want to remove the instance
					log.ErrorLog.Printf("Force kill encountered errors for %s: %v", title, forceErr)
					resultErr = nil // Set to nil so instance is removed from UI
				} else {
					// Force kill succeeded
					resultErr = nil
					log.InfoLog.Printf("Force kill succeeded for %s", title)
				}
			} else {
				resultErr = nil
			}
			close(done)
		})

		// Wait for completion
		<-done

		// Always remove from UI list after kill attempt
		// Even if there were errors, the instance should be considered gone
		m.list.Kill()

		return instanceDeletedMsg{
			title: title,
			err:   resultErr,
		}
	}
}

// handleError handles all errors which get bubbled up to the app. sets the error message. We return a callback tea.Cmd that returns a hideErrMsg message
// which clears the error message after 3 seconds.
func (m *home) handleError(err error) tea.Cmd {
	log.ErrorLog.Printf("%v", err)
	m.errBox.SetError(err)

	// Store error in the error log with timestamp
	timestamp := time.Now().Format("15:04:05")
	errorMsg := fmt.Sprintf("[%s] %v", timestamp, err)
	m.errorLog = append(m.errorLog, errorMsg)

	// Keep only the last 100 errors to prevent memory issues
	if len(m.errorLog) > 100 {
		m.errorLog = m.errorLog[len(m.errorLog)-100:]
	}

	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(3 * time.Second):
		}

		return hideErrMsg{}
	}
}

// confirmAction shows a confirmation modal and stores the action to execute on confirm
func (m *home) confirmAction(message string, action tea.Cmd) tea.Cmd {
	m.state = stateConfirm

	// Create and show the confirmation overlay using ConfirmationOverlay
	m.confirmationOverlay = overlay.NewConfirmationOverlay(message)
	// Set a fixed width for consistent appearance
	m.confirmationOverlay.SetWidth(50)

	// Store the pending command
	m.pendingCmd = action

	// Set callbacks for confirmation and cancellation
	m.confirmationOverlay.OnConfirm = func() {
		m.state = stateDefault
	}

	m.confirmationOverlay.OnCancel = func() {
		m.state = stateDefault
		m.pendingCmd = nil
	}

	return nil
}

func (m *home) View() string {
	listWithPadding := lipgloss.NewStyle().PaddingTop(1).Render(m.list.String())
	previewWithPadding := lipgloss.NewStyle().PaddingTop(1).Render(m.tabbedWindow.String())
	listAndPreview := lipgloss.JoinHorizontal(lipgloss.Top, listWithPadding, previewWithPadding)

	mainView := lipgloss.JoinVertical(
		lipgloss.Center,
		listAndPreview,
		m.menu.String(),
		m.errBox.String(),
	)

	if m.state == statePrompt {
		if m.textInputOverlay == nil {
			log.ErrorLog.Printf("text input overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
	} else if m.state == stateHelp {
		if m.textOverlay == nil {
			log.ErrorLog.Printf("text overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.textOverlay.Render(), mainView, true, true)
	} else if m.state == stateConfirm {
		if m.confirmationOverlay == nil {
			log.ErrorLog.Printf("confirmation overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.confirmationOverlay.Render(), mainView, true, true)
	} else if m.state == stateBranchSelect {
		if m.branchSelectorOverlay == nil {
			log.ErrorLog.Printf("branch selector overlay is nil")
			// Return to default state if overlay is nil
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return mainView
		}
		return overlay.PlaceOverlay(0, 0, m.branchSelectorOverlay.View(), mainView, true, true)
	} else if m.state == stateErrorLog {
		if m.textOverlay == nil {
			log.ErrorLog.Printf("error log overlay is nil")
			m.state = stateDefault
			return mainView
		}
		return overlay.PlaceOverlay(0, 0, m.textOverlay.Render(), mainView, true, true)
	} else if m.state == statePRReview {
		if m.prReviewOverlay == nil {
			log.ErrorLog.Printf("PR review overlay is nil")
			m.state = stateDefault
			return mainView
		}
		// Return PR review directly - it manages its own full-screen layout
		return m.prReviewOverlay.View()
	} else if m.state == stateBookmark {
		if m.textInputOverlay == nil {
			log.ErrorLog.Printf("text input overlay is nil")
			m.state = stateDefault
			return mainView
		}
		return overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
	} else if m.state == stateHistory {
		if m.historyOverlay == nil {
			log.ErrorLog.Printf("history overlay is nil")
			m.state = stateDefault
			return mainView
		}
		return overlay.PlaceOverlay(0, 0, m.historyOverlay.Render(), mainView, true, true)
	} else if m.state == stateKeybindingEditor {
		if m.keybindingEditorOverlay == nil {
			log.ErrorLog.Printf("keybinding editor overlay is nil")
			m.state = stateDefault
			return mainView
		}
		return overlay.PlaceOverlay(0, 0, m.keybindingEditorOverlay.Render(), mainView, true, true)
	}

	return mainView
}

func (m *home) handleErrorLogState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Any key press closes the error log
	m.state = stateDefault
	m.textOverlay = nil
	return m, nil
}

// handleHistoryState handles key events when in history state
func (m *home) handleHistoryState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Let the history overlay handle the key press
	shouldClose := m.historyOverlay.HandleKeyPress(msg)
	if shouldClose {
		m.state = stateDefault
		m.menu.SetState(ui.StateDefault)
		m.historyOverlay = nil
		return m, tea.WindowSize()
	}

	// Update the viewport
	_, cmd := m.historyOverlay.Update(msg)
	return m, cmd
}

func (m *home) handleKeybindingEditorState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.keybindingEditorOverlay == nil {
		m.state = stateDefault
		return m, nil
	}

	// Let the overlay handle the key press
	if m.keybindingEditorOverlay.HandleKeyPress(msg) {
		// Overlay was dismissed, reload keybindings
		m.state = stateDefault
		m.keybindingEditorOverlay = nil

		// Reload keybindings
		if err := keys.InitializeCustomKeyBindings(); err != nil {
			log.ErrorLog.Printf("Failed to reload custom keybindings: %v", err)
		}

		// Update menu to reflect new keybindings
		m.menu = ui.NewMenu()

		return m, nil
	}

	return m, nil
}

func (m *home) showTestResults(output string) {
	// Create text overlay with test results
	m.textOverlay = overlay.NewTextOverlay(output)
	m.state = stateHelp // Use help state since it handles text overlay display
	m.menu.SetState(ui.StateDefault)
}

func (m *home) showErrorLog() (tea.Model, tea.Cmd) {
	// Create content for error log
	var content string
	if len(m.errorLog) == 0 {
		content = "No errors have been logged."
	} else {
		content = lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("Error Log"),
			"",
			"Recent errors (newest first):",
			"")

		// Show errors in reverse order (newest first)
		for i := len(m.errorLog) - 1; i >= 0; i-- {
			content = lipgloss.JoinVertical(lipgloss.Left,
				content,
				m.errorLog[i])
		}

		content = lipgloss.JoinVertical(lipgloss.Left,
			content,
			"",
			dimStyle.Render("Press any key to close"))
	}

	// Create text overlay
	m.textOverlay = overlay.NewTextOverlay(content)
	m.state = stateErrorLog
	m.menu.SetState(ui.StateDefault)

	return m, nil
}

func (m *home) createInstanceWithBranch(branchName string) (tea.Model, tea.Cmd) {
	// Create a unique title by adding a timestamp suffix
	// This prevents tmux session name conflicts when checking out the same branch multiple times
	timestamp := time.Now().Format("150405") // HHMMSS format
	title := fmt.Sprintf("%s-%s", branchName, timestamp)

	// Create a new instance with the selected branch
	instance, err := session.NewInstanceWithBranch(session.InstanceOptions{
		Title:      title,
		Path:       ".",
		Program:    m.program,
		BranchName: branchName,
	})
	if err != nil {
		m.state = stateDefault
		m.menu.SetState(ui.StateDefault)
		m.branchSelectorOverlay = nil
		return m, m.handleError(err)
	}

	m.newInstanceFinalizer = m.list.AddInstance(instance)
	m.list.SetSelectedInstance(m.list.NumInstances() - 1)
	m.branchSelectorOverlay = nil

	// Start the instance asynchronously
	cmd := m.startInstanceAsync(instance)

	// Save after adding new instance
	if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
		return m, m.handleError(err)
	}

	// Instance added successfully, call the finalizer
	m.newInstanceFinalizer()

	// Set state back to default
	m.state = stateDefault
	m.menu.SetState(ui.StateDefault)

	return m, tea.Batch(m.instanceChanged(), cmd)
}

// showHistoryView displays the history overlay for the current pane
func (m *home) showHistoryView() tea.Cmd {
	selected := m.list.GetSelectedInstance()
	if selected == nil {
		return nil
	}

	var content string
	var title string
	var err error

	// Determine which pane's history to show based on the active tab
	if m.tabbedWindow.IsInTerminalTab() {
		// Show terminal pane history
		content, err = selected.GetTerminalFullHistory()
		if err != nil {
			return m.handleError(fmt.Errorf("failed to get terminal history: %v", err))
		}
		title = fmt.Sprintf("Terminal History - %s", selected.Title)
	} else if m.tabbedWindow.IsInAITab() {
		// Show AI pane history
		content, err = selected.GetAIFullHistory()
		if err != nil {
			return m.handleError(fmt.Errorf("failed to get AI history: %v", err))
		}
		title = fmt.Sprintf("AI History - %s", selected.Title)
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
