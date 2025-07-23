package ui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/keiko-app/claude-squad/session"
)

type JestPane struct {
	width        int
	height       int
	viewport     viewport.Model
	content      string
	running      bool
	testResults  []TestResult
	failedFiles  []string
	workingDir   string
	currentIndex int
	mu           sync.Mutex
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
		viewport:     vp,
		testResults:  []TestResult{},
		failedFiles:  []string{},
		currentIndex: -1,
	}
}

func (j *JestPane) SetSize(width, height int) {
	j.width = width
	j.height = height
	j.viewport.Width = width
	j.viewport.Height = height - 4 // Leave room for header and status
	j.viewport.SetContent(j.formatContent())
}

func (j *JestPane) String() string {
	if j.height < 5 {
		return ""
	}

	header := titleStyle.Render("Jest Test Runner")
	
	var status string
	if j.running {
		status = statusStyle.Render("⏳ Running tests...")
	} else if len(j.failedFiles) > 0 {
		status = failureStyle.Render(fmt.Sprintf("❌ %d test(s) failed", len(j.failedFiles)))
	} else if len(j.testResults) > 0 {
		status = successStyle.Render("✅ All tests passed")
	} else {
		status = statusStyle.Render("No tests run yet")
	}

	content := j.viewport.View()
	
	help := helpStyle.Render("↑/↓: scroll • n/p: next/prev failure • Enter: open in IDE • r: rerun • ESC: exit scroll mode")
	
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		status,
		content,
		help,
	)
}

func (j *JestPane) formatContent() string {
	if len(j.testResults) == 0 {
		return dimStyle.Render("No test results to display.\nPress 'r' to run tests.")
	}

	var buf strings.Builder
	currentFile := ""
	
	for i, result := range j.testResults {
		if result.FilePath != currentFile {
			if currentFile != "" {
				buf.WriteString("\n")
			}
			currentFile = result.FilePath
			relPath, _ := filepath.Rel(j.workingDir, result.FilePath)
			if relPath == "" {
				relPath = result.FilePath
			}
			buf.WriteString(fileHeaderStyle.Render(relPath))
			buf.WriteString("\n")
		}
		
		var statusSymbol, statusColor string
		switch result.Status {
		case "passed":
			statusSymbol = "✓"
			statusColor = "green"
		case "failed":
			statusSymbol = "✗"
			statusColor = "red"
		case "skipped":
			statusSymbol = "○"
			statusColor = "yellow"
		}
		
		testLine := fmt.Sprintf("  %s %s", statusSymbol, result.TestName)
		if result.Status == "failed" && i == j.currentIndex {
			testLine = selectedStyle.Render(testLine)
		} else {
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor))
			testLine = style.Render(testLine)
		}
		buf.WriteString(testLine)
		buf.WriteString("\n")
		
		if result.Status == "failed" && result.ErrorOutput != "" {
			errorLines := strings.Split(result.ErrorOutput, "\n")
			for _, line := range errorLines {
				if strings.TrimSpace(line) != "" {
					buf.WriteString(errorStyle.Render("    " + line))
					buf.WriteString("\n")
				}
			}
		}
	}
	
	return buf.String()
}

func (j *JestPane) RunTests(instance *session.Instance) error {
	j.mu.Lock()
	j.running = true
	j.testResults = []TestResult{}
	j.failedFiles = []string{}
	j.currentIndex = -1
	j.mu.Unlock()
	
	// Find Jest working directory
	workDir, err := j.findJestWorkingDir(instance.Path)
	if err != nil {
		j.mu.Lock()
		j.running = false
		j.content = errorStyle.Render(fmt.Sprintf("Error finding Jest: %v", err))
		j.viewport.SetContent(j.content)
		j.mu.Unlock()
		return err
	}
	
	j.workingDir = workDir
	
	// Run Jest with JSON reporter
	cmd := exec.Command("npx", "jest", "--json", "--outputFile=/tmp/jest-results.json")
	cmd.Dir = workDir
	
	// Capture both stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	// Run the command
	_ = cmd.Run() // Jest returns non-zero on test failures, which is expected
	
	// Parse the JSON results
	jsonData, err := os.ReadFile("/tmp/jest-results.json")
	if err != nil {
		// Fallback to parsing stdout if JSON file not created
		j.parseJestOutput(stdout.String(), stderr.String())
	} else {
		j.parseJestJSON(jsonData)
	}
	
	j.mu.Lock()
	j.running = false
	j.viewport.SetContent(j.formatContent())
	j.mu.Unlock()
	
	return nil
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
	
	// Search upward for package.json with Jest
	currentDir := searchDir
	for {
		packagePath := filepath.Join(currentDir, "package.json")
		if _, err := os.Stat(packagePath); err == nil {
			// Check if this package.json has Jest
			data, err := os.ReadFile(packagePath)
			if err == nil {
				var pkg map[string]interface{}
				if err := json.Unmarshal(data, &pkg); err == nil {
					// Check for Jest in dependencies or devDependencies
					if deps, ok := pkg["dependencies"].(map[string]interface{}); ok {
						if _, hasJest := deps["jest"]; hasJest {
							return currentDir, nil
						}
					}
					if devDeps, ok := pkg["devDependencies"].(map[string]interface{}); ok {
						if _, hasJest := devDeps["jest"]; hasJest {
							return currentDir, nil
						}
					}
					// Also check for Jest config
					if _, hasJestConfig := pkg["jest"]; hasJestConfig {
						return currentDir, nil
					}
				}
			}
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
		err := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Continue walking
			}
			if info.Name() == "package.json" {
				data, err := os.ReadFile(path)
				if err == nil {
					var pkg map[string]interface{}
					if err := json.Unmarshal(data, &pkg); err == nil {
						// Check for Jest
						if deps, ok := pkg["dependencies"].(map[string]interface{}); ok {
							if _, hasJest := deps["jest"]; hasJest {
								foundDir = filepath.Dir(path)
								return io.EOF // Stop walking
							}
						}
						if devDeps, ok := pkg["devDependencies"].(map[string]interface{}); ok {
							if _, hasJest := devDeps["jest"]; hasJest {
								foundDir = filepath.Dir(path)
								return io.EOF // Stop walking
							}
						}
						if _, hasJestConfig := pkg["jest"]; hasJestConfig {
							foundDir = filepath.Dir(path)
							return io.EOF // Stop walking
						}
					}
				}
			}
			return nil
		})
		
		if foundDir != "" {
			return foundDir, nil
		}
	}
	
	return "", fmt.Errorf("no Jest configuration found")
}

func (j *JestPane) parseJestJSON(data []byte) {
	var result struct {
		TestResults []struct {
			Name           string `json:"name"`
			Status         string `json:"status"`
			AssertionResults []struct {
				Title    string   `json:"title"`
				Status   string   `json:"status"`
				FailureMessages []string `json:"failureMessages"`
				Location struct {
					Line int `json:"line"`
				} `json:"location"`
			} `json:"assertionResults"`
		} `json:"testResults"`
	}
	
	if err := json.Unmarshal(data, &result); err != nil {
		return
	}
	
	j.testResults = []TestResult{}
	j.failedFiles = []string{}
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
					j.failedFiles = append(j.failedFiles, testFile.Name)
					failedSet[testFile.Name] = true
				}
			}
			
			j.testResults = append(j.testResults, tr)
		}
	}
	
	if len(j.failedFiles) > 0 && j.currentIndex == -1 {
		j.currentIndex = j.findFirstFailureIndex()
	}
}

func (j *JestPane) parseJestOutput(stdout, stderr string) {
	// Fallback parser for non-JSON output
	lines := strings.Split(stdout+"\n"+stderr, "\n")
	currentFile := ""
	fileRegex := regexp.MustCompile(`(?:PASS|FAIL)\s+(.+\.(?:js|jsx|ts|tsx))`)
	testRegex := regexp.MustCompile(`\s*([✓✗])\s+(.+)`)
	
	j.testResults = []TestResult{}
	j.failedFiles = []string{}
	failedSet := make(map[string]bool)
	
	for _, line := range lines {
		if matches := fileRegex.FindStringSubmatch(line); matches != nil {
			currentFile = matches[1]
		} else if matches := testRegex.FindStringSubmatch(line); matches != nil && currentFile != "" {
			status := "passed"
			if matches[1] == "✗" {
				status = "failed"
				if !failedSet[currentFile] {
					j.failedFiles = append(j.failedFiles, currentFile)
					failedSet[currentFile] = true
				}
			}
			
			j.testResults = append(j.testResults, TestResult{
				FilePath: currentFile,
				TestName: matches[2],
				Status:   status,
			})
		}
	}
	
	if len(j.failedFiles) > 0 && j.currentIndex == -1 {
		j.currentIndex = j.findFirstFailureIndex()
	}
}

func (j *JestPane) findFirstFailureIndex() int {
	for i, result := range j.testResults {
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
	if len(j.failedFiles) == 0 {
		return
	}
	
	startIdx := j.currentIndex + 1
	for i := startIdx; i < len(j.testResults); i++ {
		if j.testResults[i].Status == "failed" {
			j.currentIndex = i
			j.viewport.SetContent(j.formatContent())
			j.scrollToCurrentIndex()
			return
		}
	}
	
	// Wrap around to beginning
	for i := 0; i < startIdx && i < len(j.testResults); i++ {
		if j.testResults[i].Status == "failed" {
			j.currentIndex = i
			j.viewport.SetContent(j.formatContent())
			j.scrollToCurrentIndex()
			return
		}
	}
}

func (j *JestPane) PreviousFailure() {
	if len(j.failedFiles) == 0 {
		return
	}
	
	startIdx := j.currentIndex - 1
	for i := startIdx; i >= 0; i-- {
		if j.testResults[i].Status == "failed" {
			j.currentIndex = i
			j.viewport.SetContent(j.formatContent())
			j.scrollToCurrentIndex()
			return
		}
	}
	
	// Wrap around to end
	for i := len(j.testResults) - 1; i > startIdx && i >= 0; i-- {
		if j.testResults[i].Status == "failed" {
			j.currentIndex = i
			j.viewport.SetContent(j.formatContent())
			j.scrollToCurrentIndex()
			return
		}
	}
}

func (j *JestPane) scrollToCurrentIndex() {
	if j.currentIndex < 0 || j.currentIndex >= len(j.testResults) {
		return
	}
	
	// Calculate approximate line number for the current test
	lineCount := 0
	for i := 0; i <= j.currentIndex; i++ {
		if i == 0 || j.testResults[i].FilePath != j.testResults[i-1].FilePath {
			lineCount += 2 // File header + blank line
		}
		lineCount++ // Test line
		if j.testResults[i].Status == "failed" && j.testResults[i].ErrorOutput != "" {
			lineCount += strings.Count(j.testResults[i].ErrorOutput, "\n") + 1
		}
	}
	
	// Scroll to make the line visible
	j.viewport.SetYOffset(lineCount - j.viewport.Height/2)
}

func (j *JestPane) OpenCurrentInIDE() error {
	if j.currentIndex < 0 || j.currentIndex >= len(j.testResults) {
		return fmt.Errorf("no test selected")
	}
	
	result := j.testResults[j.currentIndex]
	if result.Status != "failed" {
		return fmt.Errorf("selected test did not fail")
	}
	
	// Open the file in IDE
	ideCmd := getIDECommand()
	if ideCmd == "" {
		return fmt.Errorf("no IDE command configured")
	}
	
	// Format: "code file:line" or just "code file"
	var cmd *exec.Cmd
	if result.Line > 0 {
		cmd = exec.Command(ideCmd, fmt.Sprintf("%s:%d", result.FilePath, result.Line))
	} else {
		cmd = exec.Command(ideCmd, result.FilePath)
	}
	
	return cmd.Start()
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