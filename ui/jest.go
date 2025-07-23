package ui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"claude-squad/session"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

type JestPane struct {
	width    int
	height   int
	viewport viewport.Model
	content  string
	// Per-instance state maps
	instanceStates  map[string]*JestInstanceState
	currentInstance *session.Instance
	mu              sync.Mutex
}

type JestInstanceState struct {
	running      bool
	testResults  []TestResult
	failedFiles  []string
	workingDir   string
	currentIndex int
	liveOutput   string
	cmd          *exec.Cmd
	outputChan   chan string
}

type TestResult struct {
	FilePath    string
	TestName    string
	Status      string
	ErrorOutput string
	Line        int
}

func NewJestPane() *JestPane {
	vp := viewport.New(0, 0)
	return &JestPane{
		viewport:       vp,
		instanceStates: make(map[string]*JestInstanceState),
	}
}

func (j *JestPane) SetSize(width, height int) {
	j.width = width
	j.height = height
	j.viewport.Width = width
	j.viewport.Height = height - 4 // Leave room for header and status
	j.viewport.SetContent(j.formatContent())
}

func (j *JestPane) getInstanceKey(instance *session.Instance) string {
	if instance == nil {
		return ""
	}
	// Use a combination of Path and Branch as a unique key
	// This handles cases where Title might be empty
	key := fmt.Sprintf("%s:%s", instance.Path, instance.Branch)
	if instance.Title != "" {
		key = instance.Title
	}
	return key
}

func (j *JestPane) getCurrentState() *JestInstanceState {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.currentInstance == nil {
		return nil
	}

	key := j.getInstanceKey(j.currentInstance)
	state, exists := j.instanceStates[key]
	if !exists {
		// Create a new state for this instance if it doesn't exist
		state = &JestInstanceState{
			testResults:  []TestResult{},
			failedFiles:  []string{},
			currentIndex: -1,
		}
		j.instanceStates[key] = state
	}
	return state
}

func (j *JestPane) getOrCreateState(instance *session.Instance) *JestInstanceState {
	j.mu.Lock()
	defer j.mu.Unlock()

	if instance == nil {
		return nil
	}

	key := j.getInstanceKey(instance)
	state, exists := j.instanceStates[key]
	if !exists {
		state = &JestInstanceState{
			testResults:  []TestResult{},
			failedFiles:  []string{},
			currentIndex: -1,
		}
		j.instanceStates[key] = state
	}
	return state
}

func (j *JestPane) SetInstance(instance *session.Instance) {
	j.mu.Lock()
	j.currentInstance = instance
	j.mu.Unlock()
	j.viewport.SetContent(j.formatContent())
}

func (j *JestPane) String() string {
	if j.height < 5 {
		return ""
	}

	header := titleStyle.Render("Jest Test Runner")

	var status string
	var instanceInfo string
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	if j.currentInstance != nil {
		instanceInfo = fmt.Sprintf(" - %s", j.currentInstance.Title)
	}

	state := j.getCurrentState()
	if j.currentInstance == nil {
		status = statusStyle.Render("No instance selected")
	} else if state.running {
		status = statusStyle.Render("⏳ Running tests...")
	} else if len(state.failedFiles) > 0 {
		status = failureStyle.Render(fmt.Sprintf("❌ %d test(s) failed", len(state.failedFiles)))
	} else if len(state.testResults) > 0 {
		status = successStyle.Render("✅ All tests passed")
	} else {
		status = statusStyle.Render("No tests run yet")
	}

	content := j.viewport.View()

	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	help := helpStyle.Render("↑/↓: scroll • r: rerun tests • ESC: exit scroll mode")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header+instanceInfo,
		status,
		content,
		help,
	)
}

func (j *JestPane) formatContent() string {
	state := j.getCurrentState()
	if state == nil {
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		return dimStyle.Render("No instance selected")
	}

	// Always show raw output if available (whether running or not)
	if state.liveOutput != "" {
		return state.liveOutput
	}

	// If no output yet
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	return dimStyle.Render("No test results to display.\nPress 'r' to run tests.")
}

func (j *JestPane) RunTests(instance *session.Instance) error {
	state := j.getOrCreateState(instance)
	if state == nil {
		return fmt.Errorf("no instance provided")
	}

	// Stop any existing test run for this instance
	j.stopTests(instance)

	// Reset state
	j.mu.Lock()
	state.running = true
	state.testResults = []TestResult{}
	state.failedFiles = []string{}
	state.currentIndex = -1
	state.liveOutput = ""
	j.mu.Unlock()

	// Find Jest working directory
	workDir, err := j.findJestWorkingDir(instance.Path)
	if err != nil {
		j.mu.Lock()
		state.running = false
		state.liveOutput = errorStyle.Render(fmt.Sprintf("Error finding Jest: %v", err))
		j.mu.Unlock()
		j.viewport.SetContent(j.formatContent())
		return err
	}

	state.workingDir = workDir

	// Create output channel for live updates
	outputChan := make(chan string, 100)
	state.outputChan = outputChan

	// Start goroutine to update live output
	go func() {
		for line := range outputChan {
			j.mu.Lock()
			state.liveOutput += line + "\n"
			j.mu.Unlock()
			j.viewport.SetContent(j.formatContent())
		}
	}()

	// Run Jest with streaming output
	go j.runJestWithStream(instance, state, workDir, outputChan)

	return nil
}

func (j *JestPane) runJestWithStream(instance *session.Instance, state *JestInstanceState, workDir string, outputChan chan<- string) {
	defer close(outputChan)

	// Run Jest without JSON for live output
	cmd := exec.Command("yarn", "tester")
	cmd.Dir = workDir
	
	// Debug: Log the command being run
	outputChan <- fmt.Sprintf("Running: yarn tester in %s\n", workDir)

	// Store cmd in state so we can kill it if needed
	j.mu.Lock()
	state.cmd = cmd
	j.mu.Unlock()

	// Create pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		outputChan <- fmt.Sprintf("Error creating stdout pipe: %v", err)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		outputChan <- fmt.Sprintf("Error creating stderr pipe: %v", err)
		return
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		outputChan <- fmt.Sprintf("Error starting Jest: %v", err)
		j.mu.Lock()
		state.running = false
		j.mu.Unlock()
		return
	}

	// Collect all output for parsing
	var allOutput strings.Builder
	failedFiles := []string{}

	// Create a wait group to ensure both stdout and stderr are read
	var wg sync.WaitGroup
	wg.Add(2)

	// Read stdout in a goroutine
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			outputChan <- line
			allOutput.WriteString(line + "\n")

			// Look for test failures in real-time
			if strings.Contains(line, "FAIL") {
				// Extract file path from FAIL line
				parts := strings.Fields(line)
				for _, part := range parts {
					if strings.HasSuffix(part, ".js") || strings.HasSuffix(part, ".jsx") ||
						strings.HasSuffix(part, ".ts") || strings.HasSuffix(part, ".tsx") {
						absPath := part
						if !filepath.IsAbs(part) {
							absPath = filepath.Join(workDir, part)
						}
						failedFiles = append(failedFiles, absPath)
						break
					}
				}
			}
		}
		if err := scanner.Err(); err != nil {
			outputChan <- fmt.Sprintf("Error reading stdout: %v", err)
		}
	}()

	// Read stderr in a goroutine
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			outputChan <- line
			allOutput.WriteString(line + "\n")
		}
		if err := scanner.Err(); err != nil {
			outputChan <- fmt.Sprintf("Error reading stderr: %v", err)
		}
	}()

	// Wait for both readers to finish
	wg.Wait()

	// Wait for command to finish
	if err := cmd.Wait(); err != nil {
		outputChan <- fmt.Sprintf("\nCommand finished with error: %v", err)
	}

	// Auto-open failed files in IDE
	if len(failedFiles) > 0 {
		j.autoOpenFailedTests(failedFiles)
	}

	j.mu.Lock()
	state.running = false
	state.cmd = nil
	// Keep the liveOutput so it persists after tests complete
	j.mu.Unlock()
	j.viewport.SetContent(j.formatContent())
}

func (j *JestPane) stopTests(instance *session.Instance) {
	state := j.getOrCreateState(instance)
	if state == nil || state.cmd == nil {
		return
	}

	j.mu.Lock()
	if state.cmd.Process != nil {
		state.cmd.Process.Kill()
	}
	if state.outputChan != nil {
		// Don't close the channel here as the goroutine will close it
		state.outputChan = nil
	}
	state.running = false
	j.mu.Unlock()
}

func (j *JestPane) autoOpenFailedTests(failedFiles []string) {
	ideCmd := getIDECommand()
	if ideCmd == "" {
		return
	}

	// Open up to 5 failed test files
	maxFiles := 5
	if len(failedFiles) < maxFiles {
		maxFiles = len(failedFiles)
	}

	for i := 0; i < maxFiles; i++ {
		cmd := exec.Command(ideCmd, failedFiles[i])
		go cmd.Start()
		// Small delay to avoid overwhelming the IDE
		time.Sleep(100 * time.Millisecond)
	}
}

func (j *JestPane) findJestWorkingDir(startPath string) (string, error) {
	// First check if startPath is a file or directory
	info, err := os.Stat(startPath)
	if err != nil {
		return "", err
	}

	var searchDir string
	if info.IsDir() {
		searchDir = startPath
	} else {
		searchDir = filepath.Dir(startPath)
	}

	// First, check if the starting directory itself has a package.json
	packagePath := filepath.Join(searchDir, "package.json")
	if _, err := os.Stat(packagePath); err == nil {
		// Found package.json in the starting directory, use it
		return searchDir, nil
	}

	// Search upward for package.json
	currentDir := searchDir
	for {
		packagePath := filepath.Join(currentDir, "package.json")
		if _, err := os.Stat(packagePath); err == nil {
			// Found a package.json, use this directory
			return currentDir, nil
		}

		// Move up one directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir || currentDir == "/" {
			break
		}
		currentDir = parentDir
	}

	// If not found upward, search downward from original directory
	if info.IsDir() {
		var foundDir string
		_ = filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Continue walking
			}
			if info.Name() == "package.json" {
				foundDir = filepath.Dir(path)
				return io.EOF // Stop walking on first package.json found
			}
			return nil
		})

		if foundDir != "" {
			return foundDir, nil
		}
	}

	return "", fmt.Errorf("no package.json found in %s or parent directories", startPath)
}

func (j *JestPane) parseJestJSON(state *JestInstanceState, data []byte) {
	var result struct {
		TestResults []struct {
			Name             string `json:"name"`
			Status           string `json:"status"`
			AssertionResults []struct {
				Title           string   `json:"title"`
				Status          string   `json:"status"`
				FailureMessages []string `json:"failureMessages"`
				Location        struct {
					Line int `json:"line"`
				} `json:"location"`
			} `json:"assertionResults"`
		} `json:"testResults"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return
	}

	state.testResults = []TestResult{}
	state.failedFiles = []string{}
	failedSet := make(map[string]bool)

	for _, testFile := range result.TestResults {
		for _, assertion := range testFile.AssertionResults {
			tr := TestResult{
				FilePath: testFile.Name,
				TestName: assertion.Title,
				Status:   assertion.Status,
				Line:     assertion.Location.Line,
			}

			if assertion.Status == "failed" && len(assertion.FailureMessages) > 0 {
				tr.ErrorOutput = strings.Join(assertion.FailureMessages, "\n")
				if !failedSet[testFile.Name] {
					state.failedFiles = append(state.failedFiles, testFile.Name)
					failedSet[testFile.Name] = true
				}
			}

			state.testResults = append(state.testResults, tr)
		}
	}

	if len(state.failedFiles) > 0 && state.currentIndex == -1 {
		state.currentIndex = j.findFirstFailureIndex(state)
	}
}

func (j *JestPane) parseJestOutput(state *JestInstanceState, output string) {
	// Fallback parser for non-JSON output
	lines := strings.Split(output, "\n")
	currentFile := ""
	fileRegex := regexp.MustCompile(`(?:PASS|FAIL)\s+(.+\.(?:js|jsx|ts|tsx))`)
	testRegex := regexp.MustCompile(`\s*([✓✗])\s+(.+)`)

	state.testResults = []TestResult{}
	state.failedFiles = []string{}
	failedSet := make(map[string]bool)

	for _, line := range lines {
		if matches := fileRegex.FindStringSubmatch(line); matches != nil {
			currentFile = matches[1]
		} else if matches := testRegex.FindStringSubmatch(line); matches != nil && currentFile != "" {
			status := "passed"
			if matches[1] == "✗" {
				status = "failed"
				if !failedSet[currentFile] {
					state.failedFiles = append(state.failedFiles, currentFile)
					failedSet[currentFile] = true
				}
			}

			state.testResults = append(state.testResults, TestResult{
				FilePath: currentFile,
				TestName: matches[2],
				Status:   status,
			})
		}
	}

	if len(state.failedFiles) > 0 && state.currentIndex == -1 {
		state.currentIndex = j.findFirstFailureIndex(state)
	}
}

func (j *JestPane) findFirstFailureIndex(state *JestInstanceState) int {
	for i, result := range state.testResults {
		if result.Status == "failed" {
			return i
		}
	}
	return -1
}

func (j *JestPane) ScrollUp() {
	j.viewport.LineUp(3)
}

func (j *JestPane) ScrollDown() {
	j.viewport.LineDown(3)
}

func (j *JestPane) NextFailure() {
	// Navigation disabled when showing raw output
	return
}

func (j *JestPane) PreviousFailure() {
	// Navigation disabled when showing raw output
	return
}

func (j *JestPane) scrollToCurrentIndex(state *JestInstanceState) {
	if state.currentIndex < 0 || state.currentIndex >= len(state.testResults) {
		return
	}

	// Calculate approximate line number for the current test
	lineCount := 0
	for i := 0; i <= state.currentIndex; i++ {
		if i == 0 || state.testResults[i].FilePath != state.testResults[i-1].FilePath {
			lineCount += 2 // File header + blank line
		}
		lineCount++ // Test line
		if state.testResults[i].Status == "failed" && state.testResults[i].ErrorOutput != "" {
			lineCount += strings.Count(state.testResults[i].ErrorOutput, "\n") + 1
		}
	}

	// Scroll to make the line visible
	j.viewport.SetYOffset(lineCount - j.viewport.Height/2)
}

func (j *JestPane) OpenCurrentInIDE() error {
	// Disabled when showing raw output - files are auto-opened on test failure
	return nil
}

func (j *JestPane) ResetToNormalMode() {
	j.viewport.SetYOffset(0)
}

func getIDECommand() string {
	// Check CLAUDE.md configuration
	claudeMdPath := "CLAUDE.md"
	if data, err := os.ReadFile(claudeMdPath); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		inConfig := false
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "[claude-squad]" {
				inConfig = true
				continue
			}
			if inConfig && strings.HasPrefix(line, "ide_command:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "ide_command:"))
			}
			if inConfig && strings.TrimSpace(line) == "" {
				break
			}
		}
	}

	// Check .claude-squad/config.json
	if data, err := os.ReadFile(".claude-squad/config.json"); err == nil {
		var config map[string]string
		if err := json.Unmarshal(data, &config); err == nil {
			if cmd, ok := config["ide_command"]; ok {
				return cmd
			}
		}
	}

	// Check global config
	homeDir, _ := os.UserHomeDir()
	globalConfigPath := filepath.Join(homeDir, ".claude-squad", "config.json")
	if data, err := os.ReadFile(globalConfigPath); err == nil {
		var config map[string]string
		if err := json.Unmarshal(data, &config); err == nil {
			if cmd, ok := config["default_ide_command"]; ok {
				return cmd
			}
		}
	}

	// Default to code
	return "code"
}

// Add styles used by Jest pane
var (
	fileHeaderStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("cyan")).
		MarginTop(1)

	selectedStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("237")).
		Foreground(lipgloss.Color("white"))

	errorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("red"))

	successStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("green"))

	failureStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("red")).
		Bold(true)
)
