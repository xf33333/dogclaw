package main

import (
	"fmt"
	"dogclaw/pkg/version"
)

// PrintVersion 打印版本信息
func PrintVersion() {
	fmt.Println(version.GetVersionString())
	fmt.Println()
}
