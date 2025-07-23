// +build ignore

package main

import (
	"fmt"
	"claude-squad/ui"
)

func main() {
	testContent := `# Enhanced Markdown Test

This demonstrates **enhanced markdown** rendering with *various* features.

## Features

- **Bold** and *italic* text
- ~~Strikethrough~~ support
- ` + "`Inline code`" + ` blocks
- [Links](https://example.com)

### Code Example

` + "```go" + `
func hello() {
    fmt.Println("Hello, World!")
}
` + "```" + `

> **Note:** This is a blockquote with **bold** text.
> It can span multiple lines.

## Lists

Unordered:
- First item
- Second item
  - Nested item
  - Another nested
    - Deep nested

Ordered:
1. First item
2. Second item
   1. Nested item

## Table

| Feature | Status | Notes |
|---------|--------|-------|
| Headers | ✓ | All levels |
| Lists | ✓ | Nested too |
| Tables | ✓ | Simple |

---

Created with ❤️`

	rendered := ui.RenderMarkdownLight(testContent)
	fmt.Println(rendered)
}