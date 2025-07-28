package ui

import (
	"fmt"
	"sort"
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
	logs         []CommandLog
	viewport     viewport.Model
	width        int
	height       int
	mu           sync.RWMutex
	isScrolling  bool // Track if user is manually scrolling
	showDistinct bool // Show only distinct commands
	sortByCommand bool // Sort by command instead of date
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

	// Check if this exact command already exists
	if p.showDistinct {
		for i, existingLog := range p.logs {
			if p.getCommandKey(existingLog) == p.getCommandKey(CommandLog{Command: cmd, Args: args, Dir: dir}) {
				// Update timestamp of existing command
				p.logs[i].Timestamp = time.Now()
				// Only update viewport if not scrolling
				if !p.isScrolling {
					p.updateViewport()
				}
				return
			}
		}
	}

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
	countStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("green"))

	// Show mode indicators
	modes := []string{}
	if p.showDistinct {
		modes = append(modes, "Distinct")
	}
	if p.sortByCommand {
		modes = append(modes, "Sorted")
	}
	if len(modes) > 0 {
		builder.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("yellow")).
			Bold(true).
			Render(fmt.Sprintf("[%s Mode - 'u': toggle distinct, 's': toggle sort]", strings.Join(modes, ", "))))
		builder.WriteString("\n\n")
	}

	logsToRender := make([]CommandLog, len(p.logs))
	copy(logsToRender, p.logs)

	// Apply sorting if enabled
	if p.sortByCommand {
		sort.Slice(logsToRender, func(i, j int) bool {
			// First sort by command
			if logsToRender[i].Command != logsToRender[j].Command {
				return logsToRender[i].Command < logsToRender[j].Command
			}
			// Then by args
			argsI := strings.Join(logsToRender[i].Args, " ")
			argsJ := strings.Join(logsToRender[j].Args, " ")
			if argsI != argsJ {
				return argsI < argsJ
			}
			// Then by directory
			if logsToRender[i].Dir != logsToRender[j].Dir {
				return logsToRender[i].Dir < logsToRender[j].Dir
			}
			// Finally by timestamp (most recent first)
			return logsToRender[i].Timestamp.After(logsToRender[j].Timestamp)
		})
	} else {
		// Default: newest first
		sort.Slice(logsToRender, func(i, j int) bool {
			return logsToRender[i].Timestamp.After(logsToRender[j].Timestamp)
		})
	}

	for i, log := range logsToRender {
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

		// In distinct mode, show count
		if p.showDistinct {
			count := p.getCommandCount(log)
			if count > 1 {
				entry += " " + countStyle.Render(fmt.Sprintf("(×%d)", count))
			}
		}

		if log.Dir != "" {
			entry += fmt.Sprintf("\n      %s %s",
				timestampStyle.Render("in"),
				dirStyle.Render(log.Dir),
			)
		}

		builder.WriteString(entry + "\n")
		if i < len(logsToRender)-1 {
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

// ToggleDistinct toggles showing only distinct commands
func (p *LogPane) ToggleDistinct() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.showDistinct = !p.showDistinct
	p.updateViewport()
}

// ToggleSort toggles sorting by command
func (p *LogPane) ToggleSort() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sortByCommand = !p.sortByCommand
	p.updateViewport()
}

// getDistinctLogs is no longer needed as we handle distinct mode differently now

// getCommandKey returns a unique key for a command
func (p *LogPane) getCommandKey(log CommandLog) string {
	return fmt.Sprintf("%s|%s|%s", log.Command, strings.Join(log.Args, " "), log.Dir)
}

// getCommandCount returns how many times this command has been executed
func (p *LogPane) getCommandCount(target CommandLog) int {
	key := p.getCommandKey(target)
	count := 0
	
	for _, log := range p.logs {
		if p.getCommandKey(log) == key {
			count++
		}
	}
	
	return count
}

// PageUp scrolls up by a page
func (p *LogPane) PageUp() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.isScrolling = true
	p.updateViewportNoScroll()
	p.viewport.HalfViewUp()
}

// PageDown scrolls down by a page  
func (p *LogPane) PageDown() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.isScrolling = true
	p.updateViewportNoScroll()
	p.viewport.HalfViewDown()
	
	if p.viewport.AtBottom() {
		p.isScrolling = false
	}
}