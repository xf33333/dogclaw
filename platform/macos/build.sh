#!/bin/bash

# macOS 应用程序构建脚本
# 用于构建 DogClaw 状态栏应用

set -e

# 项目根目录
PROJECT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
MACOS_DIR="$PROJECT_ROOT/platform/macos"
BUILD_DIR="$MACOS_DIR/build"
APP_NAME="DogClawUI.app"
APP_DIR="$BUILD_DIR/$APP_NAME"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR_IN_APP="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"

echo "开始构建 DogClaw macOS 状态栏应用..."

# 清理旧的构建
echo "清理旧的构建..."
rm -rf "$BUILD_DIR"

# 创建目录结构
echo "创建目录结构..."
mkdir -p "$MACOS_DIR_IN_APP"
mkdir -p "$RESOURCES_DIR"

# 复制 Info.plist
echo "复制 Info.plist..."
cp "$MACOS_DIR/Info.plist" "$CONTENTS_DIR/"

# 复制应用图标（如果存在）
if [ -f "$MACOS_DIR/AppIcon.icns" ]; then
    echo "复制应用图标..."
    cp "$MACOS_DIR/AppIcon.icns" "$RESOURCES_DIR/"
else
    echo "注意: 未找到 AppIcon.icns，应用将使用默认图标"
    echo "参考 ICONS.md 了解如何添加自定义图标"
fi

# 构建 dogclaw 主程序
echo "构建 dogclaw 主程序..."
cd "$PROJECT_ROOT"
go build -o "$MACOS_DIR_IN_APP/dogclaw" ./cmd/dogclaw

# 构建状态栏应用
echo "构建状态栏应用..."
cd "$MACOS_DIR"
CGO_ENABLED=1 go build -o "$MACOS_DIR_IN_APP/DogClawUI" .

# 设置可执行权限
echo "设置可执行权限..."
chmod +x "$MACOS_DIR_IN_APP/dogclaw"
chmod +x "$MACOS_DIR_IN_APP/DogClawUI"

echo "构建完成！"
echo "应用位置: $APP_DIR"
echo ""
echo "你可以使用以下命令运行应用："
echo "open $APP_DIR"
