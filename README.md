# DogClaw 🦞

**24小时 AI Agent** - Go语言实现，可运行在各种平台系统，50元以上机器都可运行

Go implementation of Claude Code - an AI-powered coding assistant.

## Overview

This is a Go translation of the core functionality of [Claude Code](https://github.com/anthropics/claude-code), focusing on the essential features:

- **Query Engine**: LLM conversation loop with tool calling
- **Tool System**: File operations, shell execution, web search, and more
- **API Client**: Anthropic Messages API with streaming support
- **CLI Interface**: Interactive and non-interactive modes

## Features

### 核心能力

- **智能对话引擎**: 支持多轮对话，工具调用，流式响应
- **多种API提供商**: 支持 Anthropic、OpenRouter 及任何 OpenAI 兼容接口
- **自动会话管理**: 自动保存和恢复历史会话
- **灵活配置**: YAML/JSON 配置文件，环境变量支持
- **多平台运行**: 支持 Linux、macOS、Windows
- **多通道支持**: QQ、微信等即时通讯平台集成

### Tools Implemented

| Tool | Description | Status |
|------|-------------|--------|
| `Bash` | Execute shell commands | ✅ |
| `Read` | Read file contents | ✅ |
| `Write` | Create or overwrite files | ✅ |
| `Edit` | Partial file editing (string replacement) | ✅ |
| `Grep` | Search file contents with regex (ripgrep) | ✅ |
| `Glob` | Find files by pattern matching | ✅ |
| `WebSearch` | Search the web for information | ✅ |
| `WebFetch` | Fetch and extract URL content | ✅ |
| `TodoWrite` | Manage todo lists | ✅ |
| `TaskCreate/Update` | Track work items | ✅ |
| `SendMessage` | Inter-agent messaging | ✅ |
| `EnterPlanMode/ExitPlanMode` | Plan mode toggle | ✅ |
| `Sleep` | Wait for user input | ✅ |
| `NotebookEdit` | Edit Jupyter notebook cells | ✅ |
| `Exit` | Exit session | ✅ |
| `MemoryRead/Write` | Persistent memory management | ✅ |
| `AgentTool` | Load and execute other agents | ✅ |

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

### Architecture

```
dogclaw/
├── cmd/dogclaw/          # CLI entry point (main.go)
├── pkg/
│   ├── types/             # Core type definitions (Tool, Message, Permission)
│   ├── tools/             # Tool implementations (25+ tools)
│   ├── query/             # Query engine (conversation loop)
│   ├── channel/           # Channel adapters (QQ, Weixin)
│   │   ├── qq/           # QQ官方机器人API
│   │   └── weixin/       # 微信公众号/企业微信
│   ├── commands/          # CLI command system
│   ├── terminal/          # Terminal UI with readline
│   ├── memory/            # Persistent memory system
│   ├── bootstrap/         # Bootstrapping and state management
│   ├── coordinator/       # Multi-agent coordination
│   ├── context/           # Context management
│   ├── core/              # Core query engine logic
│   ├── compact/           # Compact session storage
│   ├── claudemd/          # Markdown rendering
│   └── ink/               # Terminal styling
├── internal/
│   ├── api/               # Anthropic/OpenRouter API client (streaming)
│   ├── config/            # Configuration and settings management
│   └── logger/            # Structured logging with caller info
└── go.mod
```

## Building

```bash
go build -o dogclaw ./cmd/dogclaw/
```

## Usage

### Interactive Mode

```bash
# Anthropic API
export ANTHROPIC_API_KEY="your-api-key"
./dogclaw

# OpenRouter API (auto-detected)
export OPENROUTER_API_KEY="your-api-key"
./dogclaw

# OpenRouter with custom model
export OPENROUTER_API_KEY="your-api-key"
OPENROUTER_MODEL="anthropic/claude-sonnet-4.5" ./dogclaw
```

### Non-Interactive Mode

```bash
./dogclaw "Write a hello world program in Go"
```

### Supported Providers

| Provider | Base URL | Env Variables |
|----------|----------|---------------|
| Anthropic | `https://api.anthropic.com` | `ANTHROPIC_API_KEY` |
| OpenRouter | `https://openrouter.ai/api/v1` | `OPENROUTER_API_KEY` |
| Custom | Any OpenAI-compatible API | `ANTHROPIC_API_KEY`, custom `BaseURL` |

The provider is **auto-detected** based on:
- `OPENROUTER_API_KEY` env var → OpenRouter
- Model name starting with `openrouter/`, `anthropic/`, `openai/`, `google/`, `qwen/`, etc. → OpenRouter
- Default → Anthropic

## Translation Progress

This is a **Phase A (Core MVP)** translation of the original TypeScript codebase:

| Module | Status | Notes |
|--------|--------|-------|
| Core Types | ✅ Complete | Tool, Message, Permission types |
| API Client | ✅ Complete | Messages API with streaming |
| Query Engine | ✅ Complete | Conversation loop |
| Tool System | ✅ 15 tools | Core tools implemented |
| Command System | 🚧 Partial | Basic CLI commands |
| Permission System | 🚧 Partial | Basic permission framework |
| UI Components | ❌ Skipped | React/Ink UI not applicable to Go |
| Bridge System | ❌ Skipped | IDE bridge (future) |
| Plugin System | ❌ Skipped | Plugin framework (future) |

## Differences from Original

1. **No Terminal UI**: The original uses React + Ink for a rich terminal UI. This version uses a simple CLI interface.
2. **Simplified Tool System**: Core tools are implemented, but some advanced features (MCP, LSP, etc.) are not yet included.
3. **No Feature Flags**: The original uses Bun's `bun:bundle` feature flags. Go uses build tags instead.
4. **Simplified Permission Model**: Basic permission framework without the full classifier system.

## License

This is an independent Go implementation inspired by the architecture of Claude Code.
It does not contain any original Anthropic code.
