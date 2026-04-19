package commands

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

//go:embed initskill/*
var initskillFS embed.FS

// InitializeSkills 检查 ~/.dogclaw/skills 是否为空，如果为空则将 initskill 的内容复制过去
func InitializeSkills() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	skillsDir := filepath.Join(homeDir, ".dogclaw", "skills")

	// 检查目录是否存在，如果不存在则创建
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		if err := os.MkdirAll(skillsDir, 0755); err != nil {
			return fmt.Errorf("failed to create skills directory: %w", err)
		}
	}

	// 检查目录是否为空
	isDirEmpty, err := isDirectoryEmpty(skillsDir)
	if err != nil {
		return fmt.Errorf("failed to check if skills directory is empty: %w", err)
	}

	if !isDirEmpty {
		fmt.Println("Skills directory is not empty, skipping initialization")
		return nil
	}

	fmt.Println("Initializing skills directory with default skills...")

	// 复制 initskill 目录的内容
	if err := copyEmbeddedFS(initskillFS, "initskill", skillsDir); err != nil {
		return fmt.Errorf("failed to copy initial skills: %w", err)
	}

	fmt.Println("Successfully initialized skills directory")
	return nil
}

// isDirectoryEmpty 检查目录是否为空
func isDirectoryEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

// copyEmbeddedFS 将嵌入式文件系统的内容复制到目标目录
func copyEmbeddedFS(fs embed.FS, srcPath, destPath string) error {
	entries, err := fs.ReadDir(srcPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcEntryPath := filepath.Join(srcPath, entry.Name())
		destEntryPath := filepath.Join(destPath, entry.Name())

		if entry.IsDir() {
			// 创建目标目录
			if err := os.MkdirAll(destEntryPath, 0755); err != nil {
				return err
			}
			// 递归复制子目录
			if err := copyEmbeddedFS(fs, srcEntryPath, destEntryPath); err != nil {
				return err
			}
		} else {
			// 复制文件
			if err := copyEmbeddedFile(fs, srcEntryPath, destEntryPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyEmbeddedFile 复制单个嵌入式文件到目标位置
func copyEmbeddedFile(fs embed.FS, srcPath, destPath string) error {
	srcFile, err := fs.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return err
	}

	return nil
}
