package main

import (
	"claude-squad/config"
	"fmt"
	"strings"
)

func main() {
	// Test current directory configuration
	cwd := "."
	globalConfig := config.LoadConfig()
	
	fmt.Printf("=== Configuration Test ===\n")
	fmt.Printf("Global config default IDE: %s\n", globalConfig.DefaultIdeCommand)
	fmt.Printf("Global config default diff: %s\n", globalConfig.DefaultDiffCommand)
	
	fmt.Printf("\n=== Per-repo Configuration ===\n")
	repoConfig := config.LoadRepoConfig(cwd)
	fmt.Printf("Repo IDE command: %s\n", repoConfig.IdeCommand)
	fmt.Printf("Repo diff command: %s\n", repoConfig.DiffCommand)
	
	fmt.Printf("\n=== Effective Configuration ===\n")
	effectiveIde := config.GetEffectiveIdeCommand(cwd, globalConfig)
	effectiveDiff := config.GetEffectiveDiffCommand(cwd, globalConfig)
	fmt.Printf("Effective IDE command: %s\n", effectiveIde)
	fmt.Printf("Effective diff command: %s\n", effectiveDiff)
	
	fmt.Printf("\n=== Command Parsing Test ===\n")
	// Test the command parsing logic that was fixed
	diffCommand := "code --diff"
	parts := strings.Fields(diffCommand)
	if len(parts) > 0 {
		fmt.Printf("Parsed command: %s\n", parts[0])
		if len(parts) > 1 {
			fmt.Printf("Parsed args: %v\n", parts[1:])
		}
	}
}