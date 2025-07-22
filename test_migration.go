package main

import (
	"claude-squad/config"
	"fmt"
)

func main() {
	// Test that LoadConfig properly handles missing fields by using defaults
	globalConfig := config.LoadConfig()
	
	fmt.Printf("=== Backward Compatibility Test ===\n")
	fmt.Printf("Global config default IDE: '%s'\n", globalConfig.DefaultIdeCommand)
	fmt.Printf("Global config default diff: '%s'\n", globalConfig.DefaultDiffCommand)
	
	// When these are empty/missing, GetEffectiveIdeCommand should still work
	effectiveIde := config.GetEffectiveIdeCommand(".", globalConfig)
	effectiveDiff := config.GetEffectiveDiffCommand(".", globalConfig)
	
	fmt.Printf("\nFallback behavior:\n")
	fmt.Printf("Effective IDE command: '%s'\n", effectiveIde)
	fmt.Printf("Effective diff command: '%s'\n", effectiveDiff)
	
	fmt.Printf("\n=== Default Config Test ===\n")
	defaultConfig := config.DefaultConfig()
	fmt.Printf("Default IDE command: '%s'\n", defaultConfig.DefaultIdeCommand)
	fmt.Printf("Default diff command: '%s'\n", defaultConfig.DefaultDiffCommand)
}