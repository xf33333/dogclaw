# DogClaw 🦞

**24小时 AI Agent** - Go语言实现，可运行在各种平台系统，50元以上机器都可运行。

使用 Go 语言构建的强大 AI 编程助手。

## 项目概述

DogClaw 是一个功能完整的 AI 助手，为你提供：

- **查询引擎**：支持工具调用的 LLM 对话循环
- **工具系统**：文件操作、Shell 执行、网页搜索等
- **API 客户端**：支持流式响应的 Anthropic Messages API
- **CLI 界面**：交互式和非交互式模式

## AI 生成项目

本项目**100% 由 AI 生成代码**——它是 AI 自我迭代和自主开发的产物。每一行代码、架构决策和 Bug 修复都由 AI 创建，为 AI 而创建。DogClaw 代表了软件工程的新范式：AI 系统可以在无需人工干预的情况下设计、构建和自我改进。

## 完全开源

DogClaw **完全开源**——可免费使用、修改和分发。我们相信开放协作和透明的力量。完整的源代码可供任何人查看、学习和构建。加入我们，共同塑造 AI 驱动开发的未来！

## 核心亮点

### 跨平台兼容性
- **无处不在**：Linux、macOS、Windows、树莓派等各种系统
- **单文件部署**：无依赖、无需运行时环境
- **架构支持**：ARM、x86、x64 全架构兼容

### 极低资源占用
- **内存占用极低**：为有限内存环境优化
- **CPU 占用轻盈**：在低功耗设备上流畅运行
- **硬件要求低**：50元以上机器、单板计算机、老旧硬件完美运行
- **无后台膨胀**：仅在活跃处理时占用资源

## 功能特性

### 核心能力
- **智能对话引擎**：支持多轮对话、工具调用、流式响应
- **多种API提供商**：支持 Anthropic、OpenRouter 及任何 OpenAI 兼容接口
- **自动会话管理**：自动保存和恢复历史会话
- **灵活配置**：YAML/JSON 配置文件，环境变量支持
- **多平台运行**：支持 Linux、macOS、Windows
- **多通道支持**：QQ、微信等即时通讯平台集成

### 运行模式

#### Agent 模式 (交互式 CLI)
直接与 AI 助手对话，支持历史记录上下翻查 (readline)

```bash
./dogclaw agent
```

#### Gateway 模式 (通道网关)
启动 QQ、微信等通道，接收消息并自动响应

```bash
./dogclaw gateway
```

#### Onboard 模式 (配置向导)
交互式配置模型、通道和 API 设置

```bash
./dogclaw onboard
```

## 构建

```bash
go build -o dogclaw ./cmd/dogclaw/
```

## 使用

### 交互模式

```bash
./dogclaw agent
```

### 网关模式

```bash
./dogclaw gateway
```

### 支持的提供商

| 提供商 | 基础 URL | 环境变量 |
|--------|----------|----------|
| Anthropic | `https://api.anthropic.com` | `ANTHROPIC_API_KEY` |
| OpenRouter | `https://openrouter.ai/api/v1` | `OPENROUTER_API_KEY` |
| 自定义 | 任何 OpenAI 兼容 API | `ANTHROPIC_API_KEY`、自定义 `BaseURL` |
