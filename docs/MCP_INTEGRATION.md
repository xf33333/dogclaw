# MCP (Model Context Protocol) 集成指南

## 概述

本项目现在支持 MCP (Model Context Protocol) 功能，可以集成第三方 MCP 服务器提供的工具。

## 架构

### 核心组件

1. **pkg/mcp/types.go** - 定义 MCP 相关的数据结构和接口
2. **pkg/mcp/config.go** - MCP 配置管理
3. **pkg/mcp/tool_adapter.go** - 将 MCP 工具适配到项目现有的工具系统
4. **pkg/mcp/manager.go** - MCP 服务器和工具的管理器

## 配置步骤

### 1. 启用 MCP 功能

在 `~/.dogclaw/setting.json` 中启用 MCP：

```json
{
  "mcp": {
    "enabled": true,
    "configPath": "~/.dogclaw/mcp.json"
  }
}
```

### 2. 配置 MCP 服务器

创建 `~/.dogclaw/mcp.json` 文件，配置你的 MCP 服务器：

#### Stdio 方式（默认）

```json
{
  "servers": [
    {
      "name": "filesystem",
      "type": "stdio",
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "/path/to/your/project"
      ]
    },
    {
      "name": "github",
      "type": "stdio",
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-github"
      ],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "your-token-here"
      }
    }
  ]
}
```

#### HTTP Header 方式

```json
{
  "servers": [
    {
      "name": "custom-http-server",
      "type": "http",
      "url": "http://localhost:8080/mcp",
      "headers": {
        "Authorization": "Bearer your-api-token",
        "X-Custom-Header": "custom-value"
      }
    }
  ]
}
```

#### HTTP OAuth 方式

```json
{
  "servers": [
    {
      "name": "oauth-server",
      "type": "oauth",
      "url": "http://localhost:8080/mcp",
      "oauth": {
        "tokenUrl": "http://localhost:8080/oauth/token",
        "clientId": "your-client-id",
        "clientSecret": "your-client-secret",
        "scope": "read write"
      }
    }
  ]
}
```

#### 混合配置

```json
{
  "servers": [
    {
      "name": "filesystem",
      "type": "stdio",
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "/path/to/your/project"
      ]
    },
    {
      "name": "custom-http-server",
      "type": "http",
      "url": "http://localhost:8080/mcp",
      "headers": {
        "Authorization": "Bearer your-api-token"
      }
    },
    {
      "name": "oauth-server",
      "type": "oauth",
      "url": "http://localhost:8080/mcp",
      "oauth": {
        "tokenUrl": "http://localhost:8080/oauth/token",
        "clientId": "your-client-id",
        "clientSecret": "your-client-secret",
        "scope": "read write"
      }
    }
  ]
}
```

### 3. 工具命名约定

MCP 工具会按照以下格式命名：
- 完整名称: `mcp_{serverName}_{toolName}`
- 别名: `{serverName}_{toolName}` 和 `{toolName}`

## 使用示例

### 在代码中集成 MCP 管理器

```go
import (
    "context"
    "dogclaw/pkg/mcp"
)

// 加载 MCP 配置
configPath, _ := mcp.GetConfigPath()
mcpConfig, _ := mcp.LoadConfig(configPath)

// 创建 MCP 管理器
manager := mcp.NewManager(mcpConfig)

// 初始化（连接服务器并加载工具）
ctx := context.Background()
if err := manager.Initialize(ctx); err != nil {
    // 处理错误
}
defer manager.Shutdown()

// 获取所有 MCP 工具
tools := manager.GetTools()
```

## 扩展开发

### 实现真实的 MCP 客户端

当前 `mockClient` 是一个占位实现。要实现真实的 MCP 客户端，你需要：

1. 实现 `Client` 接口
2. 使用 Stdio 或其他传输方式与 MCP 服务器通信
3. 处理 JSON-RPC 协议

### 可用的 MCP 服务器

- 文件系统: `@modelcontextprotocol/server-filesystem`
- GitHub: `@modelcontextprotocol/server-github`
- PostgreSQL: `@modelcontextprotocol/server-postgres`
- 还有更多...

## 注意事项

1. MCP 功能默认是禁用的，需要在配置中显式启用
2. 确保 MCP 服务器命令可执行（如 `npx`）
3. 敏感信息（如 API 密钥）应通过环境变量传递
4. MCP 工具的执行权限继承自当前进程

## 下一步

- 实现真实的 MCP 客户端（支持 Stdio 传输）
- 添加工具权限控制
- 实现 MCP 资源和提示功能
- 添加 MCP 服务器健康检查
