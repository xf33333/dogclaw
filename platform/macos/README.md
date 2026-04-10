# DogClaw macOS 状态栏应用

这是一个 DogClaw 的 macOS 状态栏应用程序，可以方便地控制 gateway 模式。

## 功能

- 🚀 启动 Gateway - 以 gateway 模式启动 DogClaw
- 🛑 关闭 Gateway - 停止运行中的 gateway
- 💬 聊天 - 在浏览器中打开聊天界面 (http://localhost:10800)
- ⚙️ 设置 - 打开设置文件 (~/.dogclaw/settings.json)
- ❌ 退出 - 退出应用程序

## 构建

运行构建脚本：

```bash
./build.sh
```

构建完成后，应用将位于 `build/DogClawStatus.app`

## 运行

直接打开应用：

```bash
open build/DogClawStatus.app
```

或者将 `DogClawStatus.app` 复制到 `/Applications` 目录以便随时使用。

## 使用说明

1. 启动应用后，在状态栏会看到 🐾 图标
2. 点击图标显示菜单
3. 选择"启动 Gateway"来启动 DogClaw 的 gateway 模式
4. 选择"聊天"在浏览器中打开聊天界面
5. 选择"设置"来编辑配置文件
6. 选择"关闭 Gateway"来停止 gateway
7. 选择"退出"来关闭应用程序

## 注意事项

- 确保 `dogclaw` 可执行文件与状态栏应用在同一目录下，或者在系统 PATH 中
- 首次使用前，请先运行 `dogclaw onboard` 完成初始配置
