// +build ignore

package main

import (
	"fmt"
	"claude-squad/ui"
)

func main() {
	testContent := `## Code Example

Here's some code:

` + "```go" + `
func main() {
    fmt.Println("Hello, World!")
    if true {
        fmt.Println("This is indented")
    }
}
` + "```" + `

And inline ` + "`code`" + ` too.

## Table Example

| Column 1 | Column 2 | Column 3 |
|----------|----------|----------|
| Data 1   | Data 2   | Data 3   |
| More     | Data     | Here     |

## Another Code Block

` + "```python" + `
def hello():
    print("Hello from Python")
    return True
` + "```" + `

That's all!`

	rendered := ui.RenderMarkdownLight(testContent)
	fmt.Println(rendered)
}