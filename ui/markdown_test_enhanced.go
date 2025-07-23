package ui

import (
	"fmt"
	"testing"
)

func TestRenderMarkdownLightEnhanced(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "Headers",
			input: `# Header 1
## Header 2
### Header 3
#### Header 4
##### Header 5
###### Header 6`,
		},
		{
			name: "Emphasis",
			input: `This is **bold text** and this is *italic text*.
You can also use __bold__ and _italic_ like this.
And here's ~~strikethrough~~ text.`,
		},
		{
			name: "Lists",
			input: `Unordered list:
- First item
- Second item
  - Nested item
  - Another nested
    - Deep nested
- Third item

Ordered list:
1. First item
2. Second item
   1. Nested item
   2. Another nested
3. Third item`,
		},
		{
			name: "Links and Images",
			input: `Here's a [link to GitHub](https://github.com).
And here's an ![alt text](image.png) image.`,
		},
		{
			name: "Code",
			input: "Inline `code` and a code block:\n\n```go\nfunc main() {\n    fmt.Println(\"Hello, World!\")\n}\n```",
		},
		{
			name: "Blockquotes",
			input: `> This is a blockquote
> with multiple lines
> 
> And it can contain **bold** and *italic* text.`,
		},
		{
			name: "Horizontal Rules",
			input: `Text above

---

Text below

***

Another separator

___

End`,
		},
		{
			name: "Tables",
			input: `| Header 1 | Header 2 | Header 3 |
|----------|----------|----------|
| Cell 1   | Cell 2   | Cell 3   |
| Cell 4   | Cell 5   | Cell 6   |`,
		},
		{
			name: "Mixed Content",
			input: `# Project README

This project demonstrates **enhanced markdown** rendering with *various* features.

## Features

- **Bold** and *italic* text
- ~~Strikethrough~~ support
- `Inline code` blocks
- [Links](https://example.com)

### Code Example

` + "```python" + `
def hello():
    print("Hello, World!")
` + "```" + `

> **Note:** This is a blockquote with **bold** text.

## Table

| Feature | Status | Notes |
|---------|--------|-------|
| Headers | ✓ | All levels |
| Lists | ✓ | Nested too |
| Tables | ✓ | Simple |

---

Created with ❤️`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RenderMarkdownLight(tt.input)
			// Just print the output for visual inspection
			fmt.Printf("\n=== %s ===\n%s\n", tt.name, result)
			
			// Basic check that we got some output
			if len(result) == 0 {
				t.Errorf("RenderMarkdownLight() returned empty string")
			}
		})
	}
}