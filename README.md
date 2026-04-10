# DogClaw 🦞

**24/7 AI Agent** - Go implementation, runs on any platform, works on any $50+ machine.

A powerful AI-powered coding assistant built with Go.

## Overview

DogClaw is a fully-featured AI assistant that empowers you with:

- **Query Engine**: LLM conversation loop with tool calling
- **Tool System**: File operations, shell execution, web search, and more
- **API Client**: Anthropic Messages API with streaming support
- **CLI Interface**: Interactive and non-interactive modes

## AI-Generated Project

This project is **100% AI-generated code** — it's a product of AI self-iteration and autonomous development. Every line of code, architecture decision, and bug fix was created by AI, for AI. DogClaw represents a new paradigm in software engineering where AI systems can design, build, and improve themselves without human intervention.

## Fully Open Source

DogClaw is **completely open source** — free to use, modify, and distribute. We believe in the power of open collaboration and transparency. The entire source code is available for anyone to inspect, learn from, and build upon. Join us in shaping the future of AI-powered development!

## Key Highlights

### Cross-Platform Compatibility
- Runs everywhere: Linux, macOS, Windows, Raspberry Pi, and more
- Single binary deployment - no dependencies or runtime required
- Works on ARM, x86, and x64 architectures

### Low Resource Footprint
- **Minimal memory usage**: Optimized for environments with limited RAM
- **Lightweight CPU footprint**: Runs smoothly on low-power devices
- **Low hardware requirements**: Works perfectly on $50+ machines, SBCs, and old hardware
- No background bloat - only uses resources when actively processing

## Features

### Core Capabilities
- **Smart Conversation Engine**: Multi-turn dialogue, tool calling, streaming responses
- **Multiple API Providers**: Support for Anthropic, OpenRouter, and any OpenAI-compatible interface
- **Automatic Session Management**: Auto-save and restore conversation history
- **Flexible Configuration**: YAML/JSON config files, environment variable support
- **Multi-Platform Support**: Linux, macOS, Windows
- **Multi-Channel Integration**: QQ, WeChat, and other instant messaging platforms

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

## Building

```bash
go build -o dogclaw ./cmd/dogclaw/
```

## Usage

### Interactive Mode

```bash
./dogclaw agent
```

### Gateway Mode

```bash
./dogclaw gateway
```

### Supported Providers

| Provider | Base URL | Env Variables |
|----------|----------|---------------|
| Anthropic | `https://api.anthropic.com` | `ANTHROPIC_API_KEY` |
| OpenRouter | `https://openrouter.ai/api/v1` | `OPENROUTER_API_KEY` |
| Custom | Any OpenAI-compatible API | `ANTHROPIC_API_KEY`, custom `BaseURL` |
