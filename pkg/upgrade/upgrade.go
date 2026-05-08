package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"dogclaw/pkg/version"
)

const (
	githubOwner = "xf33333"
	githubRepo  = "dogclaw"
	apiBaseURL  = "https://api.github.com/repos/" + githubOwner + "/" + githubRepo
)

// GitHubRelease 表示 GitHub Release API 返回的发布信息
type GitHubRelease struct {
	TagName string         `json:"tag_name"`
	Name    string         `json:"name"`
	Assets  []GitHubAsset  `json:"assets"`
	Body    string         `json:"body"`
}

// GitHubAsset 表示 Release 中的资产文件
type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// UpgradeResult 表示升级操作的结果
type UpgradeResult struct {
	CurrentVersion string
	LatestVersion  string
	Upgraded       bool
	Message        string
}

// getLatestRelease 从 GitHub API 获取最新 Release 信息
func getLatestRelease(ctx context.Context) (*GitHubRelease, error) {
	url := apiBaseURL + "/releases/latest"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "dogclaw-upgrade")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub API 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API 返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析 GitHub API 响应失败: %w", err)
	}

	return &release, nil
}

// getAssetName 根据当前系统信息生成期望的资产文件名
func getAssetName(tagName string) string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// 映射架构名称以匹配 Release 命名规则
	archMap := map[string]string{
		"amd64": "amd64",
		"arm64": "arm64",
		"arm":   "armv7",
	}
	arch, ok := archMap[goarch]
	if !ok {
		arch = goarch
	}

	// Windows 的文件名带 .exe 后缀
	ext := ""
	if goos == "windows" {
		ext = ".exe"
	}

	return fmt.Sprintf("dogclaw-%s-%s-%s%s", tagName, goos, arch, ext)
}

// findAsset 在 Release 资产列表中查找匹配当前系统的资产
func findAsset(release *GitHubRelease) *GitHubAsset {
	expectedName := getAssetName(release.TagName)

	for i := range release.Assets {
		asset := &release.Assets[i]
		if asset.Name == expectedName {
			return asset
		}
	}

	// 降级匹配：尝试不区分大小写
	expectedLower := strings.ToLower(expectedName)
	for i := range release.Assets {
		asset := &release.Assets[i]
		if strings.ToLower(asset.Name) == expectedLower {
			return asset
		}
	}

	return nil
}

// downloadFile 下载文件到指定路径
func downloadFile(ctx context.Context, url, destPath string, onProgress func(downloaded, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("创建下载请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "dogclaw-upgrade")

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载文件失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载文件返回错误状态码 %d", resp.StatusCode)
	}

	// 先写入临时文件
	tmpPath := destPath + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer os.Remove(tmpPath)

	var downloaded int64
	total := resp.ContentLength
	buf := make([]byte, 32*1024) // 32KB 缓冲区

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			written, writeErr := tmpFile.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("写入临时文件失败: %w", writeErr)
			}
			downloaded += int64(written)
			if onProgress != nil {
				onProgress(downloaded, total)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取下载内容失败: %w", err)
		}
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}

	// 验证下载的文件不为空
	info, err := os.Stat(tmpPath)
	if err != nil || info.Size() == 0 {
		return fmt.Errorf("下载的文件为空或无效")
	}

	// 移动临时文件到目标路径
	if err := os.Rename(tmpPath, destPath); err != nil {
		// 如果跨设备移动失败，使用复制+删除
		if err := copyFile(tmpPath, destPath); err != nil {
			return fmt.Errorf("移动文件到目标路径失败: %w", err)
		}
		os.Remove(tmpPath)
	}

	return nil
}

// copyFile 复制文件（跨设备移动的降级方案）
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// getCurrentBinaryPath 获取当前正在执行的二进制文件路径
func getCurrentBinaryPath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("获取当前可执行文件路径失败: %w", err)
	}

	// 解析符号链接
	resolvedPath, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		return exePath, nil // 降级使用原始路径
	}
	return resolvedPath, nil
}

// normalizeVersion 规范化版本号字符串用于比较
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	return v
}

// Run 执行升级操作
func Run(ctx context.Context) (*UpgradeResult, error) {
	currentVersion := version.Version

	// 1. 获取最新 Release 信息
	release, err := getLatestRelease(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取最新版本信息失败: %w", err)
	}

	latestVersion := normalizeVersion(release.TagName)
	currentVer := normalizeVersion(currentVersion)

	// 2. 比较版本号
	if currentVer == latestVersion {
		return &UpgradeResult{
			CurrentVersion: currentVersion,
			LatestVersion:  release.TagName,
			Upgraded:       false,
			Message:        fmt.Sprintf("当前已是最新版本 %s", currentVersion),
		}, nil
	}

	if currentVer == "dev" || currentVer == "unknown" {
		// 开发版本或未知版本，允许升级
	} else if currentVer >= latestVersion {
		return &UpgradeResult{
			CurrentVersion: currentVersion,
			LatestVersion:  release.TagName,
			Upgraded:       false,
			Message:        fmt.Sprintf("当前版本 %s 不低于最新版本 %s", currentVersion, release.TagName),
		}, nil
	}

	// 3. 查找匹配当前系统的资产
	asset := findAsset(release)
	if asset == nil {
		// 列出可用资产供参考
		var available []string
		for _, a := range release.Assets {
			available = append(available, a.Name)
		}
		return nil, fmt.Errorf("未找到适配当前系统 (%s/%s) 的二进制文件，期望文件名: %s，可用资产: %s",
			runtime.GOOS, runtime.GOARCH,
			getAssetName(release.TagName),
			strings.Join(available, ", "))
	}

	// 4. 获取当前二进制文件路径
	binaryPath, err := getCurrentBinaryPath()
	if err != nil {
		return nil, fmt.Errorf("获取当前可执行文件路径失败: %w", err)
	}

	// 5. 下载新版本
	fmt.Printf("🔄 正在下载 %s (%s/%s)...\n", asset.Name, runtime.GOOS, runtime.GOARCH)

	downloadPath := binaryPath + ".new"
	if err := downloadFile(ctx, asset.BrowserDownloadURL, downloadPath, func(downloaded, total int64) {
		if total > 0 {
			percent := float64(downloaded) / float64(total) * 100
			fmt.Printf("\r📥 下载进度: %.1f%% (%d/%d bytes)", percent, downloaded, total)
		}
	}); err != nil {
		os.Remove(downloadPath)
		return nil, fmt.Errorf("下载新版本失败: %w", err)
	}
	fmt.Println() // 换行

	// 6. 设置可执行权限 (非 Windows)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(downloadPath, 0755); err != nil {
			os.Remove(downloadPath)
			return nil, fmt.Errorf("设置可执行权限失败: %w", err)
		}
	}

	// 7. 备份当前版本
	backupPath := binaryPath + ".bak"
	// 移除旧备份（如果存在）
	os.Remove(backupPath)
	if err := os.Rename(binaryPath, backupPath); err != nil {
		// 如果重命名失败（可能是因为正在运行），尝试复制方式备份
		if err := copyFile(binaryPath, backupPath); err != nil {
			fmt.Printf("⚠️  备份当前版本失败（不影响升级）: %v\n", err)
		}
	} else {
		fmt.Printf("📦 已备份当前版本到: %s\n", backupPath)
	}

	// 8. 替换二进制文件
	if err := os.Rename(downloadPath, binaryPath); err != nil {
		// 重命名失败时，尝试复制
		if err := copyFile(downloadPath, binaryPath); err != nil {
			// 恢复备份
			os.Rename(backupPath, binaryPath)
			os.Remove(downloadPath)
			return nil, fmt.Errorf("替换二进制文件失败: %w", err)
		}
		os.Remove(downloadPath)
	}

	// 9. 确保新文件有可执行权限
	if runtime.GOOS != "windows" {
		os.Chmod(binaryPath, 0755)
	}

	fmt.Printf("✅ 升级完成: %s -> %s\n", currentVersion, release.TagName)

	return &UpgradeResult{
		CurrentVersion: currentVersion,
		LatestVersion:  release.TagName,
		Upgraded:       true,
		Message:        fmt.Sprintf("成功从 %s 升级到 %s，即将重启...", currentVersion, release.TagName),
	}, nil
}

// CheckForUpdate 检查是否有可用更新（不执行升级）
func CheckForUpdate(ctx context.Context) (*UpgradeResult, error) {
	currentVersion := version.Version

	release, err := getLatestRelease(ctx)
	if err != nil {
		return nil, err
	}

	latestVersion := normalizeVersion(release.TagName)
	currentVer := normalizeVersion(currentVersion)

	hasUpdate := false
	if currentVer == "dev" || currentVer == "unknown" {
		hasUpdate = true
	} else if currentVer < latestVersion {
		hasUpdate = true
	}

	return &UpgradeResult{
		CurrentVersion: currentVersion,
		LatestVersion:  release.TagName,
		Upgraded:       false,
		Message: func() string {
			if hasUpdate {
				return fmt.Sprintf("发现新版本: %s (当前: %s)，使用 /upgrade 进行升级", release.TagName, currentVersion)
			}
			return fmt.Sprintf("当前已是最新版本 %s", currentVersion)
		}(),
	}, nil
}
