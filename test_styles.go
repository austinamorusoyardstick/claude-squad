// +build ignore

package main

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
)

func main() {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("204"))
	text := style.Render("const")
	fmt.Printf("Styled text: %q\n", text)
	fmt.Printf("Length: %d\n", len(text))
	
	// Check if it contains ANSI codes
	for i, b := range []byte(text) {
		fmt.Printf("Byte %d: %d (%c)\n", i, b, b)
	}
}