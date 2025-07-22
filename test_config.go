package main

import (
	"claude-squad/config"
	"fmt"
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
	
	// Test that .claude-squad/config.json takes precedence over CLAUDE.md
	fmt.Printf("\n=== Priority Test ===\n")
	fmt.Printf("Should use 'cursor' from .claude-squad/config.json, not 'code' from CLAUDE.md\n")
	fmt.Printf("Should use 'meld' from .claude-squad/config.json, not 'code --diff' from CLAUDE.md\n")
}