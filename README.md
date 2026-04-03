# DogClaw 🦞

Go implementation of Claude Code - an AI-powered coding assistant.

## Overview

This is a Go translation of the core functionality of [Claude Code](https://github.com/anthropics/claude-code), focusing on the essential features:

- **Query Engine**: LLM conversation loop with tool calling
- **Tool System**: File operations, shell execution, web search, and more
- **API Client**: Anthropic Messages API with streaming support
- **CLI Interface**: Interactive and non-interactive modes

## Features

### Tools Implemented

| Tool | Description |
|------|-------------|
| `Bash` | Execute shell commands |
| `Read` | Read file contents |
| `Write` | Create or overwrite files |
| `Edit` | Partial file editing (string replacement) |
| `Grep` | Search file contents with regex (ripgrep) |
| `Glob` | Find files by pattern matching |
| `WebSearch` | Search the web for information |
| `WebFetch` | Fetch and extract URL content |
| `TodoWrite` | Manage todo lists |
| `TaskCreate/Update` | Track work items |
| `SendMessage` | Inter-agent messaging |
| `EnterPlanMode/ExitPlanMode` | Plan mode toggle |
| `Sleep` | Wait for user input |
| `NotebookEdit` | Edit Jupyter notebook cells |
| `Exit` | Exit session |

### Architecture

```
dogclaw/
├── cmd/dogclaw/          # CLI entry point
├── pkg/
│   ├── types/             # Core type definitions
│   ├── tools/             # Tool implementations
│   └── query/             # Query engine
├── internal/
│   ├── api/               # Anthropic API client
│   ├── config/            # Configuration
│   └── permission/        # Permission system
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
