package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// CommandLog represents a logged command execution
type CommandLog struct {
	Timestamp time.Time
	Command   string
	Args      []string
	Dir       string
	Source    string // Where the command was executed from
}

// LogPane displays command execution logs
type LogPane struct {
	logs        []CommandLog
	viewport    viewport.Model
	width       int
	height      int
	mu          sync.RWMutex
	isScrolling bool // Track if user is manually scrolling
}

// NewLogPane creates a new log pane
func NewLogPane() *LogPane {
	return &LogPane{
		logs:     make([]CommandLog, 0),
		viewport: viewport.New(0, 0),
	}
}

// AddLog adds a new command log entry
func (p *LogPane) AddLog(cmd string, args []string, dir string, source string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	log := CommandLog{
		Timestamp: time.Now(),
		Command:   cmd,
		Args:      args,
		Dir:       dir,
		Source:    source,
	}
	p.logs = append(p.logs, log)
	
	// Only update viewport if not scrolling
	if !p.isScrolling {
		p.updateViewport()
	}
}

// SetSize updates the size of the log pane
func (p *LogPane) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.viewport.Width = width
	p.viewport.Height = height
	p.updateViewport()
}

// ScrollUp scrolls the viewport up
func (p *LogPane) ScrollUp() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// Enable scroll mode when user scrolls
	p.isScrolling = true
	// Make sure content is updated before scrolling
	p.updateViewportNoScroll()
	p.viewport.LineUp(3)
}

// ScrollDown scrolls the viewport down
func (p *LogPane) ScrollDown() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// Enable scroll mode when user scrolls
	p.isScrolling = true
	// Make sure content is updated before scrolling
	p.updateViewportNoScroll()
	p.viewport.LineDown(3)
	
	// If we're at the bottom, disable scroll mode
	if p.viewport.AtBottom() {
		p.isScrolling = false
	}
}

// updateViewport updates the viewport content and scrolls to bottom
func (p *LogPane) updateViewport() {
	content := p.renderLogs()
	p.viewport.SetContent(content)
	p.viewport.GotoBottom()
}

// updateViewportNoScroll updates the viewport content without changing scroll position
func (p *LogPane) updateViewportNoScroll() {
	// Save current position
	yOffset := p.viewport.YOffset
	
	// Update content
	content := p.renderLogs()
	p.viewport.SetContent(content)
	
	// Restore position
	p.viewport.YOffset = yOffset
}

// renderLogs renders all logs as a string
func (p *LogPane) renderLogs() string {
	if len(p.logs) == 0 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("No commands executed yet")
	}

	var builder strings.Builder
	timestampStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	commandStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("cyan")).Bold(true)
	argsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("white"))
	dirStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("yellow"))
	sourceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("magenta"))

	for i := len(p.logs) - 1; i >= 0; i-- { // Show newest first
		log := p.logs[i]
		timestamp := log.Timestamp.Format("15:04:05")
		
		// Format command with arguments
		cmdLine := commandStyle.Render(log.Command)
		if len(log.Args) > 0 {
			cmdLine += " " + argsStyle.Render(strings.Join(log.Args, " "))
		}

		// Build the log entry
		entry := fmt.Sprintf("%s [%s] %s",
			timestampStyle.Render(timestamp),
			sourceStyle.Render(log.Source),
			cmdLine,
		)

		if log.Dir != "" {
			entry += fmt.Sprintf("\n      %s %s",
				timestampStyle.Render("in"),
				dirStyle.Render(log.Dir),
			)
		}

		builder.WriteString(entry + "\n")
		if i > 0 {
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

// Clear clears all logs
func (p *LogPane) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logs = make([]CommandLog, 0)
	p.updateViewport()
}

// String returns the string representation of the log pane
func (p *LogPane) String() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// Update content if we have new logs
	if p.isScrolling {
		p.updateViewportNoScroll()
	} else {
		p.updateViewport()
	}
	
	return p.viewport.View()
}

// ResetScroll resets the scroll mode
func (p *LogPane) ResetScroll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.isScrolling = false
	p.updateViewport()
}