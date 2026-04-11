# DogClaw Windows Tray Application

Windows 平台下的 DogClaw 托盘应用程序。

## 功能特性

- 系统托盘图标
- 启动/关闭/重启 Gateway
- 打开聊天界面
- 编辑设置
- 退出应用

## 构建

```bash
cd platform/win
GOOS=windows GOARCH=amd64 go build -o dogclaw-ui.exe main.go
```

## 使用

运行生成的 `dogclaw-ui.exe` 即可在系统托盘中看到 DogClaw 图标。
