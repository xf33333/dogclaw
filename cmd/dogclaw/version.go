package main

import "fmt"

// 这些变量会在构建时通过 -ldflags 注入
var (
	BuildTime = "unknown"
	GitCommit = "unknown"
	Version   = "dev"
)

// PrintVersion 打印版本信息
func PrintVersion() {
	fmt.Println("🦞 DogClaw - AI Coding Assistant (Go Implementation)")
	fmt.Printf("   Version: %s\n", Version)
	fmt.Printf("   Commit:  %s\n", GitCommit)
	fmt.Printf("   Built:   %s\n", BuildTime)
	fmt.Println()
}
