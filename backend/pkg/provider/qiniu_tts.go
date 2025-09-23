package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zeromicro/go-zero/core/logx"
)

type QiniuTTSProvider struct {
	apiKey     string
	baseURL    string
	wsURL      string
	httpClient *http.Client
}

// 七牛云 TTS 请求结构
type QiniuTTSRequest struct {
	Audio   QiniuTTSAudio   `json:"audio"`
	Request QiniuTTSContent `json:"request"`
}

type QiniuTTSAudio struct {
	VoiceType  string  `json:"voice_type"`
	Encoding   string  `json:"encoding"`
	SpeedRatio float64 `json:"speed_ratio"`
}

type QiniuTTSContent struct {
	Text string `json:"text"`
}

// 七牛云 TTS 响应结构
type QiniuTTSResponse struct {
	ReqID     string           `json:"reqid"`
	Operation string           `json:"operation"`
	Sequence  int              `json:"sequence"`
	Data      string           `json:"data"`      // base64 编码的音频数据
	Addition  QiniuTTSAddition `json:"addition"`
}

type QiniuTTSAddition struct {
	Duration string `json:"duration"`
}

// 音色列表响应结构
type QiniuVoice struct {
	VoiceName  string `json:"voice_name"`
	VoiceType  string `json:"voice_type"`
	URL        string `json:"url"`
	Category   string `json:"category"`
	UpdateTime int64  `json:"updatetime"`
}

func NewQiniuTTSProvider(apiKey string) *QiniuTTSProvider {
	return &QiniuTTSProvider{
		apiKey:     apiKey,
		baseURL:    "https://openai.qiniu.com/v1",
		wsURL:      "wss://api.qnaigc.com/v1/voice/tts", // 使用官方示例的URL
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *QiniuTTSProvider) Name() string {
	return "qiniu-tts"
}

// 实现 TTSProvider 接口中的 SynthesizeStream 方法
func (p *QiniuTTSProvider) SynthesizeStream(ctx context.Context, textStream <-chan string, opts *TTSOptions) (<-chan *AudioChunk, error) {
	resultChan := make(chan *AudioChunk, 10)

	go func() {
		defer close(resultChan)

		for {
			select {
			case <-ctx.Done():
				return
			case text, ok := <-textStream:
				if !ok {
					return // 文本流结束
				}

				// 使用 WebSocket 进行流式合成
				err := p.synthesizeStreamWS(ctx, text, opts, resultChan)
				if err != nil {
					logx.Errorf("TTS WebSocket synthesis failed: %v", err)
					continue
				}
			}
		}
	}()

	return resultChan, nil
}

// 基于 WebSocket 的流式合成（参考官方 Golang 示例）
func (p *QiniuTTSProvider) synthesizeStreamWS(ctx context.Context, text string, opts *TTSOptions, resultChan chan<- *AudioChunk) error {
	// 设置 WebSocket 连接头
	header := http.Header{
		"Authorization": []string{fmt.Sprintf("Bearer %s", p.apiKey)},
	}
	
	// 如果提供了音色类型，添加到头部
	if opts != nil && opts.Voice != "" {
		header.Set("VoiceType", opts.Voice)
	}

	// 建立 WebSocket 连接
	conn, _, err := websocket.DefaultDialer.Dial(p.wsURL, header)
	if err != nil {
		return fmt.Errorf("failed to dial WebSocket: %v", err)
	}
	defer conn.Close()

	// 构建 TTS 请求
	voice := "qiniu_zh_female_wwxkjx" // 默认音色
	encoding := "mp3"                // 默认编码
	speedRatio := 1.0               // 默认语速

	if opts != nil {
		if opts.Voice != "" {
			voice = opts.Voice
		}
		if opts.Speed > 0 {
			speedRatio = opts.Speed
		}
	}

	request := QiniuTTSRequest{
		Audio: QiniuTTSAudio{
			VoiceType:  voice,
			Encoding:   encoding,
			SpeedRatio: speedRatio,
		},
		Request: QiniuTTSContent{
			Text: text,
		},
	}

	// 序列化请求为 JSON
	requestData, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal TTS request: %v", err)
	}

	// 发送请求（使用 BinaryMessage 发送 JSON 数据，参考官方示例）
	err = conn.WriteMessage(websocket.BinaryMessage, requestData)
	if err != nil {
		return fmt.Errorf("failed to send TTS request: %v", err)
	}

	// 接收响应
	seqNum := 0
	var audioBuffer bytes.Buffer

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 读取消息
		_, message, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("failed to read message: %v", err)
		}

		// 解析响应
		var response QiniuTTSResponse
		err = json.Unmarshal(message, &response)
		if err != nil {
			logx.Errorf("Failed to unmarshal TTS response: %v", err)
			continue
		}

		// 解码音频数据
		audioData, err := base64.StdEncoding.DecodeString(response.Data)
		if err != nil {
			logx.Errorf("Failed to decode audio data: %v", err)
			continue
		}

		// 累积音频数据
		audioBuffer.Write(audioData)

		// 发送音频块
		if len(audioData) > 0 {
			select {
			case resultChan <- &AudioChunk{
				Data:   audioData,
				Format: encoding,
				SeqNum: seqNum,
			}:
			case <-ctx.Done():
				return ctx.Err()
			}
			seqNum++
		}

		// 检查是否是最后一个数据包（sequence < 0 表示结束）
		if response.Sequence < 0 {
			logx.Infof("TTS synthesis completed, total audio size: %d bytes", audioBuffer.Len())
			break
		}
	}

	return nil
}

// 文本转语音的核心实现
func (p *QiniuTTSProvider) synthesizeText(ctx context.Context, text string, opts *TTSOptions) (*AudioChunk, error) {
	if opts == nil {
		opts = &TTSOptions{
			Voice: "qiniu_zh_female_wwxkjx", // 默认音色
			Speed: 1.0,
		}
	}

	reqData := QiniuTTSRequest{
		Audio: QiniuTTSAudio{
			VoiceType:  opts.Voice,
			Encoding:   "mp3",
			SpeedRatio: opts.Speed,
		},
		Request: QiniuTTSContent{
			Text: text,
		},
	}

	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/voice/tts", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %d - %s", resp.StatusCode, string(body))
	}

	var qiniuResp QiniuTTSResponse
	if err := json.NewDecoder(resp.Body).Decode(&qiniuResp); err != nil {
		return nil, fmt.Errorf("decode response failed: %v", err)
	}

	// 解码 base64 音频数据
	audioData, err := base64.StdEncoding.DecodeString(qiniuResp.Data)
	if err != nil {
		return nil, fmt.Errorf("decode audio data failed: %v", err)
	}

	return &AudioChunk{
		Data:   audioData,
		Format: "mp3",
		SeqNum: 1,
	}, nil
}

// 获取支持的音色列表（辅助方法）
func (p *QiniuTTSProvider) GetVoices(ctx context.Context) ([]QiniuVoice, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/voice/list", nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %d - %s", resp.StatusCode, string(body))
	}

	var voices []QiniuVoice
	if err := json.NewDecoder(resp.Body).Decode(&voices); err != nil {
		return nil, fmt.Errorf("decode response failed: %v", err)
	}

	return voices, nil
}