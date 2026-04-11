@echo off
REM Windows 平台构建脚本

echo Building DogClaw UI for Windows...

set GOOS=windows
set GOARCH=amd64

REM 构建 UI 程序
go build -ldflags "-s -w" -o build/dogclaw-ui.exe main.go

if %errorlevel% neq 0 (
    echo Build failed!
    exit /b 1
)

echo Build successful!
echo Output: build/dogclaw-ui.exe
