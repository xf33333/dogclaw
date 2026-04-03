package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"dogclaw/pkg/agent"
)

func main() {
	// 演示 agent 加载器的基本用法
	cwd, _ := os.Getwd()
	fmt.Printf("Loading agents from: %s\n", cwd)

	loader := agent.NewConfigLoader(cwd)

	// 加载 agent 定义
	ctx := context.Background()
	result, err := loader.LoadAgents(ctx)
	if err != nil {
		fmt.Printf("Error loading agents: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✅ Loaded %d active agents (out of %d total)\n", len(result.ActiveAgents), len(result.AllAgents))

	// 显示失败的 files
	if len(result.FailedFiles) > 0 {
		fmt.Printf("\n⚠️  Failed to parse %d files:\n", len(result.FailedFiles))
		for _, f := range result.FailedFiles {
			fmt.Printf("  - %s: %s\n", filepath.Base(f.Path), f.Error)
		}
		fmt.Println()
	}

	// 显示所有内置 agent
	fmt.Println("Built-in Agents:")
	for _, a := range result.ActiveAgents {
		if a.GetSource() == agent.SourceBuiltIn {
			printAgentInfo(a)
		}
	}
	fmt.Println()

	// 显示自定义 agent
	customAgents := make([]agent.AgentDefinition, 0)
	for _, a := range result.ActiveAgents {
		if a.GetSource() != agent.SourceBuiltIn {
			customAgents = append(customAgents, a)
		}
	}
	if len(customAgents) > 0 {
		fmt.Println("Custom Agents:")
		for _, a := range customAgents {
			printAgentInfo(a)
		}
	} else {
		fmt.Println("No custom agents defined (create ~/.dogclaw/agents/ or ./.dogclaw/agents/ to add)")
	}
}

func printAgentInfo(a agent.AgentDefinition) {
	tools := a.GetTools()
	toolsDesc := "all tools"
	if tools != nil {
		toolsDesc = fmt.Sprintf("%d tools", len(tools))
		if len(tools) <= 3 {
			toolsDesc = fmt.Sprintf("%v", tools)
		}
	}

	memory := a.GetMemory()
	if memory == "" {
		memory = "none"
	}

	model := a.GetModel()
	if model == "" || model == "inherit" {
		model = "inherited"
	}

	fmt.Printf("  • %s (%s)\n", a.GetAgentType(), a.GetSource())
	fmt.Printf("    Description: %s\n", truncate(a.GetWhenToUse(), 80))
	fmt.Printf("    Tools: %s, Model: %s, Memory: %s\n", toolsDesc, model, memory)
	if a.GetMaxTurns() != nil {
		fmt.Printf("    Max Turns: %d\n", *a.GetMaxTurns())
	}
	if a.GetBackground() {
		fmt.Printf("    ⚡ Background agent\n")
	}
	fmt.Println()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
