package chat

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unclewu3242592726/CosTalk/backend/internal/svc"
	"github.com/unclewu3242592726/CosTalk/backend/internal/types"
	"github.com/unclewu3242592726/CosTalk/backend/pkg/provider"

	"github.com/gorilla/websocket"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	MessageTypeConfig    = "config"
	MessageTypeText      = "text"
	MessageTypeAudio     = "audio"
	MessageTypeAudioFile = "audio_file"
	MessageTypeBinary    = "binary"
	MessageTypeASR       = "asr"
	MessageTypeASRResult = "asr_result"
	MessageTypeTTS       = "tts"
	MessageTypeResponse  = "response"
	MessageTypeError     = "error"
)

type ChatStreamLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
	// WebSocket写入互斥锁 - 每个连接一个
	wsWriteMutex sync.Mutex
	// TTS队列管理器
	ttsSequence int32 // 音频序列号
	ttsMutex    sync.Mutex // TTS序列化锁
}

func NewChatStreamLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ChatStreamLogic {
	return &ChatStreamLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}



// WebSocket 消息结构
type WSMessage struct {
	Type      string      `json:"type"`
	Seq       int         `json:"seq,omitempty"`
	Content   interface{} `json:"content,omitempty"`
	Timestamp int64       `json:"timestamp,omitempty"`
}

// 配置消息
type ConfigMessage struct {
	LLMProvider string            `json:"llmProvider,omitempty"`
	ASRProvider string            `json:"asrProvider,omitempty"`
	TTSProvider string            `json:"ttsProvider,omitempty"`
	Voice       string            `json:"voice,omitempty"`
	Speed       float64           `json:"speed,omitempty"`
	Role        string            `json:"role,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
}

// 文本消息
type TextMessage struct {
	Content string `json:"content"`
	Role    string `json:"role,omitempty"`
}

// 错误消息
type ErrorMessage struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (l *ChatStreamLogic) HandleWebSocket(conn *websocket.Conn) {
	defer conn.Close()

	// 会话状态
	var config ConfigMessage

	// 发送欢迎消息
	l.sendMessage(conn, &WSMessage{
		Type:      "welcome",
		Content:   "WebSocket connection established. Send config to start.",
		Timestamp: time.Now().Unix(),
	})

	// 主消息循环
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logx.Errorf("WebSocket error: %v", err)
			}
			break
		}

		switch messageType {
		case websocket.TextMessage:
			// 处理JSON消息
			var msg WSMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				l.sendError(conn, 400, "Invalid JSON message: "+err.Error())
				continue
			}

			switch msg.Type {
			case MessageTypeConfig:
				if err := l.handleConfig(&msg, &config); err != nil {
					l.sendError(conn, 400, err.Error())
				} else {
					l.sendMessage(conn, &WSMessage{
						Type:      "config_updated",
						Content:   "Configuration updated successfully",
						Timestamp: time.Now().Unix(),
					})
				}

			case MessageTypeAudio, MessageTypeAudioFile:
				// 处理完整音频文件进行ASR（支持audio和audio_file两种类型）
				go l.handleAudioFile(&msg, &config, conn)

			case MessageTypeText:
				// 直接处理文本输入
				go l.handleTextInput(&msg, &config, conn)

			default:
				l.sendError(conn, 400, "Unknown message type: "+msg.Type)
			}

		case websocket.BinaryMessage:
			// 处理二进制音频数据
			go l.handleBinaryAudio(data, &config, conn)

		default:
			l.sendError(conn, 400, "Unsupported message type")
		}
	}
}

// 处理完整音频文件进行ASR识别
func (l *ChatStreamLogic) handleAudioFile(msg *WSMessage, config *ConfigMessage, conn *websocket.Conn) {
	// 打印调试信息
	logx.Infof("Audio message content: %+v", msg.Content)
	
	// 解析音频文件数据
	audioData, ok := msg.Content.(map[string]interface{})
	if !ok {
		l.sendError(conn, 400, "Invalid audio file format")
		return
	}

	// 打印所有字段名以调试
	logx.Infof("Audio data fields: %v", func() []string {
		keys := make([]string, 0, len(audioData))
		for k := range audioData {
			keys = append(keys, k)
		}
		return keys
	}())

	// 获取音频数据，尝试多种可能的字段名
	var audioBytes []byte
	var audioDataRaw interface{}
	var exists bool
	
	// 尝试不同的字段名
	if audioDataRaw, exists = audioData["audio_data"]; exists {
		// 使用 audio_data 字段
	} else if audioDataRaw, exists = audioData["data"]; exists {
		// 使用 data 字段
	} else if audioDataRaw, exists = audioData["audioData"]; exists {
		// 使用 audioData 字段
	} else if audioDataRaw, exists = audioData["audio"]; exists {
		// 使用 audio 字段
	} else {
		l.sendError(conn, 400, "Missing audio data field (tried: audio_data, data, audioData, audio)")
		return
	}

	switch data := audioDataRaw.(type) {
	case string:
		// base64 编码的音频数据
		var err error
		audioBytes, err = base64.StdEncoding.DecodeString(data)
		if err != nil {
			l.sendError(conn, 400, "Failed to decode audio data: "+err.Error())
			return
		}
	case []byte:
		audioBytes = data
	case []interface{}:
		// 处理数字数组（JavaScript Array -> Go []interface{}）
		audioBytes = make([]byte, len(data))
		for i, v := range data {
			if num, ok := v.(float64); ok {
				audioBytes[i] = byte(num)
			} else {
				l.sendError(conn, 400, "Invalid audio data: array contains non-numeric values")
				return
			}
		}
	default:
		l.sendError(conn, 400, fmt.Sprintf("Unsupported audio data format: %T", data))
		return
	}

	if len(audioBytes) == 0 {
		l.sendError(conn, 400, "Empty audio data")
		return
	}

	logx.Infof("Processing audio file: %d bytes", len(audioBytes))

	// 发送处理状态
	l.sendMessage(conn, &WSMessage{
		Type:      "status",
		Content:   map[string]interface{}{"status": "processing_audio", "message": "正在识别语音..."},
		Timestamp: time.Now().Unix(),
	})

	// 调用ASR识别（一次性）
	text, err := l.performASR(audioBytes, config)
	if err != nil {
		l.sendError(conn, 500, "ASR failed: "+err.Error())
		return
	}

	if text == "" {
		l.sendError(conn, 400, "No text recognized from audio")
		return
	}

	logx.Infof("ASR result: '%s'", text)

	// 发送ASR结果
	l.sendMessage(conn, &WSMessage{
		Type:      MessageTypeASRResult,
		Content:   map[string]interface{}{"text": text},
		Timestamp: time.Now().Unix(),
	})

	// 继续处理文本 -> LLM -> TTS
	l.processTextToResponse(text, config, conn)
}

// 处理文本输入
func (l *ChatStreamLogic) handleTextInput(msg *WSMessage, config *ConfigMessage, conn *websocket.Conn) {
	textData, ok := msg.Content.(map[string]interface{})
	if !ok {
		l.sendError(conn, 400, "Invalid text format")
		return
	}

	text, ok := textData["content"].(string)
	if !ok {
		l.sendError(conn, 400, "Missing text content")
		return
	}

	if text == "" {
		l.sendError(conn, 400, "Empty text content")
		return
	}

	logx.Infof("Processing text input: '%s'", text)

	// 直接处理文本 -> LLM -> TTS
	l.processTextToResponse(text, config, conn)
}

// 处理二进制音频数据
func (l *ChatStreamLogic) handleBinaryAudio(audioData []byte, config *ConfigMessage, conn *websocket.Conn) {
	if len(audioData) == 0 {
		l.sendError(conn, 400, "Empty binary audio data")
		return
	}

	logx.Infof("Processing binary audio: %d bytes", len(audioData))

	// 发送处理状态
	l.sendMessage(conn, &WSMessage{
		Type:      "status",
		Content:   map[string]interface{}{"status": "processing_audio", "message": "正在识别语音..."},
		Timestamp: time.Now().Unix(),
	})

	// 调用ASR识别
	text, err := l.performASR(audioData, config)
	if err != nil {
		l.sendError(conn, 500, "ASR failed: "+err.Error())
		return
	}

	if text == "" {
		l.sendError(conn, 400, "No text recognized from audio")
		return
	}

	// 发送ASR结果并继续处理
	l.sendMessage(conn, &WSMessage{
		Type:      MessageTypeASRResult,
		Content:   map[string]interface{}{"text": text},
		Timestamp: time.Now().Unix(),
	})

	l.processTextToResponse(text, config, conn)
}

// 处理音频消息
func (l *ChatStreamLogic) handleAudioMessage(msg *WSMessage, audioStream chan<- []byte) error {
	// 期望的音频数据格式：
	// {
	//   "audio_data": [byte array],
	//   "format": "pcm",
	//   "sample_rate": 16000,
	//   "channels": 1
	// }
	audioData, ok := msg.Content.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid audio data format: expected object, got %T", msg.Content)
	}

	// 获取音频字节数组
	audioDataRaw, exists := audioData["audio_data"]
	if !exists {
		return fmt.Errorf("missing audio_data field")
	}

	var audioBytes []byte
	
	// 处理不同的音频数据格式
	switch data := audioDataRaw.(type) {
	case []interface{}:
		// 数组格式 [1, 2, 3, ...]
		audioBytes = make([]byte, len(data))
		for i, val := range data {
			if byteVal, ok := val.(float64); ok {
				audioBytes[i] = byte(byteVal)
			} else {
				return fmt.Errorf("invalid byte value in audio_data array at index %d", i)
			}
		}
	case string:
		// base64 编码的字符串
		var err error
		audioBytes, err = base64.StdEncoding.DecodeString(data)
		if err != nil {
			return fmt.Errorf("failed to decode base64 audio data: %v", err)
		}
	case []byte:
		// 直接的字节数组
		audioBytes = data
	default:
		return fmt.Errorf("unsupported audio_data format: %T", data)
	}

	if len(audioBytes) == 0 {
		return fmt.Errorf("empty audio data")
	}

	logx.Infof("Received audio data: %d bytes, format: %v, sample_rate: %v, channels: %v", 
		len(audioBytes), 
		audioData["format"], 
		audioData["sample_rate"], 
		audioData["channels"])
	
	select {
	case audioStream <- audioBytes:
		return nil
	default:
		return fmt.Errorf("audio stream buffer full")
	}
}

// 处理文本消息
func (l *ChatStreamLogic) handleTextMessage(msg *WSMessage, textStream chan<- string) error {
	textData, ok := msg.Content.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid text format")
	}

	content, ok := textData["content"].(string)
	if !ok {
		return fmt.Errorf("missing text content")
	}

	select {
	case textStream <- content:
		return nil
	default:
		return fmt.Errorf("text stream buffer full")
	}
}

// 处理音频流 -> ASR
func (l *ChatStreamLogic) handleAudioStream(ctx context.Context, audioStream <-chan []byte, asrResults chan<- *provider.Transcript, config *ConfigMessage, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	// 获取 ASR Provider
	asrProvider := config.ASRProvider
	if asrProvider == "" {
		asrProvider = "iflytek" // 默认使用讯飞ASR（更稳定）
	}

	asrProviderInstance, err := l.svcCtx.Registry.GetASR(asrProvider)
	if err != nil {
		logx.Errorf("Failed to get ASR provider %s: %v", asrProvider, err)
		return
	}

	// 创建持久的音频流通道
	persistentAudioStream := make(chan []byte, 100)
	
	// 启动 ASR 流式识别
	transcriptChan, err := asrProviderInstance.StreamRecognize(ctx, persistentAudioStream)
	if err != nil {
		logx.Errorf("ASR stream recognition failed: %v", err)
		return
	}

	// 转发 ASR 结果
	go func() {
		for transcript := range transcriptChan {
			select {
			case asrResults <- transcript:
			case <-ctx.Done():
				return
			}
		}
	}()

	// 处理音频数据
	for {
		select {
		case <-ctx.Done():
			close(persistentAudioStream)
			return
		case audioData := <-audioStream:
			if len(audioData) == 0 {
				continue
			}

			// 发送音频数据到持久流
			select {
			case persistentAudioStream <- audioData:
				// 音频数据已发送
			default:
				logx.Errorw("ASR audio stream buffer full, dropping audio data")
			}
		}
	}
}

// 处理文本流 -> LLM -> TTS (支持流式处理)
func (l *ChatStreamLogic) handleTextStream(ctx context.Context, textStream <-chan string, conn *websocket.Conn, config *ConfigMessage, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case text := <-textStream:
			if text == "" {
				continue
			}

			logx.Infof("LLM输入文本: '%s'", text)

			// 启动流式LLM处理
			go l.processStreamingLLM(ctx, text, config, conn)
		}
	}
}

// 处理 ASR 结果
func (l *ChatStreamLogic) handleASRResults(ctx context.Context, asrResults <-chan *provider.Transcript, textStream chan<- string, conn *websocket.Conn, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case transcript := <-asrResults:
			if transcript == nil {
				continue
			}

			logx.Infof("ASR结果: text='%s', is_final=%v, confidence=%.2f", 
				transcript.Text, transcript.IsFinal, transcript.Confidence)

			// 发送 ASR 结果给客户端
			l.sendMessage(conn, &WSMessage{
				Type: MessageTypeASR,
				Content: map[string]interface{}{
					"text":       transcript.Text,
					"is_final":   transcript.IsFinal,
					"confidence": transcript.Confidence,
				},
				Timestamp: time.Now().Unix(),
			})

			// 如果是最终结果，发送到文本流进行 LLM 处理
			if transcript.IsFinal && transcript.Text != "" {
				logx.Infof("发送到LLM处理: '%s'", transcript.Text)
				select {
				case textStream <- transcript.Text:
				case <-ctx.Done():
					return
				default:
					logx.Infof("Text stream buffer full, dropping message: %s", transcript.Text)
				}
			}
		}
	}
}

// 处理 TTS 结果
func (l *ChatStreamLogic) handleTTSResults(ctx context.Context, ttsResults <-chan *provider.AudioChunk, conn *websocket.Conn, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case audioChunk := <-ttsResults:
			if audioChunk == nil {
				continue
			}

			// 发送 TTS 音频结果给客户端
			l.sendMessage(conn, &WSMessage{
				Type: MessageTypeTTS,
				Content: map[string]interface{}{
					"audio_data": audioChunk.Data,
					"format":     audioChunk.Format,
					"seq_num":    audioChunk.SeqNum,
				},
				Timestamp: time.Now().Unix(),
			})
		}
	}
}

// 流式LLM处理 - 支持逐句TTS
func (l *ChatStreamLogic) processStreamingLLM(ctx context.Context, text string, config *ConfigMessage, conn *websocket.Conn) {
	llmProvider := config.LLMProvider
	if llmProvider == "" {
		llmProvider = "qiniu" // 默认使用七牛云
	}

	llmProviderInstance, err := l.svcCtx.Registry.GetLLM(llmProvider)
	if err != nil {
		logx.Errorf("Failed to get LLM provider %s: %v", llmProvider, err)
		l.sendError(conn, 500, "LLM provider not available: "+err.Error())
		return
	}

	// 构建聊天请求
	messages := []*provider.Message{
		{Role: "user", Content: text},
	}

	if config.Role != "" {
		messages = []*provider.Message{
			{Role: "system", Content: config.Role},
			{Role: "user", Content: text},
		}
	}

	// 启用流式处理
	req := &provider.ChatRequest{
		Model:    "deepseek-v3", // 使用七牛云支持的模型
		Messages: messages,
		Stream:   true, // 关键：启用流式处理
	}

	// 调用流式LLM
	streamChan, err := llmProviderInstance.ChatStream(ctx, req)
	if err != nil {
		logx.Errorf("LLM stream call failed: %v", err)
		l.sendError(conn, 500, "LLM stream processing failed: "+err.Error())
		return
	}

	var (
		accumulatedText = ""
		sentenceBuffer  = ""
		isFirstChunk    = true
	)

	for chunk := range streamChan {
		if chunk.Text == "" {
			continue
		}

		accumulatedText += chunk.Text
		sentenceBuffer += chunk.Text

		// 发送实时流式响应给客户端
		l.sendMessage(conn, &WSMessage{
			Type: MessageTypeResponse,
			Content: map[string]interface{}{
				"text":        chunk.Text,
				"type":        "llm_stream",
				"accumulated": accumulatedText,
				"is_first":    isFirstChunk,
				"is_done":     false,
			},
			Timestamp: time.Now().Unix(),
		})

		isFirstChunk = false

		// 检查是否完成了一个句子（以句号、问号、感叹号结尾）
		if l.isSentenceComplete(sentenceBuffer) {
			logx.Infof("检测到完整句子，启动TTS: '%s'", sentenceBuffer)
			
			// 序列化处理TTS，确保音频按顺序播放
			l.callSequentialTTS(ctx, sentenceBuffer, config, conn)
			
			sentenceBuffer = "" // 清空句子缓冲区
		}
	}

	// 处理最后可能剩余的文本
	if sentenceBuffer != "" {
		logx.Infof("处理剩余文本TTS: '%s'", sentenceBuffer)
		l.callSequentialTTS(ctx, sentenceBuffer, config, conn)
	}

	// 发送完成标志
	l.sendMessage(conn, &WSMessage{
		Type: MessageTypeResponse,
		Content: map[string]interface{}{
			"text":        "",
			"type":        "llm_stream",
			"accumulated": accumulatedText,
			"is_first":    false,
			"is_done":     true,
		},
		Timestamp: time.Now().Unix(),
	})

	logx.Infof("LLM流式处理完成，总文本: '%s'", accumulatedText)
}

// 判断句子是否完整
func (l *ChatStreamLogic) isSentenceComplete(text string) bool {
	if len(text) < 2 {
		return false
	}
	
	// 检查中文和英文的句子结束符
	lastChar := text[len(text)-1:]
	return lastChar == "。" || lastChar == "？" || lastChar == "！" || 
		   lastChar == "." || lastChar == "?" || lastChar == "!" ||
		   lastChar == "\n"
}

// 流式TTS处理
// callSequentialTTS 序列化TTS处理，确保音频按顺序播放
func (l *ChatStreamLogic) callSequentialTTS(ctx context.Context, text string, config *ConfigMessage, conn *websocket.Conn) {
	// 使用TTS互斥锁确保串行处理
	l.ttsMutex.Lock()
	defer l.ttsMutex.Unlock()
	
	ttsProvider := config.TTSProvider
	if ttsProvider == "" {
		ttsProvider = "qiniu" // 默认使用七牛云
	}

	ttsProviderInstance, err := l.svcCtx.Registry.GetTTS(ttsProvider)
	if err != nil {
		logx.Errorf("Failed to get TTS provider %s: %v", ttsProvider, err)
		return
	}

	// 创建文本流通道
	textStreamChan := make(chan string, 1)
	textStreamChan <- text
	close(textStreamChan)

	// TTS 选项
	opts := &provider.TTSOptions{
		Voice: config.Voice,
		Speed: config.Speed,
	}
	if opts.Voice == "" {
		opts.Voice = "qiniu_zh_female_wwxkjx"
	}
	if opts.Speed == 0 {
		opts.Speed = 1.0
	}

	// 调用 TTS
	audioChunkChan, err := ttsProviderInstance.SynthesizeStream(ctx, textStreamChan, opts)
	if err != nil {
		logx.Errorf("TTS stream call failed: %v", err)
		return
	}

	// 流式发送音频块，使用全局序列号
	for audioChunk := range audioChunkChan {
		if audioChunk == nil {
			continue
		}

		// 获取并递增序列号
		seqNumber := atomic.AddInt32(&l.ttsSequence, 1)

		logx.Infof("发送TTS音频块: %d bytes, format: %s, seq: %d", 
			len(audioChunk.Data), audioChunk.Format, seqNumber)

		// 发送音频块给客户端
		l.sendMessage(conn, &WSMessage{
			Type: MessageTypeTTS,
			Content: map[string]interface{}{
				"audio":     base64.StdEncoding.EncodeToString(audioChunk.Data),
				"format":    audioChunk.Format,
				"sequence":  seqNumber,
				"text":      text, // 关联的文本
			},
			Timestamp: time.Now().Unix(),
		})
	}
	
	logx.Infof("TTS序列化处理完成: %s", text)
}

func (l *ChatStreamLogic) callStreamTTS(ctx context.Context, text string, config *ConfigMessage, conn *websocket.Conn) {
	ttsProvider := config.TTSProvider
	if ttsProvider == "" {
		ttsProvider = "qiniu" // 默认使用七牛云
	}

	ttsProviderInstance, err := l.svcCtx.Registry.GetTTS(ttsProvider)
	if err != nil {
		logx.Errorf("Failed to get TTS provider %s: %v", ttsProvider, err)
		return
	}

	// 创建文本流通道
	textStreamChan := make(chan string, 1)
	textStreamChan <- text
	close(textStreamChan)

	// TTS 选项
	opts := &provider.TTSOptions{
		Voice: config.Voice,
		Speed: config.Speed,
	}
	if opts.Voice == "" {
		opts.Voice = "qiniu_zh_female_wwxkjx"
	}
	if opts.Speed == 0 {
		opts.Speed = 1.0
	}

	// 调用 TTS
	audioChunkChan, err := ttsProviderInstance.SynthesizeStream(ctx, textStreamChan, opts)
	if err != nil {
		logx.Errorf("TTS stream call failed: %v", err)
		return
	}

	// 立即流式发送音频块，不等待完整音频
	for audioChunk := range audioChunkChan {
		if audioChunk == nil {
			continue
		}

		logx.Infof("发送TTS音频块: %d bytes, format: %s, seq: %d", 
			len(audioChunk.Data), audioChunk.Format, audioChunk.SeqNum)

		l.sendMessage(conn, &WSMessage{
			Type: MessageTypeTTS,
			Content: map[string]interface{}{
				"audio_data": audioChunk.Data,
				"format":     audioChunk.Format,
				"seq_num":    audioChunk.SeqNum,
				"text":       text, // 包含对应的文本
				"streaming":  true,
			},
			Timestamp: time.Now().Unix(),
		})
	}
}

// 调用 LLM
func (l *ChatStreamLogic) callLLM(ctx context.Context, text string, config *ConfigMessage) (string, error) {
	llmProvider := config.LLMProvider
	if llmProvider == "" {
		llmProvider = "qiniu" // 默认使用七牛云
	}

	llmProviderInstance, err := l.svcCtx.Registry.GetLLM(llmProvider)
	if err != nil {
		return "", err
	}

	// 构建聊天请求
	messages := []*provider.Message{
		{Role: "user", Content: text},
	}

	if config.Role != "" {
		messages = []*provider.Message{
			{Role: "system", Content: config.Role},
			{Role: "user", Content: text},
		}
	}

	req := &provider.ChatRequest{
		Model:    "deepseek-v3", // 使用七牛云支持的模型
		Messages: messages,
		Stream:   false,
	}

	resp, err := llmProviderInstance.Chat(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Text, nil
}

// 调用 TTS
func (l *ChatStreamLogic) callTTS(ctx context.Context, text string, config *ConfigMessage, conn *websocket.Conn) error {
	ttsProvider := config.TTSProvider
	if ttsProvider == "" {
		ttsProvider = "qiniu" // 默认使用七牛云
	}

	ttsProviderInstance, err := l.svcCtx.Registry.GetTTS(ttsProvider)
	if err != nil {
		return err
	}

	// 创建文本流通道
	textStreamChan := make(chan string, 1)
	textStreamChan <- text
	close(textStreamChan)

	// TTS 选项
	opts := &provider.TTSOptions{
		Voice: config.Voice,
		Speed: config.Speed,
	}
	if opts.Voice == "" {
		opts.Voice = "qiniu_zh_female_wwxkjx"
	}
	if opts.Speed == 0 {
		opts.Speed = 1.0
	}

	// 调用 TTS
	audioChunkChan, err := ttsProviderInstance.SynthesizeStream(ctx, textStreamChan, opts)
	if err != nil {
		return err
	}

	// 收集所有音频块并合并
	go func() {
		var allAudioData []byte
		var format string
		
		for audioChunk := range audioChunkChan {
			allAudioData = append(allAudioData, audioChunk.Data...)
			if format == "" {
				format = audioChunk.Format
			}
		}
		
		// 发送合并后的完整音频
		if len(allAudioData) > 0 {
			logx.Infof("Sending complete TTS audio: %d bytes, format: %s", len(allAudioData), format)
			l.sendMessage(conn, &WSMessage{
				Type: MessageTypeTTS,
				Content: map[string]interface{}{
					"audio_data": allAudioData,
					"format":     format,
					"complete":   true,
				},
				Timestamp: time.Now().Unix(),
			})
		}
	}()

	return nil
}

// 发送消息 - 使用互斥锁确保线程安全
func (l *ChatStreamLogic) sendMessage(conn *websocket.Conn, msg *WSMessage) {
	l.wsWriteMutex.Lock()
	defer l.wsWriteMutex.Unlock()
	
	if err := conn.WriteJSON(msg); err != nil {
		logx.Errorf("Failed to send WebSocket message: %v", err)
	}
}

// 发送错误消息
func (l *ChatStreamLogic) sendError(conn *websocket.Conn, code int, message string) {
	l.sendMessage(conn, &WSMessage{
		Type: MessageTypeError,
		Content: ErrorMessage{
			Code:    code,
			Message: message,
		},
		Timestamp: time.Now().Unix(),
	})
}

// 发送ASR协议格式的响应
func (l *ChatStreamLogic) sendASRResponse(conn *websocket.Conn, response interface{}) error {
	// 序列化响应为JSON
	jsonData, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal ASR response: %v", err)
	}

	// GZIP压缩
	var compressedData bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressedData)
	if _, err := gzipWriter.Write(jsonData); err != nil {
		gzipWriter.Close()
		return fmt.Errorf("failed to compress ASR response: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %v", err)
	}

	// 构建ASR协议格式的响应
	var responseMsg bytes.Buffer
	
	// 协议头（4字节）
	// 第1字节：版本(高4位) + 头部大小(低4位)
	responseMsg.WriteByte((1 << 4) | 1) // 版本1，头部大小1
	// 第2字节：消息类型(高4位) + 消息标志(低4位)
	responseMsg.WriteByte((ASRProtocolFullServerResponse << 4)) // FULL_SERVER_RESPONSE，无序列号
	// 第3字节：序列化方法(高4位) + 压缩类型(低4位)
	responseMsg.WriteByte((1 << 4) | 1) // JSON序列化，GZIP压缩
	// 第4字节：保留字段
	responseMsg.WriteByte(0)

	// 负载长度（4字节，大端序）
	payloadLength := compressedData.Len()
	responseMsg.WriteByte(byte(payloadLength >> 24))
	responseMsg.WriteByte(byte(payloadLength >> 16))
	responseMsg.WriteByte(byte(payloadLength >> 8))
	responseMsg.WriteByte(byte(payloadLength))

	// 压缩后的负载数据
	responseMsg.Write(compressedData.Bytes())

	// 发送二进制消息
	return conn.WriteMessage(websocket.BinaryMessage, responseMsg.Bytes())
}

func (l *ChatStreamLogic) ChatStream() (resp *types.ChatResponse, err error) {
	// 这个方法保留用于兼容性，实际的 WebSocket 处理在 HandleWebSocket 中
	return &types.ChatResponse{
		Code:    0,
		Message: "WebSocket endpoint",
		Data: types.ChatData{
			Message: "Use WebSocket connection for real-time chat",
		},
	}, nil
}

// ASR协议消息类型（根据官方文档）
const (
	ASRProtocolFullClientRequest  = 0x01  // 0b0001
	ASRProtocolAudioOnlyRequest   = 0x02  // 0b0010
	ASRProtocolFullServerResponse = 0x09  // 0b1001
	ASRProtocolServerACK          = 0x0B  // 0b1011
	ASRProtocolServerError        = 0x0F  // 0b1111
)

// ASR协议头结构
type ASRProtocolHeader struct {
	Version     uint8
	MessageType uint8
	Serialize   uint8
	Compress    uint8
}

// 处理ASR协议的二进制消息
func (l *ChatStreamLogic) handleASRProtocolMessage(data []byte, config *ConfigMessage, audioStream chan<- []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("message too short for ASR protocol header")
	}

	// 解析协议头（按照七牛云官方协议格式）
	header := ASRProtocolHeader{
		Version:     (data[0] >> 4) & 0x0F, // 第1字节高4位：协议版本
		MessageType: (data[1] >> 4) & 0x0F, // 第2字节高4位：消息类型
		Serialize:   (data[2] >> 4) & 0x0F, // 第3字节高4位：序列化方法
		Compress:    data[2] & 0x0F,        // 第3字节低4位：压缩类型
	}

	// 解析消息类型特定标志
	messageFlags := data[1] & 0x0F

	// 验证版本号
	if header.Version != 1 {
		return fmt.Errorf("unsupported ASR protocol version: %d", header.Version)
	}

	// 获取头部大小并计算当前读取位置
	headerSize := int(data[0] & 0x0F)
	currentPos := headerSize * 4

	if len(data) < currentPos {
		return fmt.Errorf("message too short for declared header size")
	}

	// 检查是否有序列号字段
	hasSequence := (messageFlags & 0x01) != 0
	if hasSequence {
		if len(data) < currentPos+4 {
			return fmt.Errorf("message too short for sequence number")
		}
		// 跳过序列号字段（4字节）
		currentPos += 4
	}

	// 读取负载长度（4字节）
	if len(data) < currentPos+4 {
		return fmt.Errorf("message too short for payload length")
	}
	
	payloadLength := int(uint32(data[currentPos])<<24 | uint32(data[currentPos+1])<<16 | 
					   uint32(data[currentPos+2])<<8 | uint32(data[currentPos+3]))
	currentPos += 4

	// 验证负载长度
	if len(data) < currentPos+payloadLength {
		return fmt.Errorf("message too short for declared payload length")
	}

	// 获取压缩的负载数据
	compressedPayload := data[currentPos : currentPos+payloadLength]

	// 解压缩负载数据（如果需要）
	var payload []byte
	if header.Compress == 1 {
		// GZIP 解压缩
		reader, err := gzip.NewReader(bytes.NewReader(compressedPayload))
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %v", err)
		}
		defer reader.Close()

		payload, err = io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("failed to decompress message: %v", err)
		}
	} else {
		payload = compressedPayload
	}

	// 根据消息类型处理
	switch header.MessageType {
	case ASRProtocolFullClientRequest:
		return l.handleASRFullRequest(payload, config, audioStream)
	case ASRProtocolAudioOnlyRequest:
		return l.handleASRAudioOnlyRequest(payload, audioStream)
	default:
		return fmt.Errorf("unsupported ASR message type: %d", header.MessageType)
	}
}

// 处理完整请求消息（包含配置和音频）
func (l *ChatStreamLogic) handleASRFullRequest(payload []byte, config *ConfigMessage, audioStream chan<- []byte) error {
	// 对于 FULL_CLIENT_REQUEST，负载直接是 JSON 配置
	var request map[string]interface{}
	if err := json.Unmarshal(payload, &request); err != nil {
		return fmt.Errorf("failed to parse ASR request: %v", err)
	}

	// 更新配置（如果提供）
	if userInfo, exists := request["user"]; exists {
		logx.Infof("ASR user info: %v", userInfo)
	}
	
	if audioInfo, exists := request["audio"]; exists {
		if audioMap, ok := audioInfo.(map[string]interface{}); ok {
			// 更新 ASR 配置参数
			if config.Params == nil {
				config.Params = make(map[string]string)
			}
			
			if format, exists := audioMap["format"]; exists {
				if formatStr, ok := format.(string); ok {
					config.Params["audio_format"] = formatStr
				}
			}
			
			if sampleRate, exists := audioMap["sample_rate"]; exists {
				if rate, ok := sampleRate.(float64); ok {
					config.Params["sample_rate"] = fmt.Sprintf("%.0f", rate)
				}
			}
			
			if bits, exists := audioMap["bits"]; exists {
				if bitsVal, ok := bits.(float64); ok {
					config.Params["bits"] = fmt.Sprintf("%.0f", bitsVal)
				}
			}
			
			if channels, exists := audioMap["channel"]; exists {
				if channelVal, ok := channels.(float64); ok {
					config.Params["channels"] = fmt.Sprintf("%.0f", channelVal)
				}
			}
		}
	}

	if requestInfo, exists := request["request"]; exists {
		if reqMap, ok := requestInfo.(map[string]interface{}); ok {
			if modelName, exists := reqMap["model_name"]; exists {
				if model, ok := modelName.(string); ok && model == "asr" {
					// 确认这是ASR请求
					logx.Infof("ASR model confirmed: %s", model)
				}
			}
		}
	}

	logx.Infof("ASR configuration updated successfully")
	return nil
}

// 处理纯音频请求消息
func (l *ChatStreamLogic) handleASRAudioOnlyRequest(payload []byte, audioStream chan<- []byte) error {
	// 对于 AUDIO_ONLY_REQUEST，负载直接是音频数据
	if len(payload) == 0 {
		return nil // 空音频数据，忽略
	}

	logx.Infof("Received audio data: %d bytes", len(payload))
	
	select {
	case audioStream <- payload:
		return nil
	default:
		return fmt.Errorf("audio stream buffer full")
	}
}

// 处理配置消息
func (l *ChatStreamLogic) handleConfig(msg *WSMessage, config *ConfigMessage) error {
	configData, ok := msg.Content.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid config format")
	}

	// 将 map 转换为 ConfigMessage
	configBytes, err := json.Marshal(configData)
	if err != nil {
		return err
	}

	return json.Unmarshal(configBytes, config)
}

// 执行ASR识别
func (l *ChatStreamLogic) performASR(audioData []byte, config *ConfigMessage) (string, error) {
	// 设置默认ASR提供商
	asrProviderName := config.ASRProvider
	if asrProviderName == "" {
		asrProviderName = "iflytek" // 使用iFlytek作为默认ASR
	}
	
	logx.Infof("Using ASR provider: %s (configured: %s)", asrProviderName, config.ASRProvider)
	
	asrProvider, err := l.svcCtx.Registry.GetASR(asrProviderName)
	if err != nil {
		// 如果指定的provider不可用，尝试备选方案
		logx.Errorf("ASR provider '%s' not found: %v, trying fallback", asrProviderName, err)
		
		// 尝试备选provider
		fallbackProviders := []string{"iflytek", "qiniu"}
		for _, fallback := range fallbackProviders {
			if fallback != asrProviderName {
				logx.Infof("Trying fallback ASR provider: %s", fallback)
				asrProvider, err = l.svcCtx.Registry.GetASR(fallback)
				if err == nil {
					asrProviderName = fallback
					break
				}
			}
		}
		
		if err != nil {
			return "", fmt.Errorf("no ASR provider available: %v", err)
		}
	}

	// 使用批量识别接口
	logx.Infof("Calling ASR provider '%s' with %d bytes of audio data", asrProviderName, len(audioData))
	text, err := asrProvider.Recognize(audioData)
	if err != nil {
		return "", fmt.Errorf("ASR recognition failed: %v", err)
	}
	
	logx.Infof("ASR result: %s", text)
	return text, nil
}

// 处理文本到响应的完整流程（LLM + TTS）
func (l *ChatStreamLogic) processTextToResponse(text string, config *ConfigMessage, conn *websocket.Conn) {
	// 发送处理状态
	l.sendMessage(conn, &WSMessage{
		Type:      "status",
		Content:   map[string]interface{}{"status": "processing_llm", "message": "正在生成回复..."},
		Timestamp: time.Now().Unix(),
	})

	// 调用LLM获取回复（流式）
	go l.processLLMStreaming(text, config, conn)
}

// 处理LLM流式生成
func (l *ChatStreamLogic) processLLMStreaming(text string, config *ConfigMessage, conn *websocket.Conn) {
	ctx := context.Background()
	l.processStreamingLLM(ctx, text, config, conn)
}
