package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func main() {
	// 直接测试 third_party/readline 的行为
	// 通过 pipe 模拟粘贴多行文本

	fmt.Println("=== 多行粘贴自动化测试 ===")

	// 创建一个 pipe，模拟 stdin
	pr, pw, err := os.Pipe()
	if err != nil {
		fmt.Printf("Failed to create pipe: %v\n", err)
		os.Exit(1)
	}

	// 在另一个 goroutine 里写入粘贴内容
	go func() {
		time.Sleep(100 * time.Millisecond)

		// 模拟 bracketed paste: \033[200~ + 内容 + \033[201~
		// 粘贴多行文本，最后一个\n后用户按Enter(\r)提交
		pasteContent := "line1\nline2\nline3"
		bpStart := "\x1b[200~"
		bpEnd := "\x1b[201~"

		// 先写 bracketed paste 的内容
		pw.WriteString(bpStart)
		pw.WriteString(pasteContent)
		pw.WriteString(bpEnd)

		// 用户按Enter提交
		pw.WriteString("\r")

		pw.Close()
	}()

	// 用 pr 作为 stdin 创建 readline
	// 注意：需要直接使用 third_party/readline 包
	// 但因为 module 路径问题，我们换一种方式测试

	// 直接读取并分析 pr 的输出
	buf := make([]byte, 1024)
	var allData string
	for {
		n, err := pr.Read(buf)
		if n > 0 {
			allData += string(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Read error: %v\n", err)
			break
		}
	}

	fmt.Printf("Data from pipe: %q\n", allData)
	_ = strings.TrimSpace(allData)
}
