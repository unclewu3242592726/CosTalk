package provider

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zeromicro/go-zero/core/logx"
)

type QiniuASRProvider struct {
	apiKey     string
	baseURL    string
	wsURL      string
	httpClient *http.Client
}

// 七牛云 ASR WebSocket 配置请求
type QiniuASRConfig struct {
	User    QiniuUser      `json:"user"`
	Audio   QiniuWSAudio   `json:"audio"`
	Request QiniuWSRequest `json:"request"`
}

type QiniuUser struct {
	UID string `json:"uid"`
}

type QiniuWSAudio struct {
	Format     string `json:"format"`
	SampleRate int    `json:"sample_rate"`
	Bits       int    `json:"bits"`
	Channel    int    `json:"channel"`
	Codec      string `json:"codec"`
}

type QiniuWSRequest struct {
	ModelName  string `json:"model_name"`
	EnablePunc bool   `json:"enable_punc"`
}

// 七牛云 ASR 响应
type QiniuASRResult struct {
	Result QiniuResult `json:"result"`
}

type QiniuResult struct {
	Text string `json:"text"`
}

// WebSocket 协议常量
const (
	PROTOCOL_VERSION = 0x01

	// Message Types
	FULL_CLIENT_REQUEST  = 0x01
	AUDIO_ONLY_REQUEST   = 0x02
	FULL_SERVER_RESPONSE = 0x09
	SERVER_ACK           = 0x0B

	// Flags
	POS_SEQUENCE = 0x01

	// Serialization
	NO_SERIALIZATION   = 0x00
	JSON_SERIALIZATION = 0x01
	
	// Compression
	NO_COMPRESSION   = 0x00
	GZIP_COMPRESSION = 0x01
)

func NewQiniuASRProvider(apiKey string) *QiniuASRProvider {
	return &QiniuASRProvider{
		apiKey:     apiKey,
		baseURL:    "https://openai.qiniu.com/v1",
		wsURL:      "wss://openai.qiniu.com/v1/voice/asr",
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *QiniuASRProvider) Name() string {
	return "qiniu-asr"
}

// 实现 ASRProvider 接口中的 StreamRecognize 方法
func (p *QiniuASRProvider) StreamRecognize(ctx context.Context, audioStream <-chan []byte) (<-chan *Transcript, error) {
	resultChan := make(chan *Transcript, 10)

	go func() {
		defer close(resultChan)

		// 连接 WebSocket - 尝试不同的认证方式
		headers := http.Header{}
		headers.Set("Authorization", "Bearer "+p.apiKey)
		// 尝试添加额外的头部（某些API可能需要）
		headers.Set("User-Agent", "CosTalk/1.0")
		headers.Set("Accept", "*/*")
		
		logx.Infof("Connecting to ASR WebSocket: %s", p.wsURL)
		logx.Infof("Using API Key: %s...%s", p.apiKey[:10], p.apiKey[len(p.apiKey)-10:])
		logx.Infof("Headers: %v", headers)

		conn, response, err := websocket.DefaultDialer.Dial(p.wsURL, headers)
		if err != nil {
			logx.Errorf("WebSocket dial failed: %v", err)
			if response != nil {
				logx.Errorf("HTTP response status: %s", response.Status)
				logx.Errorf("HTTP response headers: %v", response.Header)
				// 尝试读取响应体获取更多错误信息
				if response.Body != nil {
					body, readErr := io.ReadAll(response.Body)
					if readErr == nil {
						logx.Errorf("HTTP response body: %s", string(body))
					}
				}
			}
			return
		}
		defer conn.Close()

		// 发送配置信息
		if err := p.sendConfig(conn); err != nil {
			logx.Errorf("Send config failed: %v", err)
			return
		}

		// 启动消息接收 goroutine
		go p.handleMessages(ctx, conn, resultChan)

		// 发送音频数据
		seq := 2
		for {
			select {
			case <-ctx.Done():
				return
			case audioData, ok := <-audioStream:
				if !ok {
					return // 音频流结束
				}

				if err := p.sendAudioData(conn, audioData, seq); err != nil {
					logx.Errorf("Send audio failed: %v", err)
					return
				}
				seq++
			}
		}
	}()

	return resultChan, nil
}

// 发送配置信息
func (p *QiniuASRProvider) sendConfig(conn *websocket.Conn) error {
	config := QiniuASRConfig{
		User: QiniuUser{
			UID: fmt.Sprintf("user-%d", time.Now().Unix()),
		},
		Audio: QiniuWSAudio{
			Format:     "pcm",
			SampleRate: 16000,
			Bits:       16,
			Channel:    1,
			Codec:      "raw",
		},
		Request: QiniuWSRequest{
			ModelName:  "asr",
			EnablePunc: true,
		},
	}

	// 序列化为JSON
	payload, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config failed: %v", err)
	}

	// GZIP 压缩 payload
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	if _, err := gzipWriter.Write(payload); err != nil {
		return fmt.Errorf("gzip compress failed: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("gzip close failed: %v", err)
	}
	compressedPayload := buf.Bytes()

	// 按照官方协议构建消息
	header := p.generateHeader(FULL_CLIENT_REQUEST, POS_SEQUENCE, JSON_SERIALIZATION, GZIP_COMPRESSION)
	sequence := p.int32ToBytes(1) // 序列号为1
	payloadLength := p.int32ToBytes(len(compressedPayload))

	// 完整消息：协议头 + 序列号 + 负载长度 + 负载数据
	message := make([]byte, 0, len(header)+len(sequence)+len(payloadLength)+len(compressedPayload))
	message = append(message, header...)
	message = append(message, sequence...)
	message = append(message, payloadLength...)
	message = append(message, compressedPayload...)

	logx.Infof("Sending ASR config, header: %x, seq: %d, payload_len: %d, total_len: %d", 
		header, 1, len(compressedPayload), len(message))

	return conn.WriteMessage(websocket.BinaryMessage, message)
}

// 发送音频数据
func (p *QiniuASRProvider) sendAudioData(conn *websocket.Conn, audioData []byte, seq int) error {
	// GZIP 压缩音频数据
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	if _, err := gzipWriter.Write(audioData); err != nil {
		return fmt.Errorf("gzip compress audio failed: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("gzip close failed: %v", err)
	}
	compressedAudio := buf.Bytes()

	// 音频数据使用 AUDIO_ONLY_REQUEST 类型，不使用JSON序列化
	header := p.generateHeader(AUDIO_ONLY_REQUEST, POS_SEQUENCE, NO_SERIALIZATION, GZIP_COMPRESSION)
	sequence := p.int32ToBytes(seq)
	payloadLength := p.int32ToBytes(len(compressedAudio))

	// 完整消息：协议头 + 序列号 + 负载长度 + 负载数据
	message := make([]byte, 0, len(header)+len(sequence)+len(payloadLength)+len(compressedAudio))
	message = append(message, header...)
	message = append(message, sequence...)
	message = append(message, payloadLength...)
	message = append(message, compressedAudio...)

	logx.Debugf("Sending audio data, seq: %d, audio_len: %d, compressed_len: %d, total_len: %d", 
		seq, len(audioData), len(compressedAudio), len(message))

	return conn.WriteMessage(websocket.BinaryMessage, message)
}

// 处理服务器消息
func (p *QiniuASRProvider) handleMessages(ctx context.Context, conn *websocket.Conn, resultChan chan<- *Transcript) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				logx.Errorf("Read message failed: %v", err)
				return
			}

			transcript := p.parseMessage(message)
			if transcript != nil {
				resultChan <- transcript
			}
		}
	}
}

// 解析服务器消息
func (p *QiniuASRProvider) parseMessage(data []byte) *Transcript {
	if len(data) < 4 {
		logx.Errorf("Message too short: %d bytes", len(data))
		return nil
	}

	// 输出原始数据的十六进制用于调试
	if len(data) <= 32 {
		logx.Infof("Raw message data: %x", data)
	} else {
		logx.Infof("Raw message header: %x...", data[:32])
	}
	
	// 尝试直接解析为JSON（如果是错误消息）
	if data[0] == '{' {
		logx.Infof("Received JSON message: %s", string(data))
		return nil
	}

	// 解析协议头 (按照官方Python示例)
	headerSize := data[0] & 0x0f
	messageType := data[1] >> 4
	messageTypeSpecificFlags := data[1] & 0x0f
	serializationMethod := data[2] >> 4
	messageCompression := data[2] & 0x0f

	logx.Infof("Parsed header: type=%d, flags=%d, serial_method=%d, compression=%d, header_size=%d, total_len=%d", 
		messageType, messageTypeSpecificFlags, serializationMethod, messageCompression, headerSize, len(data))

	payload := data[headerSize*4:]
	logx.Infof("Payload start offset: %d, payload_len: %d", headerSize*4, len(payload))

	// 处理序列号 (如果存在)
	if messageTypeSpecificFlags&0x01 != 0 {
		if len(payload) < 4 {
			logx.Errorf("Payload too short for sequence number")
			return nil
		}
		seq := int32(payload[0])<<24 | int32(payload[1])<<16 | int32(payload[2])<<8 | int32(payload[3])
		logx.Infof("Message sequence: %d", seq)
		payload = payload[4:]
	}

	// 检查是否是最后一个包
	isLastPackage := (messageTypeSpecificFlags & 0x02) != 0
	logx.Infof("Is last package: %v", isLastPackage)

	// 处理不同消息类型的负载长度
	switch messageType {
	case FULL_SERVER_RESPONSE:
		if len(payload) < 4 {
			logx.Errorf("FULL_SERVER_RESPONSE payload too short")
			return nil
		}
		payloadSize := int32(payload[0])<<24 | int32(payload[1])<<16 | int32(payload[2])<<8 | int32(payload[3])
		logx.Debugf("FULL_SERVER_RESPONSE payload size: %d", payloadSize)
		if len(payload) >= 4+int(payloadSize) {
			payload = payload[4 : 4+payloadSize]
		} else {
			payload = payload[4:]
		}
	case SERVER_ACK:
		if len(payload) < 4 {
			logx.Infof("SERVER_ACK received (no payload)")
			return nil // ACK消息可能没有文本内容
		}
		// SERVER_ACK可能包含序列号和可选的负载长度
		if len(payload) >= 8 {
			payloadSize := int32(payload[4])<<24 | int32(payload[5])<<16 | int32(payload[6])<<8 | int32(payload[7])
			logx.Debugf("SERVER_ACK payload size: %d", payloadSize)
			if len(payload) >= 8+int(payloadSize) {
				payload = payload[8 : 8+payloadSize]
			} else {
				payload = payload[8:]
			}
		} else {
			payload = payload[4:]
		}
	default:
		logx.Debugf("Unknown message type: %d", messageType)
	}

	// GZIP 解压缩
	if messageCompression == GZIP_COMPRESSION {
		reader, err := gzip.NewReader(bytes.NewReader(payload))
		if err != nil {
			logx.Errorf("Failed to create gzip reader: %v", err)
			return nil
		}
		defer reader.Close()

		decompressed, err := io.ReadAll(reader)
		if err != nil {
			logx.Errorf("Failed to decompress payload: %v", err)
			return nil
		}
		payload = decompressed
		logx.Debugf("Decompressed payload: %s", string(payload))
	}

	// JSON 反序列化
	if serializationMethod == JSON_SERIALIZATION {
		// 尝试解析标准响应格式
		var result map[string]interface{}
		if err := json.Unmarshal(payload, &result); err != nil {
			logx.Errorf("Failed to unmarshal JSON: %v", err)
			return nil
		}

		// 提取文本内容
		if resultData, ok := result["result"].(map[string]interface{}); ok {
			if text, ok := resultData["text"].(string); ok && text != "" {
				return &Transcript{
					Text:       text,
					IsFinal:    true,
					Confidence: 0.95,
				}
			}
		}

		// 兼容其他可能的响应格式
		if payloadMsg, ok := result["payload_msg"].(map[string]interface{}); ok {
			if resultData, ok := payloadMsg["result"].(map[string]interface{}); ok {
				if text, ok := resultData["text"].(string); ok && text != "" {
					return &Transcript{
						Text:       text,
						IsFinal:    true,
						Confidence: 0.95,
					}
				}
			}
		}
	} else {
		// 直接作为文本处理
		text := string(payload)
		if text != "" {
			return &Transcript{
				Text:       text,
				IsFinal:    true,
				Confidence: 0.95,
			}
		}
	}

	return nil
}

// 生成协议头
func (p *QiniuASRProvider) generateHeader(messageType, messageTypeSpecificFlags, serialMethod, compressionType byte) []byte {
	header := make([]byte, 4)
	headerSize := byte(1)
	
	// 第1字节：协议版本(高4位) + 头长度(低4位)
	header[0] = (PROTOCOL_VERSION << 4) | headerSize
	
	// 第2字节：消息类型(高4位) + 消息特定标志(低4位)
	header[1] = (messageType << 4) | messageTypeSpecificFlags
	
	// 第3字节：序列化方法(高4位) + 压缩类型(低4位)
	header[2] = (serialMethod << 4) | compressionType
	
	// 第4字节：保留字段
	header[3] = 0x00
	
	return header
}

// int32 转字节数组（大端序）
func (p *QiniuASRProvider) int32ToBytes(value int) []byte {
	bytes := make([]byte, 4)
	bytes[0] = byte(value >> 24)
	bytes[1] = byte(value >> 16)
	bytes[2] = byte(value >> 8)
	bytes[3] = byte(value)
	return bytes
}