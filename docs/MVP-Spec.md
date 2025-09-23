# CosTalk MVP 产品与技术设计说明（v1）

本文档用于指导 CosTalk 角色扮演语音对话产品的 MVP 设计与实现，覆盖产品范围、技术架构、接口契约、合规与验收标准。技术栈：后端 go-zero，前端 React。

## 1. 目标与愿景
- 目标：提供“可实时语音对话的角色扮演”体验，首包<1s，角色行为一致，至少3个可感知的“角色技能”。
- 愿景：低延迟、强沉浸、可扩展（多模型、多角色、可编排技能）。

## 2. 用户与场景
- 用户类型
  - 娱乐与IP爱好者：想与虚构角色对话（哈利波特等）。
  - 学习者：希望向历史/哲学/科学名人请教。
  - 内容创作者：需要灵感、剧情共创、设定打磨。
- 痛点
  - 普通聊天机器人缺少人设一致性、情绪与沉浸感差；语音对话延迟高；对话缺少结构化引导。
- 关键用户故事
  - 作为用户，我可搜索角色卡并一键开始语音对话；1秒内听到角色回应；角色保持其世界观与语气。
  - 作为学习者，我向“苏格拉底”提问，他会用反问法引导思考并布置小练习。
  - 作为创作者，我和“哈利波特”共同即兴编故事，他用兴奋语气讲述并可继续推进剧情。

## 3. 功能范围与优先级
- P0（MVP）
  - 语音对话链路：流式 ASR → 流式 LLM → 分段/流式 TTS → 前端播放队列
  - 角色卡3个以上：哈利波特、苏格拉底、居里夫人（示例）
  - 角色技能≥3：知识问答、讲故事/场景模拟、情绪表达（+短期记忆）
  - 会话上下文（短期记忆）与简要摘要
  - 内容安全：输入预审+输出流式后审；违规中断并给安全替代答复
  - 多LLM适配雏形：统一契约+1家默认+1家备选切换
- P1
  - 角色搜索与切换、收藏
  - 长期记忆（要点摘要持久化）
  - 打断/打岔（barge-in）与双工优化
  - 基本运营观测（留存/时长/成本）与控制台
- P2
  - 任务脚本/小游戏、RAG 小知识库
  - 用户登录、历史管理、多端同步

## 4. 模型选型与多模型策略
- 海外推荐
  - LLM：GPT-4o-mini 或 Claude 3.5 Sonnet（性价比/能力/多模态）
  - 语音：Azure Speech（ASR/TTS 流式、情绪/风格丰富）
- 国内推荐
  - LLM：通义 Qwen2.5-Turbo/Plus、百度ERNIE、腾讯混元（中文好、延迟低）
  - 语音：火山引擎/科大讯飞/百度语音（成熟流式能力）
- 默认与回退（建议）
  - 国内部署默认：Qwen2.5-Turbo + 科大讯飞/火山 ASR/TTS；海外默认：GPT-4o-mini + Azure Speech
  - 回退：本地小模型（仅文本）或次优云厂商，保证可用性与成本弹性

## 5. 角色技能定义与实现
- 技能列表（MVP必做）
  1) 知识问答：绑定角色人设+少量背景知识，回答风格贴合角色；越权拒答
  2) 讲故事/场景模拟：结构化生成（场景-冲突-互动-收束），分段输出以便 TTS 连播
  3) 情绪表达：在提示中注入语气/节奏/情绪标签；TTS 使用相应风格/韵律参数
  - 增强：短期记忆（会话裁剪+上一轮摘要2句）
- 实现方式
  - 提示工程：系统提示包含“角色守护/越权拒答/风格样例”
  - 小型状态：针对“练习/测验”类互动，用轻量模板（无需外部Agent）

## 6. 低延迟语音交互方案
- ASR：使用流式识别 + VAD 端点检测（静音阈值0.4~0.8s，末尾补偿200ms）
- LLM：流式输出，分句策略（标点/停顿/最大token）
- TTS：分段合成并边播边收；若支持流式 TTS 更优
- 前端：AudioContext 播放队列；首包缓冲200~300ms；Safari 不支持 Ogg，优先 MP3/AAC 或 PCM
- 目标：用户停语→1s 内角色开口

## 7. 安全与合规（双层防线）
- 预拦截（Pre-LLM）：规则引擎（敏感词/正则/越狱提示）+ 内容安全模型；动作=Block/Safe-Rewrite/Warn
- 后拦截（Post-LLM）：对增量文本分句审核；违规则中断流，返回安全替代答复；TTS仅对审核通过文本合成
- go-zero 落地
  - SafetyPreMiddleware：进入 Chat 处理前同步判定
  - SafetyPostStream：对 LLM 流式输出窗口化审核，边审边推
- 默认阈值（建议起步）
  - 审核分高≥0.8→Block，0.5~0.8→重写，<0.5→放行；审核超时>600ms→规则结果回退

## 8. 系统架构与时序（文字化）
- 时序：麦克风→ASR流→文字片段→LLM流文本→分句→TTS分段音频→前端播放队列
- 后端服务分层
  - API 网关（go-zero）：WS/SSE/REST；限流与认证
  - Conversation 服务：会话管理/记忆摘要/角色提示拼装
  - LLM Provider：统一适配器，支持流式
  - ASR/TTS Provider：统一适配器，分段/流式合成
  - Safety 模块：预审/后审
  - 观测：日志、指标、追踪

## 9. API 设计（MVP）
- WebSocket：`/v1/chat/stream`
  - 入参（JSON，首帧或URL Query）：
    - roleId: string
    - model: string（可选，默认配置）
    - messages: [{role: system|user|assistant, content: string}]（可选，首轮为空）
    - stream: boolean（true）
    - tts: { voice: string, style?: string }
  - 出站帧（JSON line 或二进制封装，统一使用 JSON 帧更简洁）
    - {type: "text_delta", seq: number, content: string}
    - {type: "audio_chunk", seq: number, format: "mp3|pcm", data: base64}
    - {type: "meta", usage?: {promptTokens, completionTokens, cost}, warnings?: []}
    - {type: "end"} / {type: "error", code: string, message: string}
- 角色列表：`GET /v1/roles`
- 健康检查：`GET /v1/health`

## 10. 适配器与统一契约（Go 接口草案）
- LLMProvider
  - Chat(ctx, req) → (resp)；ChatStream(ctx, req) → (<-chan Delta)
- ASRProvider
  - StreamRecognize(ctx, audioStream) → (<-chan Transcript)
- TTSProvider
  - SynthesizeStream(ctx, textStream, opts) → (<-chan AudioChunk)
- ModerationProvider / RuleEngine
  - CheckText(ctx, text) → Verdict；Match(text) → RuleHit
- ProviderRegistry：注册/选路/超时与熔断；参数映射（温度、top_p、max_tokens）与统一错误码

## 11. 数据模型（MVP）
- Role
  - id, name, avatar, description, systemPrompt, guardrails, ttsDefault
- Conversation
  - id, roleId, messages[{role, content, ts}], lastSummary（可选）
- Memory
  - Transient：窗口裁剪+上一轮摘要2句
  - Persistent（P1）：要点列表，按会话/用户存储

## 12. 非功能需求
- 性能：首包<1s，整轮<3~4s；并发>100（按资源可调）
- 可用性：Provider 降级/回退；超时重试指数退避
- 观测：latency、tps、错误率、token/成本；分模型/分区域统计
- 兼容：Safari/iOS 自动播放限制处理（用户手势激活 AudioContext）

## 13. 路线图与验收
- 里程碑 P0（2~3 周）
  - 完成端到端语音对话链路
  - 三个角色上线，三项技能可感知
  - 预审+后审可用；多LLM适配器（至少1默认+1备选）
  - 指标：首包<1s、无明显音频断裂、错误率<1%、日常成本可控
- 验收用例
  - 延迟：从用户停语到第一段音频播放时间
  - 角色一致性：5轮对话中≥4轮风格符合
  - 安全：违规输入/输出被拦截并给替代答复

## 14. 开发任务拆分（P0）
- 后端（go-zero）
  - WebSocket `/v1/chat/stream` 与事件帧
  - SafetyPreMiddleware / SafetyPostStream；规则词典与内容安全接入
  - LLM/ASR/TTS Provider 接口与默认实现；ProviderRegistry 与配置
  - Conversation 服务：上下文裁剪+摘要
  - 观测埋点与基础限流
- 前端（React）
  - 录音采集并上行；接收 WS 帧并维护播放队列
  - AudioContext 播放与缓冲；错误提示与自动重连
  - 角色卡与角色切换；基础设置（声音、风格）

## 15. 配置与部署
- 配置（env/yaml）
  - PROVIDER_LLM= qwen|gpt4omini|…；各 Provider 的 API Key/Region
  - ASR/TTS 格式、采样率、TTS 语音/风格默认
  - 安全阈值（block/rewrite/warn）、超时与重试
- 部署
  - 开发：本地 env + 直连云服务
  - 生产：多可用区、限流与熔断；日志与指标上报

## 16. 风险与备选
- TTS 情绪与风格受厂商能力限制 → 多家对比与热切换
- 浏览器限制导致首包受阻 → 首次交互即启用 AudioContext 并预热
- 成本不可控 → 粒度更大的分段策略与速率限制；按需压缩音频
- 合规（IP/形象） → 免责声明与角色“致敬而非复刻”的文案

---

附：建议的安全动作矩阵
- Pre-LLM：高危→Block；中危→Safe-Rewrite；低危→Warn
- Post-LLM：高危→中断并替代答复；中危→重写后继续；低危→放行

附：事件帧最小示例
- 入站：{"roleId":"socrates","model":"qwen2.5-turbo","stream":true}
- 出站：
  - {"type":"text_delta","seq":1,"content":"我很高兴与你探讨…"}
  - {"type":"audio_chunk","seq":1,"format":"mp3","data":"<base64>"}
  - {"type":"end"}

> 该文档作为 MVP 的执行依据；如需更改，采用轻量 PR 评审流程更新。
