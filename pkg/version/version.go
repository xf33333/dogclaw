package version

import "fmt"

// 这些变量会在构建时通过 -ldflags 注入
var (
	BuildTime = "unknown"
	GitCommit = "unknown"
	Version   = "dev"
)

// GetVersionString 返回格式化的版本信息字符串
func GetVersionString() string {
	return fmt.Sprintf("🦞 DogClaw - AI Coding Assistant (Go Implementation)\n"+
		"   Version: %s\n"+
		"   Commit:  %s\n"+
		"   Built:   %s",
		Version, GitCommit, BuildTime)
}
