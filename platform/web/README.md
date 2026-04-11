# DogClaw Web 平台

这是 DogClaw 的 Web 平台界面，提供了聊天和设置功能。

## 开发

```bash
# 安装依赖
npm install

# 启动开发服务器
npm run dev

# 构建生产版本
npm run build
```

## 功能特性

### 聊天界面
- 现代化的聊天界面
- 支持发送消息和接收回复
- 实时打字指示器
- 用户和 AI 消息区分显示

### 设置界面
- 通用设置：活跃别名、温度、Top P 等
- 提供商配置：管理不同的 LLM 提供商
- 渠道配置：Gateway、QQ、微信等渠道设置
- 高级设置：心跳、预算、权限模式等

## 技术栈

- Vue 3
- Vite
- CSS3 (Flexbox, Grid)

## 项目结构

```
platform/web/
├── dist/                # 构建输出目录
├── src/
│   ├── components/
│   │   ├── Chat.vue    # 聊天组件
│   │   └── Settings.vue# 设置组件
│   ├── App.vue         # 主应用组件
│   └── main.js         # 应用入口
├── index.html          # HTML 模板
└── package.json        # 项目配置
```
