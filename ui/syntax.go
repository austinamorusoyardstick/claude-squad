package ui

import (
	"regexp"
	"strings"
	
	"github.com/charmbracelet/lipgloss"
)

// JavaScript syntax patterns
var (
	// Keywords
	jsKeywordRegex = regexp.MustCompile(`\b(async|await|break|case|catch|class|const|continue|debugger|default|delete|do|else|export|extends|finally|for|function|if|import|in|instanceof|let|new|of|return|super|switch|this|throw|try|typeof|var|void|while|with|yield)\b`)
	
	// Literals
	jsStringRegex   = regexp.MustCompile(`("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|` + "`" + `(?:[^` + "`" + `\\]|\\.)*` + "`" + `)`)
	jsNumberRegex   = regexp.MustCompile(`\b(\d+\.?\d*([eE][+-]?\d+)?|0x[0-9a-fA-F]+|0b[01]+|0o[0-7]+)\b`)
	jsBooleanRegex  = regexp.MustCompile(`\b(true|false|null|undefined|NaN|Infinity)\b`)
	
	// Comments
	jsSingleCommentRegex = regexp.MustCompile(`//.*$`)
	jsMultiCommentRegex  = regexp.MustCompile(`/\*[\s\S]*?\*/`)
	
	// Functions and methods
	jsFunctionRegex = regexp.MustCompile(`\b([a-zA-Z_$][a-zA-Z0-9_$]*)\s*\(`)
	
	// Common objects and methods
	jsBuiltinRegex = regexp.MustCompile(`\b(console|document|window|Array|Object|String|Number|Boolean|Date|Math|JSON|Promise|Map|Set|Symbol|Error|RegExp)\b`)
	
	// Operators
	jsOperatorRegex = regexp.MustCompile(`(===|!==|==|!=|<=|>=|<|>|\+=|-=|\*=|\/=|%=|&&|\|\||!|\+\+|--|=>)`)
)

// Syntax highlighting styles
var (
	keywordStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("204")) // Pink
	stringStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))  // Green
	numberStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("209")) // Orange
	commentStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("243")) // Gray
	functionStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))  // Blue
	builtinStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // Yellow
	operatorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203")) // Red
	defaultStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252")) // Light gray
)

// HighlightJavaScript applies syntax highlighting to JavaScript code
func HighlightJavaScript(code string) string {
	lines := strings.Split(code, "\n")
	highlightedLines := make([]string, len(lines))
	
	for i, line := range lines {
		highlightedLines[i] = highlightJSLine(line)
	}
	
	return strings.Join(highlightedLines, "\n")
}

// highlightJSLine highlights a single line of JavaScript code
func highlightJSLine(line string) string {
	if strings.TrimSpace(line) == "" {
		return line
	}
	
	// Track what has been highlighted to avoid overlaps
	highlighted := make([]bool, len(line))
	result := make([]string, len(line))
	
	// Helper to apply highlighting
	applyHighlight := func(regex *regexp.Regexp, style lipgloss.Style, groupIndex int) {
		matches := regex.FindAllStringSubmatchIndex(line, -1)
		for _, match := range matches {
			start := match[groupIndex*2]
			end := match[groupIndex*2+1]
			
			// Check if already highlighted
			alreadyHighlighted := false
			for j := start; j < end; j++ {
				if highlighted[j] {
					alreadyHighlighted = true
					break
				}
			}
			
			if !alreadyHighlighted {
				styledText := style.Render(line[start:end])
				// Mark as highlighted
				for j := start; j < end; j++ {
					highlighted[j] = true
				}
				// Store the styled text at the start position
				result[start] = styledText
				// Mark other positions as handled
				for j := start + 1; j < end; j++ {
					result[j] = ""
				}
			}
		}
	}
	
	// Apply highlighting in order of precedence
	// 1. Comments (highest precedence)
	applyHighlight(jsSingleCommentRegex, commentStyle, 0)
	applyHighlight(jsMultiCommentRegex, commentStyle, 0)
	
	// 2. Strings
	applyHighlight(jsStringRegex, stringStyle, 0)
	
	// 3. Numbers
	applyHighlight(jsNumberRegex, numberStyle, 0)
	
	// 4. Booleans and special values
	applyHighlight(jsBooleanRegex, numberStyle, 0)
	
	// 5. Keywords
	applyHighlight(jsKeywordRegex, keywordStyle, 0)
	
	// 6. Built-in objects
	applyHighlight(jsBuiltinRegex, builtinStyle, 0)
	
	// 7. Functions (group 1 is the function name)
	applyHighlight(jsFunctionRegex, functionStyle, 1)
	
	// 8. Operators
	applyHighlight(jsOperatorRegex, operatorStyle, 0)
	
	// Build the final result
	var finalResult strings.Builder
	for i, char := range line {
		if !highlighted[i] {
			finalResult.WriteString(defaultStyle.Render(string(char)))
		} else if result[i] != "" {
			finalResult.WriteString(result[i])
		}
	}
	
	return finalResult.String()
}

// HighlightCode applies syntax highlighting based on the language
func HighlightCode(code, language string) string {
	switch strings.ToLower(language) {
	case "javascript", "js", "jsx":
		return HighlightJavaScript(code)
	case "typescript", "ts", "tsx":
		// TypeScript is similar to JavaScript, so use the same highlighter
		return HighlightJavaScript(code)
	default:
		// No highlighting for other languages yet
		return code
	}
}