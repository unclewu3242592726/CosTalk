package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

// 科大讯飞 TTS Provider 实现
type IflytekTTSProvider struct {
	appID     string
	apiSecret string
	apiKey    string
	baseURL   string
}

func NewIflytekTTSProvider(appID, apiSecret, apiKey string) *IflytekTTSProvider {
	return &IflytekTTSProvider{
		appID:     appID,
		apiSecret: apiSecret,
		apiKey:    apiKey,
		baseURL:   "wss://tts-api.xfyun.cn/v2/tts",
	}
}

func (p *IflytekTTSProvider) Name() string {
	return "iflytek-tts"
}

// 科大讯飞 TTS 请求参数
type iflytekTTSParams struct {
	Common   iflytekCommonTTS   `json:"common"`
	Business iflytekTTSBusiness `json:"business"`
	Data     iflytekTTSData     `json:"data"`
}

type iflytekCommonTTS struct {
	AppID string `json:"app_id"`
}

type iflytekTTSBusiness struct {
	Aue   string `json:"aue"`   // 音频编码格式
	Auf   string `json:"auf"`   // 音频采样率
	Vcn   string `json:"vcn"`   // 发音人
	Speed int    `json:"speed"` // 语速
	Volume int   `json:"volume"` // 音量
	Pitch int    `json:"pitch"`  // 音调
	Bgs   int    `json:"bgs"`    // 背景音乐
	Tte   string `json:"tte"`    // 文本编码格式
}

type iflytekTTSData struct {
	Status int    `json:"status"`
	Text   string `json:"text"`
}

// 科大讯飞 TTS 响应
type iflytekTTSResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Sid     string                 `json:"sid"`
	Data    iflytekTTSResponseData `json:"data"`
}

type iflytekTTSResponseData struct {
	Audio  string `json:"audio"`
	Status int    `json:"status"`
	Ced    string `json:"ced"`
}

func (p *IflytekTTSProvider) SynthesizeStream(ctx context.Context, textStream <-chan string, opts *TTSOptions) (<-chan *AudioChunk, error) {
	// 生成鉴权 URL
	authURL, err := p.generateTTSAuthURL()
	if err != nil {
		return nil, fmt.Errorf("failed to generate auth URL: %w", err)
	}

	// 建立 WebSocket 连接
	conn, _, err := websocket.DefaultDialer.Dial(authURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to websocket: %w", err)
	}

	audioChan := make(chan *AudioChunk, 100)
	seqNum := 0

	go func() {
		defer conn.Close()
		defer close(audioChan)

		// 处理文本流
		go func() {
			defer func() {
				// 发送结束帧
				endFrame := iflytekTTSParams{
					Common: iflytekCommonTTS{AppID: p.appID},
					Business: p.getTTSBusiness(opts),
					Data: iflytekTTSData{
						Status: 2, // 结束
						Text:   "",
					},
				}
				conn.WriteJSON(endFrame)
				conn.WriteMessage(websocket.CloseMessage, []byte{})
			}()
			
			for {
				select {
				case text, ok := <-textStream:
					if !ok {
						return
					}

					// 文本 base64 编码
					textB64 := base64.StdEncoding.EncodeToString([]byte(text))

					frame := iflytekTTSParams{
						Common: iflytekCommonTTS{AppID: p.appID},
						Business: p.getTTSBusiness(opts),
						Data: iflytekTTSData{
							Status: 1, // 中间数据
							Text:   textB64,
						},
					}

					if err := conn.WriteJSON(frame); err != nil {
						fmt.Printf("Failed to write to websocket: %v\n", err)
						return
					}

				case <-ctx.Done():
					return
				}
			}
		}()

		// 处理响应
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					return
				}
				fmt.Printf("Failed to read from websocket: %v\n", err)
				return
			}

			var response iflytekTTSResponse
			if err := json.Unmarshal(message, &response); err != nil {
				fmt.Printf("Failed to unmarshal response: %v\n", err)
				continue
			}

			if response.Code != 0 {
				fmt.Printf("TTS error: %s\n", response.Message)
				continue
			}

			// 解码音频数据
			if response.Data.Audio != "" {
				audioData, err := base64.StdEncoding.DecodeString(response.Data.Audio)
				if err != nil {
					fmt.Printf("Failed to decode audio: %v\n", err)
					continue
				}

				audioChunk := &AudioChunk{
					Data:   audioData,
					Format: "pcm", // 科大讯飞默认返回 PCM 格式
					SeqNum: seqNum,
				}
				seqNum++

				select {
				case audioChan <- audioChunk:
				case <-ctx.Done():
					return
				}
			}

			// 如果是最终结果，结束
			if response.Data.Status == 2 {
				return
			}
		}
	}()

	return audioChan, nil
}

func (p *IflytekTTSProvider) getTTSBusiness(opts *TTSOptions) iflytekTTSBusiness {
	business := iflytekTTSBusiness{
		Aue:    "raw",      // 原始音频
		Auf:    "audio/L16;rate=16000", // 16k采样率
		Vcn:    "xiaoyan",  // 默认发音人
		Speed:  50,         // 默认语速
		Volume: 50,         // 默认音量
		Pitch:  50,         // 默认音调
		Bgs:    0,          // 无背景音乐
		Tte:    "UTF8",     // 文本编码格式
	}

	if opts != nil {
		if opts.Voice != "" {
			business.Vcn = opts.Voice
		}
		if opts.Speed > 0 {
			business.Speed = int(opts.Speed * 100) // 转换为0-100范围
		}
	}

	return business
}

func (p *IflytekTTSProvider) generateTTSAuthURL() (string, error) {
	u, err := url.Parse(p.baseURL)
	if err != nil {
		return "", err
	}

	// 生成RFC1123格式的时间戳
	date := time.Now().UTC().Format(time.RFC1123)

	// 生成签名字符串
	signatureOrigin := fmt.Sprintf("host: %s\ndate: %s\nGET %s HTTP/1.1", u.Host, date, u.Path)

	// HMAC-SHA256 签名
	h := hmac.New(sha256.New, []byte(p.apiSecret))
	h.Write([]byte(signatureOrigin))
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))

	// 生成 authorization 字符串
	authorizationOrigin := fmt.Sprintf(`api_key="%s", algorithm="hmac-sha256", headers="host date request-line", signature="%s"`, p.apiKey, signature)
	authorization := base64.StdEncoding.EncodeToString([]byte(authorizationOrigin))

	// 生成最终的 URL
	v := url.Values{}
	v.Add("authorization", authorization)
	v.Add("date", date)
	v.Add("host", u.Host)

	return p.baseURL + "?" + v.Encode(), nil
}