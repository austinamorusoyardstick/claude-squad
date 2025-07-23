// +build ignore

package main

import (
	"fmt"
	"claude-squad/ui"
)

func main() {
	testCases := []string{
		// Basic table
		`| Header 1 | Header 2 | Header 3 |
|----------|----------|----------|
| Cell 1   | Cell 2   | Cell 3   |
| Cell 4   | Cell 5   | Cell 6   |`,

		// Table with alignment
		`| Left | Center | Right |
|:-----|:------:|------:|
| L1   | C1     | R1    |
| L2   | C2     | R2    |`,

		// Table without separator
		`| Header 1 | Header 2 |
| Cell 1   | Cell 2   |
| Cell 3   | Cell 4   |`,

		// Table with varying content lengths
		`| Short | Very Long Header | Mid |
|-------|------------------|-----|
| A | This is a very long cell content | OK |
| B | Short | Medium length |`,

		// Single column table
		`| Single Column |
|---------------|
| Row 1         |
| Row 2         |`,

		// Empty cells
		`| Col1 | Col2 | Col3 |
|------|------|------|
| A    |      | C    |
|      | B    |      |`,
	}

	for i, table := range testCases {
		fmt.Printf("\n=== Test Case %d ===\n", i+1)
		fmt.Println("Input:")
		fmt.Println(table)
		fmt.Println("\nRendered:")
		rendered := ui.RenderMarkdownLight(table)
		fmt.Println(rendered)
		fmt.Println()
	}
}