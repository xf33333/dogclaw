# DogClaw 🦞

**24/7 AI Agent** - Go implementation, runs on any platform, lightweight and efficient.

A powerful AI-powered coding assistant built with Go. Not only an excellent Agent Coder, but also a great AI assistant.

## Overview

DogClaw is a fully-featured AI assistant that empowers you with:

- **Query Engine**: LLM conversation loop with tool calling
- **Tool System**: File operations, shell execution, web search, and more
- **API Client**: Anthropic Messages API with streaming support
- **CLI Interface**: Interactive and non-interactive modes
- **Session Management**: Auto-save and restore conversation history
- **Token Usage Tracking**: Monitor your token consumption
- **Multi-Channel Support**: QQ, WeChat, and other instant messaging platforms
- **Multi-Project Mode**: Isolate workspaces for different projects

## AI-Generated Project

This project is **100% AI-generated code** — it's a product of AI self-iteration and autonomous development. Every line of code, architecture decision, and bug fix was created by AI, for AI. DogClaw represents a new paradigm in software engineering where AI systems can design, build, and improve themselves without human intervention.

## Fully Open Source

DogClaw is **completely open source** — free to use, modify, and distribute. We believe in the power of open collaboration and transparency. The entire source code is available for anyone to inspect, learn from, and build upon. Join us in shaping the future of AI-powered development!

## Key Highlights

### Beautiful Interface & Excellent Coding Support
- **Beautiful UI**: Clean, modern, and user-friendly interface for a delightful experience
- **Excellent Coding Support**: Not just a powerful agent coder, but also an exceptional AI assistant
- **Intelligent Code Understanding**: Deep comprehension of codebases, architecture, and patterns
- **Smart Assistance**: Helps with coding, debugging, refactoring, answering questions, and more

### Cross-Platform Compatibility
- Runs everywhere: Linux, macOS, Windows, Raspberry Pi, and more
- Single binary deployment - no dependencies or runtime required
- Works on ARM, x86, and x64 architectures

### Low Resource Footprint
- **Minimal memory usage**: Optimized for environments with limited RAM
- **Lightweight CPU footprint**: Runs smoothly on low-power devices
- **Low hardware requirements**: Works perfectly on various machines, SBCs, and old hardware
- No background bloat - only uses resources when actively processing

### Smart Features
- **Auto-compaction**: Automatically compresses long conversations to stay within context limits
- **Daily log rotation**: Logs automatically rotate by date
- **Multi-project workspace**: Isolate sessions and settings per project
- **Skills system**: Load custom skills from project or user directories
- **Cron tasks**: Schedule automated tasks
- **Channel notifications**: Send messages to specific channels (QQ, WeChat, CLI)

## Features

### Core Capabilities
- **Smart Conversation Engine**: Multi-turn dialogue, tool calling, streaming responses
- **Multiple API Providers**: Support for Anthropic, OpenRouter, and any OpenAI-compatible interface
- **Automatic Session Management**: Auto-save and restore conversation history
- **Flexible Configuration**: YAML/JSON config files, environment variable support
- **Multi-Platform Support**: Linux, macOS, Windows
- **Multi-Channel Integration**: QQ, WeChat, and other instant messaging platforms
- **Token Usage Statistics**: Track token consumption with /usage command

### Operation Modes

#### Agent Mode (Interactive CLI)
Directly converse with the AI assistant, with history navigation (readline)

```bash
./dogclaw agent
```

#### Gateway Mode (Channel Gateway)
Start QQ, WeChat and other channels to receive messages and auto-respond

```bash
./dogclaw gateway
```

#### Onboard Mode (Configuration Wizard)
Interactive setup for models, channels, and API settings

```bash
./dogclaw onboard
```

## Installation

### Building from Source

```bash
go build -o dogclaw ./cmd/dogclaw/
```

### Binary Releases

Download the latest binary from the releases page.

## Usage

### Command Line Options

```
Usage: dogclaw [options] <mode>

Options:
  --config <path>, -c <path>  Path to custom configuration file
  --multi-project, -m          Enable multi-project mode (use current dir as workspace)
  --compact                    Compact the most recent session and exit
  --version                    Show version information

Modes:
  agent    CLI interactive mode for direct communication
  gateway  Starts all configured channels (QQ, Weixin, etc.)
  onboard  Interactive setup for models and channels

Examples:
  dogclaw agent
  dogclaw --config /path/to/config.json gateway
  dogclaw -c ./myconfig.json onboard
  dogclaw --compact
```

### Interactive Mode (Agent Mode)

```bash
./dogclaw agent
```

In agent mode, you can use slash commands:

| Command | Description |
|---------|-------------|
| `/help` | Show help information |
| `/usage` | Show token usage statistics |
| `/model <name>` | Switch model (sonnet/opus/haiku) |
| `/compact` | Manually trigger context compaction |
| `/verbose` | Toggle verbose mode |
| `/settings` | Show current settings |
| `/sessions` | List available sessions |
| `/session <id>` | Switch to a specific session |
| `/resume` | Resume a session |
| `/skills` | List available skills |
| `/clear` | Clear conversation history |
| `/exit` / `/quit` | Exit the program |

### Gateway Mode

```bash
./dogclaw gateway
```

Gateway mode starts all configured channels (QQ, WeChat, etc.) and automatically responds to messages.

### Multi-Project Mode

```bash
./dogclaw --multi-project agent
# or
./dogclaw -m agent
```

Multi-project mode uses the current directory as the workspace. Each project has its own isolated:
- Session history
- Memory storage
- Configuration (`.dogclaw/settings.json`)
- Skills directory (`.dogclaw/skills/`)

### Token Usage Tracking

Use the `/usage` command to view token statistics:

```
=== Token Usage Statistics ===

--- Today ---
  Model: sonnet
    Input:   12.5K tokens
    Output:  3.2K tokens
    Cache R: 500 tokens
    Total:   16.2K tokens
  Total for Today:
    Input:   12.5K tokens
    Output:  3.2K tokens
    Total:   16.2K tokens

--- This Week ---
  ...

--- Current Session ---
  Input tokens:     2.1K
  Output tokens:    850
  Total tokens:     2.95K
  Turns:            5
```

### Configuration

Configuration is loaded in the following priority:
1. Custom path from `--config` flag
2. Working directory: `.dogclaw/settings.json` (if multi-project mode and file exists)
3. Default: `~/.dogclaw/settings.json`

### Settings.json Configuration

The `settings.json` file contains all DogClaw configuration options. Here's a complete example:

```json
{
  "activeAlias": "default",
  "providers": [
    {
      "alias": "default",
      "provider": "anthropic",
      "model": "claude-3-5-sonnet-20241022",
      "url": "https://api.anthropic.com",
      "apiKey": "sk-ant-..."
    },
    {
      "alias": "openrouter",
      "provider": "openrouter",
      "model": "anthropic/claude-3.5-sonnet",
      "url": "https://openrouter.ai/api/v1",
      "apiKey": "sk-or-..."
    }
  ],
  "channel": {
    "qq": {
      "enabled": false,
      "appID": "",
      "appSecret": "",
      "allowFrom": [],
      "sendMarkdown": false
    },
    "weixin": {
      "enabled": false,
      "token": "",
      "encodingAESKey": "",
      "port": 80
    },
    "gateway": {
      "enabled": true,
      "port": 10086
    }
  },
  "autoCompact": {
    "enabled": true,
    "thresholdRatio": 0.75,
    "warningRatio": 0.65,
    "maxContextTokens": 190000
  },
  "maxTurns": 1000,
  "maxTokens": 8192,
  "maxContextLength": 200000,
  "verbose": false,
  "temperature": 0,
  "topP": 0,
  "thinkingBudget": 0,
  "showToolUsageInReply": false,
  "showThinkingInLog": true
}
```

#### Configuration Fields

**Core Settings:**
| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `activeAlias` | string | Alias of the active model provider | `"default"` |
| `maxTurns` | int | Maximum conversation turns | `1000` |
| `maxTokens` | int | Maximum tokens per response | `8192` |
| `maxContextLength` | int | Maximum total context tokens | `200000` |
| `verbose` | bool | Enable verbose logging | `false` |
| `temperature` | float | LLM temperature (0-1) | `0` |
| `topP` | float | LLM top-p sampling | `0` |
| `thinkingBudget` | int | Thinking mode token budget (0=off) | `0` |
| `showToolUsageInReply` | bool | Show tool usage in replies | `false` |
| `showThinkingInLog` | bool | Show LLM thinking in logs | `true` |

**Provider Model Settings:**
| Field | Type | Description |
|-------|------|-------------|
| `alias` | string | Custom name for quick reference |
| `provider` | string | Provider type: `anthropic`, `openrouter`, `openai` |
| `model` | string | Model name (e.g., `claude-3-5-sonnet-20241022`) |
| `url` | string | API base URL |
| `apiKey` | string | API authentication key |

**Auto-Compact Settings:**
| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `enabled` | bool | Enable auto-compaction | `true` |
| `thresholdRatio` | float | Trigger compaction at this ratio | `0.75` (75%) |
| `warningRatio` | float | Show warning at this ratio | `0.65` (65%) |
| `maxContextTokens` | int | Hard limit before blocking | `190000` |

**Channel Settings (Gateway Mode):**

- **Gateway HTTP Server:**
  - `enabled`: Enable/disable gateway server
  - `port`: HTTP server port (default: 10086)

- **QQ Bot:**
  - `enabled`: Enable/disable QQ integration
  - `appID`: QQ app ID
  - `appSecret`: QQ app secret
  - `allowFrom`: List of allowed user/group IDs
  - `sendMarkdown`: Send messages in Markdown format

- **WeChat Bot:**
  - `enabled`: Enable/disable WeChat integration
  - `token`: WeChat verification token
  - `encodingAESKey`: AES encryption key
  - `port`: Webhook server port

### Supported Providers

| Provider | Base URL | Env Variables |
|----------|----------|---------------|
| Anthropic | `https://api.anthropic.com` | `ANTHROPIC_API_KEY` |
| OpenRouter | `https://openrouter.ai/api/v1` | `OPENROUTER_API_KEY` |
| Custom | Any OpenAI-compatible API | `ANTHROPIC_API_KEY`, custom `BaseURL` |

---

# DogClaw 🦞

**24/7 AI 智能助手** - Go 语言实现，可在任何平台运行，轻量高效。

一个强大的 AI 编程助手，使用 Go 语言构建。不仅是出色的 Agent Coder，更是优秀的 AI 助手。

## 概述

DogClaw 是一个功能完整的 AI 助手，为您提供：

- **查询引擎**: 带有工具调用的 LLM 对话循环
- **工具系统**: 文件操作、Shell 执行、网页搜索等
- **API 客户端**: 支持流式传输的 Anthropic Messages API
- **CLI 界面**: 交互式和非交互式模式
- **会话管理**: 自动保存和恢复对话历史
- **Token 使用追踪**: 监控您的 token 消耗
- **多通道支持**: QQ、微信和其他即时通讯平台
- **多项目模式**: 为不同项目隔离工作区

## AI 生成项目

本项目是 **100% AI 生成的代码** — 它是 AI 自我迭代和自主开发的产物。每一行代码、架构决策和错误修复都是由 AI 创建、为 AI 服务的。DogClaw 代表了软件工程的新范式，AI 系统可以在没有人工干预的情况下设计、构建和改进自己。

## 完全开源

DogClaw 是 **完全开源的** — 可以自由使用、修改和分发。我们相信开放协作和透明的力量。完整的源代码可供任何人检查、学习和构建。加入我们，共同塑造 AI 驱动开发的未来！

## 核心亮点

### 跨平台兼容性
- 随处运行：Linux、macOS、Windows、树莓派等
- 单二进制部署 - 无需依赖或运行时
- 支持 ARM、x86 和 x64 架构

### 低资源占用
- **最小内存使用**：为内存有限的环境优化
- **轻量级 CPU 占用**：在低功耗设备上流畅运行
- **低硬件要求**：在各种设备、单板机和旧硬件上完美运行
- 无后台膨胀 - 仅在主动处理时使用资源

### 智能功能
- **自动压缩**：自动压缩长对话以保持在上下文限制内
- **按天日志轮转**：日志自动按日期轮转
- **多项目工作区**：按项目隔离会话和设置
- **技能系统**：从项目或用户目录加载自定义技能
- **定时任务**：调度自动化任务
- **通道通知**：向特定通道发送消息（QQ、微信、CLI）

## 功能

### 核心能力
- **智能对话引擎**：多轮对话、工具调用、流式响应
- **多 API 提供商**：支持 Anthropic、OpenRouter 和任何兼容 OpenAI 的接口
- **自动会话管理**：自动保存和恢复对话历史
- **灵活配置**：YAML/JSON 配置文件，环境变量支持
- **多平台支持**：Linux、macOS、Windows
- **多通道集成**：QQ、微信和其他即时通讯平台
- **Token 使用统计**：使用 /usage 命令追踪 token 消耗

### 运行模式

#### 代理模式（交互式 CLI）
直接与 AI 助手对话，支持历史记录导航（readline）

```bash
./dogclaw agent
```

#### 网关模式（通道网关）
启动 QQ、微信和其他通道以接收消息并自动回复

```bash
./dogclaw gateway
```

#### 配置向导模式（交互式设置）
模型、通道和 API 设置的交互式配置

```bash
./dogclaw onboard
```

## 安装

### 从源码构建

```bash
go build -o dogclaw ./cmd/dogclaw/
```

### 二进制发布

从发布页面下载最新的二进制文件。

## 使用方法

### 命令行选项

```
用法：dogclaw [选项] <模式>

选项：
  --config <路径>, -c <路径>  自定义配置文件路径
  --multi-project, -m          启用多项目模式（使用当前目录作为工作区）
  --compact                    压缩最近的会话并退出
  --version                    显示版本信息

模式：
  agent    CLI 交互式模式，用于直接通信
  gateway  启动所有已配置的通道（QQ、微信等）
  onboard  模型和通道的交互式设置

示例：
  dogclaw agent
  dogclaw --config /path/to/config.json gateway
  dogclaw -c ./myconfig.json onboard
  dogclaw --compact
```

### 交互式模式（代理模式）

```bash
./dogclaw agent
```

在代理模式下，您可以使用斜杠命令：

| 命令 | 描述 |
|---------|-------------|
| `/help` | 显示帮助信息 |
| `/usage` | 显示 token 使用统计 |
| `/model <名称>` | 切换模型（sonnet/opus/haiku） |
| `/compact` | 手动触发上下文压缩 |
| `/verbose` | 切换详细模式 |
| `/settings` | 显示当前设置 |
| `/sessions` | 列出可用会话 |
| `/session <id>` | 切换到特定会话 |
| `/resume` | 恢复会话 |
| `/skills` | 列出可用技能 |
| `/clear` | 清除对话历史 |
| `/exit` / `/quit` | 退出程序 |

### 网关模式

```bash
./dogclaw gateway
```

网关模式启动所有已配置的通道（QQ、微信等）并自动回复消息。

### 多项目模式

```bash
./dogclaw --multi-project agent
# 或
./dogclaw -m agent
```

多项目模式使用当前目录作为工作区。每个项目都有其独立的：
- 会话历史
- 记忆存储
- 配置（`.dogclaw/settings.json`）
- 技能目录（`.dogclaw/skills/`）

### Token 使用追踪

使用 `/usage` 命令查看 token 统计：

```
=== Token 使用统计 ===

--- 今天 ---
  模型: sonnet
    输入:   12.5K tokens
    输出:  3.2K tokens
    缓存读: 500 tokens
    总计:   16.2K tokens
  今日总计:
    输入:   12.5K tokens
    输出:  3.2K tokens
    总计:   16.2K tokens

--- 本周 ---
  ...

--- 当前会话 ---
  输入 tokens:     2.1K
  输出 tokens:    850
  总 tokens:     2.95K
  轮次:            5
```

### 配置

配置按以下优先级加载：
1. `--config` 标志指定的自定义路径
2. 工作目录：`.dogclaw/settings.json`（如果是多项目模式且文件存在）
3. 默认：`~/.dogclaw/settings.json`

### Settings.json 配置说明

`settings.json` 文件包含 DogClaw 的所有配置选项。以下是完整示例：

```json
{
  "activeAlias": "default",
  "providers": [
    {
      "alias": "default",
      "provider": "anthropic",
      "model": "claude-3-5-sonnet-20241022",
      "url": "https://api.anthropic.com",
      "apiKey": "sk-ant-..."
    },
    {
      "alias": "openrouter",
      "provider": "openrouter",
      "model": "anthropic/claude-3.5-sonnet",
      "url": "https://openrouter.ai/api/v1",
      "apiKey": "sk-or-..."
    }
  ],
  "channel": {
    "qq": {
      "enabled": false,
      "appID": "",
      "appSecret": "",
      "allowFrom": [],
      "sendMarkdown": false
    },
    "weixin": {
      "enabled": false,
      "token": "",
      "encodingAESKey": "",
      "port": 80
    },
    "gateway": {
      "enabled": true,
      "port": 10086
    }
  },
  "autoCompact": {
    "enabled": true,
    "thresholdRatio": 0.75,
    "warningRatio": 0.65,
    "maxContextTokens": 190000
  },
  "maxTurns": 1000,
  "maxTokens": 8192,
  "maxContextLength": 200000,
  "verbose": false,
  "temperature": 0,
  "topP": 0,
  "thinkingBudget": 0,
  "showToolUsageInReply": false,
  "showThinkingInLog": true
}
```

#### 配置字段说明

**核心设置：**
| 字段 | 类型 | 描述 | 默认值 |
|-------|------|------|---------|
| `activeAlias` | string | 当前使用的模型提供商别名 | `"default"` |
| `maxTurns` | int | 最大对话轮数 | `1000` |
| `maxTokens` | int | 单次响应最大 token 数 | `8192` |
| `maxContextLength` | int | 最大总上下文 token 数 | `200000` |
| `verbose` | bool | 启用详细日志 | `false` |
| `temperature` | float | LLM 温度参数 (0-1) | `0` |
| `topP` | float | LLM top-p 采样 | `0` |
| `thinkingBudget` | int | 思考模式 token 预算 (0=关闭) | `0` |
| `showToolUsageInReply` | bool | 在回复中显示工具使用 | `false` |
| `showThinkingInLog` | bool | 在日志中显示 LLM 思考 | `true` |

**提供商模型设置：**
| 字段 | 类型 | 描述 |
|-------|------|------|
| `alias` | string | 自定义名称，用于快速引用 |
| `provider` | string | 提供商类型：`anthropic`, `openrouter`, `openai` |
| `model` | string | 模型名称（如 `claude-3-5-sonnet-20241022`） |
| `url` | string | API 基础 URL |
| `apiKey` | string | API 认证密钥 |

**自动压缩设置：**
| 字段 | 类型 | 描述 | 默认值 |
|-------|------|------|---------|
| `enabled` | bool | 启用自动压缩 | `true` |
| `thresholdRatio` | float | 在此比例时触发压缩 | `0.75` (75%) |
| `warningRatio` | float | 在此比例时显示警告 | `0.65` (65%) |
| `maxContextTokens` | int | 阻塞前的硬限制 | `190000` |

**通道设置（网关模式）：**

- **Gateway HTTP 服务器：**
  - `enabled`：启用/禁用网关服务器
  - `port`：HTTP 服务器端口（默认：10086）

- **QQ 机器人：**
  - `enabled`：启用/禁用 QQ 集成
  - `appID`：QQ 应用 ID
  - `appSecret`：QQ 应用密钥
  - `allowFrom`：允许的用户/群组 ID 列表
  - `sendMarkdown`：以 Markdown 格式发送消息

- **微信机器人：**
  - `enabled`：启用/禁用微信集成
  - `token`：微信验证令牌
  - `encodingAESKey`：AES 加密密钥
  - `port`：Webhook 服务器端口

### 支持的提供商

| 提供商 | 基础 URL | 环境变量 |
|----------|----------|---------------|
| Anthropic | `https://api.anthropic.com` | `ANTHROPIC_API_KEY` |
| OpenRouter | `https://openrouter.ai/api/v1` | `OPENROUTER_API_KEY` |
| 自定义 | 任何兼容 OpenAI 的 API | `ANTHROPIC_API_KEY`，自定义 `BaseURL` |

