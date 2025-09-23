# CosTalk - AI 角色对话平台

一个基于 AI 的实时语音角色扮演对话平台，支持与虚拟角色进行低延迟语音交互。

## 技术栈

- **后端**: Go + go-zero 框架
- **前端**: React + TypeScript
- **通信**: WebSocket 实时流式传输
- **AI 能力**: LLM + ASR + TTS

## 项目特性

### 核心功能
- 🎭 **多角色对话**: 支持哈利波特、苏格拉底、居里夫人等多个预设角色
- 🎤 **实时语音**: 流式 ASR → LLM → TTS 低延迟链路（<1s 首包响应）
- 🧠 **角色技能**: 知识问答、情绪表达、讲故事、短期记忆等 3+ 核心技能
- 🔒 **内容安全**: 双层拦截（预审+后审），违规内容自动过滤
- 🔄 **多模型**: 统一接口，支持 Qwen、GPT、Claude 等多个 LLM 热切换

### 技术特性
- ⚡ **流式处理**: 边生成边播放，无需等待完整响应
- 🎵 **音频队列**: 前端 AudioContext 播放队列，无缝音频衔接
- 🛡️ **安全合规**: 敏感词过滤、内容审核、越权拒答
- 📊 **可观测**: 延迟监控、成本统计、错误追踪

## 快速开始

### 环境要求
- Go 1.19+
- Node.js 16+
- 至少一个 LLM/ASR/TTS 服务商 API Key

### 1. 克隆项目
```bash
git clone https://github.com/unclewu3242592726/CosTalk.git
cd CosTalk
```

### 2. 后端启动

```bash
cd backend

# 安装依赖
go mod tidy

# 配置环境变量
export QWEN_API_KEY="your_qwen_api_key"
export IFLYTEK_API_KEY="your_iflytek_key"
export ALIBABA_MODERATION_KEY="your_moderation_key"

# 构建并启动
go build -o bin/api cmd/api/main.go
./bin/api -f etc/api.yaml
```

### 3. 前端启动

```bash
cd frontend

# 安装依赖
npm install

# 启动开发服务器
npm start
```

访问 http://localhost:3000 开始使用。

## 配置说明

### 后端配置 (`backend/etc/api.yaml`)

```yaml
Name: costalk-api
Host: 0.0.0.0
Port: 8888

Providers:
  LLM:
    Default: "qwen"  # 默认 LLM 提供商
    APIKeys:
      qwen: "${QWEN_API_KEY}"
      gpt4omini: "${OPENAI_API_KEY}"
  
  ASR:
    Provider: "iflytek"  # ASR 提供商
    APIKey: "${IFLYTEK_API_KEY}"
    Format: "pcm"
    Language: "zh-CN"
  
  TTS:
    Provider: "iflytek"  # TTS 提供商
    APIKey: "${IFLYTEK_API_KEY}"
    Format: "mp3"
    Voice: "xiaoyan"

  Moderation:
    Provider: "alibaba"  # 内容审核提供商
    BlockThreshold: 0.8
    RewriteThreshold: 0.5

Safety:
  EnablePreCheck: true   # 启用预审
  EnablePostCheck: true  # 启用后审
  Keywords: ["暴力", "色情"]  # 敏感词列表
```

### 支持的 Provider

#### LLM
- `qwen`: 阿里通义千问
- `gpt4omini`: OpenAI GPT-4o-mini
- `claude`: Anthropic Claude (待实现)

#### ASR/TTS
- `iflytek`: 科大讯飞
- `volcano`: 火山引擎 (待实现)
- `azure`: Azure Speech (待实现)

#### 内容审核
- `alibaba`: 阿里云内容安全
- `baidu`: 百度内容审核 (待实现)

## API 文档

### WebSocket `/v1/chat/stream`

**连接参数:**
```
ws://localhost:8888/v1/chat/stream?roleId=harry
```

**消息格式:**

入站 (客户端 → 服务端):
```json
{
  "type": "user_message",
  "content": { "text": "你好，哈利" }
}
```

出站 (服务端 → 客户端):
```json
// 文本增量
{"type": "text_delta", "seq": 1, "content": {"text": "你好！我是哈利波特"}}

// 音频片段
{"type": "audio_chunk", "seq": 1, "content": {"format": "mp3", "data": "<base64>"}}

// 元数据
{"type": "meta", "content": {"usage": {"totalTokens": 50, "cost": 0.001}}}

// 结束
{"type": "end"}
```

### REST API

| 端点 | 方法 | 描述 |
|------|------|------|
| `/v1/health` | GET | 健康检查 |
| `/v1/roles` | GET | 获取可用角色列表 |

## 开发指引

### 添加新角色

1. 在 `backend/cmd/api/internal/handler/handlers.go` 的 `RolesHandler` 中添加角色定义
2. 创建角色的 system prompt 和技能配置
3. 前端会自动获取并显示新角色

### 添加新 Provider

1. 实现 `backend/pkg/provider/registry.go` 中的对应接口
2. 在 `backend/cmd/api/internal/svc/servicecontext.go` 中注册新 provider
3. 更新配置文件添加相应配置项

### 扩展角色技能

角色技能通过 system prompt 和结构化生成实现：

- **知识问答**: 在 prompt 中注入角色背景知识
- **情绪表达**: 使用情绪标签 + TTS 风格参数
- **讲故事**: 结构化模板（场景-冲突-互动-结束）
- **记忆**: 会话上下文管理 + 摘要生成

## 架构设计

```
用户录音 → ASR流式识别 → 安全预审 → LLM流式生成 → 安全后审 → TTS分段合成 → 前端播放队列
```

详细设计文档: [docs/MVP-Spec.md](docs/MVP-Spec.md)

## 部署

### Docker 部署 (TODO)
```bash
docker-compose up -d
```

### 生产配置建议
- 使用多可用区部署
- 配置限流与熔断
- 启用日志与指标监控
- 配置 HTTPS/WSS

## 贡献

1. Fork 项目
2. 创建功能分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add amazing feature'`)
4. 创建 Pull Request

## 许可证

[MIT License](LICENSE)

## 联系

项目链接: https://github.com/unclewu3242592726/CosTalk
