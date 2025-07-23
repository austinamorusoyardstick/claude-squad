// +build ignore

package main

import (
	"fmt"
	"claude-squad/ui"
)

func main() {
	testContent := `# Markdown Rendering Test

This shows the improved **markdown** rendering with *cleaner* formatting.

## Code Examples

Simple inline ` + "`code`" + ` looks clean now.

### Code Block

` + "```go" + `
func main() {
    // This is a comment
    fmt.Println("Hello, World!")
    
    for i := 0; i < 10; i++ {
        fmt.Printf("Number: %d\n", i)
    }
}
` + "```" + `

## Tables

### Simple Table

| Name     | Age | City       |
|----------|-----|------------|
| Alice    | 30  | New York   |
| Bob      | 25  | San Francisco |
| Charlie  | 35  | Seattle    |

### Table without Header

| Row 1 Col 1 | Row 1 Col 2 |
| Row 2 Col 1 | Row 2 Col 2 |
| Row 3 Col 1 | Row 3 Col 2 |

## Lists

- First item
- Second item
  - Nested item
  - Another nested
- Third item

## Other Features

> This is a blockquote
> with multiple lines

Here's a [link](https://example.com) and ~~strikethrough~~ text.

---

End of test`

	rendered := ui.RenderMarkdownLight(testContent)
	fmt.Println(rendered)
}