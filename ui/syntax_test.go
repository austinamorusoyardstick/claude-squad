package ui

import (
	"strings"
	"testing"
)

func TestHighlightJavaScript(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string // Strings that should be in the output (ANSI codes)
	}{
		{
			name:  "Keywords",
			input: "const x = 5; let y = 10;",
			contains: []string{
				"\x1b[38;5;204m", // Keyword color
				"const",
				"let",
			},
		},
		{
			name:  "Strings",
			input: `const msg = "Hello"; const name = 'World';`,
			contains: []string{
				"\x1b[38;5;82m", // String color
				`"Hello"`,
				`'World'`,
			},
		},
		{
			name:  "Numbers",
			input: "const a = 42; const b = 3.14;",
			contains: []string{
				"\x1b[38;5;209m", // Number color
				"42",
				"3.14",
			},
		},
		{
			name:  "Comments",
			input: "// This is a comment\nconst x = 5;",
			contains: []string{
				"\x1b[38;5;243m", // Comment color
				"// This is a comment",
			},
		},
		{
			name:  "Functions",
			input: "function test() { console.log('test'); }",
			contains: []string{
				"\x1b[38;5;204m", // Keyword (function)
				"\x1b[38;5;39m",  // Function name
				"\x1b[38;5;214m", // Built-in (console)
			},
		},
		{
			name:  "Boolean and null",
			input: "const a = true; const b = false; const c = null;",
			contains: []string{
				"\x1b[38;5;209m", // Number color (also used for booleans)
				"true",
				"false",
				"null",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HighlightJavaScript(tt.input)
			
			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected output to contain %q, but it didn't.\nGot: %s", expected, result)
				}
			}
		})
	}
}

func TestHighlightCode(t *testing.T) {
	// Test that JavaScript highlighting is applied
	jsCode := "const x = 42;"
	
	// Test various JavaScript-like language identifiers
	for _, lang := range []string{"javascript", "js", "jsx", "typescript", "ts", "tsx"} {
		result := HighlightCode(jsCode, lang)
		if !strings.Contains(result, "\x1b[") {
			t.Errorf("Expected highlighting for language %q, but got plain text: %s", lang, result)
		}
	}
	
	// Test that unknown languages return plain text
	result := HighlightCode(jsCode, "unknown")
	if strings.Contains(result, "\x1b[") {
		t.Errorf("Expected no highlighting for unknown language, but got: %s", result)
	}
}