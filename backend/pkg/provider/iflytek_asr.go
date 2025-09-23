package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// 科大讯飞 ASR Provider 实现
type IflytekASRProvider struct {
	appID     string
	apiSecret string
	apiKey    string
	baseURL   string
}

func NewIflytekASRProvider(appID, apiSecret, apiKey string) *IflytekASRProvider {
	return &IflytekASRProvider{
		appID:     appID,
		apiSecret: apiSecret,
		apiKey:    apiKey,
		baseURL:   "wss://ws-api.xfyun.cn/v2/iat",
	}
}

func (p *IflytekASRProvider) Name() string {
	return "iflytek-asr"
}

// 科大讯飞 ASR 请求参数
type iflytekASRParams struct {
	Common   iflytekCommon   `json:"common"`
	Business iflytekBusiness `json:"business"`
	Data     iflytekData     `json:"data"`
}

type iflytekCommon struct {
	AppID string `json:"app_id"`
}

type iflytekBusiness struct {
	Language string `json:"language"`
	Domain   string `json:"domain"`
	Accent   string `json:"accent"`
	VInfo    int    `json:"vinfo"`
	VadEos   int    `json:"vad_eos"`
}

type iflytekData struct {
	Status int    `json:"status"`
	Format string `json:"format"`
	Audio  string `json:"audio"`
}

// 科大讯飞 ASR 响应
type iflytekASRResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Sid     string                 `json:"sid"`
	Data    iflytekASRResponseData `json:"data"`
}

type iflytekASRResponseData struct {
	Result iflytekASRResult `json:"result"`
	Status int              `json:"status"`
}

type iflytekASRResult struct {
	Sn int                     `json:"sn"`
	Ls bool                    `json:"ls"`
	Bg int                     `json:"bg"`
	Ed int                     `json:"ed"`
	Ws []iflytekASRResultWord `json:"ws"`
}

type iflytekASRResultWord struct {
	Bg int                  `json:"bg"`
	Cw []iflytekASRWordChar `json:"cw"`
}

type iflytekASRWordChar struct {
	W  string `json:"w"`
	Wp string `json:"wp"`
}

func (p *IflytekASRProvider) StreamRecognize(ctx context.Context, audioStream <-chan []byte) (<-chan *Transcript, error) {
	// 生成鉴权 URL
	authURL, err := p.generateAuthURL()
	if err != nil {
		return nil, fmt.Errorf("failed to generate auth URL: %w", err)
	}

	// 建立 WebSocket 连接
	conn, _, err := websocket.DefaultDialer.Dial(authURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to websocket: %w", err)
	}

	transcriptChan := make(chan *Transcript, 100)

	go func() {
		defer conn.Close()
		defer close(transcriptChan)

		// 处理音频流
		go func() {
			defer conn.WriteMessage(websocket.CloseMessage, []byte{})
			
			first := true
			for {
				select {
				case audioData, ok := <-audioStream:
					if !ok {
						// 发送结束帧
						endFrame := iflytekASRParams{
							Common: iflytekCommon{AppID: p.appID},
							Business: iflytekBusiness{
								Language: "zh_cn",
								Domain:   "iat",
								Accent:   "mandarin",
								VInfo:    1,
								VadEos:   10000,
							},
							Data: iflytekData{
								Status: 2, // 结束
								Format: "audio/L16;rate=16000",
								Audio:  "",
							},
						}
						conn.WriteJSON(endFrame)
						return
					}

					status := 1 // 中间
					if first {
						status = 0 // 开始
						first = false
					}

					// 音频数据 base64 编码
					audioB64 := base64.StdEncoding.EncodeToString(audioData)

					frame := iflytekASRParams{
						Common: iflytekCommon{AppID: p.appID},
						Business: iflytekBusiness{
							Language: "zh_cn",
							Domain:   "iat",
							Accent:   "mandarin",
							VInfo:    1,
							VadEos:   10000,
						},
						Data: iflytekData{
							Status: status,
							Format: "audio/L16;rate=16000",
							Audio:  audioB64,
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
			var response iflytekASRResponse
			if err := conn.ReadJSON(&response); err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					return
				}
				fmt.Printf("Failed to read from websocket: %v\n", err)
				return
			}

			if response.Code != 0 {
				fmt.Printf("ASR error: %s\n", response.Message)
				continue
			}

			// 解析识别结果
			if len(response.Data.Result.Ws) > 0 {
				var text strings.Builder
				for _, word := range response.Data.Result.Ws {
					for _, char := range word.Cw {
						text.WriteString(char.W)
					}
				}

				transcript := &Transcript{
					Text:       text.String(),
					IsFinal:    response.Data.Result.Ls, // ls: 是否为最后一片结果
					Confidence: 0.9, // 科大讯飞没有直接提供置信度，使用默认值
				}

				select {
				case transcriptChan <- transcript:
				case <-ctx.Done():
					return
				}
			}

			// 如果是最终结果且状态为2，结束
			if response.Data.Status == 2 {
				return
			}
		}
	}()

	return transcriptChan, nil
}

func (p *IflytekASRProvider) generateAuthURL() (string, error) {
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