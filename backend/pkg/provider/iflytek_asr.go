package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zeromicro/go-zero/core/logx"
)

// IflytekASRProvider 科大讯飞语音识别提供商 (WebSocket批量转写)
type IflytekASRProvider struct {
	appID     string
	apiSecret string
	apiKey    string
	baseURL   string
}

// NewIflytekASRProvider 创建科大讯飞ASR提供商
func NewIflytekASRProvider(appID, apiSecret, apiKey string) *IflytekASRProvider {
	logx.Infof("Creating iFlytek ASR Provider with AppID: '%s', APISecret: '%s', APIKey: '%s'", 
		appID, apiSecret, apiKey)
	return &IflytekASRProvider{
		appID:     appID,
		apiSecret: apiSecret,
		apiKey:    apiKey,
		baseURL:   "wss://iat-api.xfyun.cn/v2/iat", // WebSocket语音听写API
	}
}

// iFlytek WebSocket 请求/响应结构
type IflytekMessage struct {
	Common   *IflytekCommon   `json:"common,omitempty"`
	Business *IflytekBusiness `json:"business,omitempty"`
	Data     *IflytekData     `json:"data,omitempty"`
}

type IflytekCommon struct {
	AppID string `json:"app_id"`
}

type IflytekBusiness struct {
	Language string `json:"language"`
	Domain   string `json:"domain"`
	Accent   string `json:"accent"`
	VadEos   int    `json:"vad_eos"`
	Dwa      string `json:"dwa,omitempty"`
}

type IflytekData struct {
	Status   int    `json:"status"`
	Format   string `json:"format"`
	Encoding string `json:"encoding"`
	Audio    string `json:"audio,omitempty"`
}

type IflytekResponse struct {
	Code    int                   `json:"code"`
	Message string               `json:"message"`
	Sid     string               `json:"sid"`
	Data    *IflytekResponseData `json:"data"`
}

type IflytekResponseData struct {
	Status int                  `json:"status"`
	Result *IflytekResultData   `json:"result"`
}

type IflytekResultData struct {
	Sn int                    `json:"sn"`
	Ls bool                   `json:"ls"`
	Ws []IflytekWordData     `json:"ws"`
}

type IflytekWordData struct {
	Bg int                    `json:"bg"`
	Cw []IflytekCharData     `json:"cw"`
}

type IflytekCharData struct {
	W string `json:"w"`
}

// Recognize 实现ASRProvider接口的批量识别方法
func (p *IflytekASRProvider) Recognize(audioData []byte) (string, error) {
	logx.Infof("iFlytek WebSocket ASR starting recognition, audio size: %d bytes", len(audioData))
	
	// 生成签名认证URL
	authURL, err := p.generateAuthURL()
	if err != nil {
		return "", fmt.Errorf("failed to generate auth URL: %v", err)
	}
	
	logx.Infof("Connecting to iFlytek WebSocket: %s", authURL)
	
	// 建立WebSocket连接
	dialer := websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
	}
	
	conn, _, err := dialer.Dial(authURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to connect to iFlytek WebSocket: %v", err)
	}
	defer conn.Close()
	
	logx.Infof("Successfully connected to iFlytek WebSocket")
	
	// 发送第一帧（带配置参数）
	firstFrame := IflytekMessage{
		Common: &IflytekCommon{
			AppID: p.appID,
		},
		Business: &IflytekBusiness{
			Language: "zh_cn",
			Domain:   "iat",
			Accent:   "mandarin",
			VadEos:   10000, // 10秒后端点检测
		},
		Data: &IflytekData{
			Status:   0, // 第一帧
			Format:   "audio/L16;rate=16000",
			Encoding: "raw",
			Audio:    base64.StdEncoding.EncodeToString(audioData),
		},
	}
	
	firstFrameData, err := json.Marshal(firstFrame)
	if err != nil {
		return "", fmt.Errorf("failed to marshal first frame: %v", err)
	}
	
	if err := conn.WriteMessage(websocket.TextMessage, firstFrameData); err != nil {
		return "", fmt.Errorf("failed to send first frame: %v", err)
	}
	
	logx.Infof("Sent first frame with audio data")
	
	// 发送结束帧
	endFrame := IflytekMessage{
		Data: &IflytekData{
			Status: 2, // 最后一帧
		},
	}
	
	endFrameData, err := json.Marshal(endFrame)
	if err != nil {
		return "", fmt.Errorf("failed to marshal end frame: %v", err)
	}
	
	if err := conn.WriteMessage(websocket.TextMessage, endFrameData); err != nil {
		return "", fmt.Errorf("failed to send end frame: %v", err)
	}
	
	logx.Infof("Sent end frame")
	
	// 接收识别结果
	var finalText strings.Builder
	
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			logx.Errorf("Error reading message: %v", err)
			break
		}
		
		logx.Infof("Raw iFlytek response: %s", string(message))
		
		var response IflytekResponse
		if err := json.Unmarshal(message, &response); err != nil {
			logx.Errorf("Failed to unmarshal response: %v", err)
			continue
		}
		
		if response.Code != 0 {
			return "", fmt.Errorf("iFlytek ASR error: code=%d, message=%s", response.Code, response.Message)
		}
		
		if response.Data != nil && response.Data.Result != nil {
			// 提取文字
			for _, word := range response.Data.Result.Ws {
				for _, char := range word.Cw {
					finalText.WriteString(char.W)
				}
			}
			
			// 检查是否是最后一个结果
			if response.Data.Status == 2 {
				logx.Infof("Received final result, closing connection")
				break
			}
		}
	}
	
	result := finalText.String()
	logx.Infof("iFlytek ASR final result: %s", result)
	
	return result, nil
}

// generateAuthURL 生成带认证的WebSocket URL
func (p *IflytekASRProvider) generateAuthURL() (string, error) {
	ul, err := url.Parse(p.baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %v", err)
	}
	
	// 生成签名时间
	date := time.Now().UTC().Format(time.RFC1123)
	
	// 参与签名的字段
	signString := []string{
		"host: " + ul.Host,
		"date: " + date,
		"GET " + ul.Path + " HTTP/1.1",
	}
	
	// 拼接签名字符串
	sgin := strings.Join(signString, "\n")
	
	// 使用HMAC-SHA256生成签名
	h := hmac.New(sha256.New, []byte(p.apiSecret))
	h.Write([]byte(sgin))
	sha := base64.StdEncoding.EncodeToString(h.Sum(nil))
	
	// 构建请求参数
	authUrl := fmt.Sprintf(`api_key="%s", algorithm="hmac-sha256", headers="host date request-line", signature="%s"`, 
		p.apiKey, sha)
	
	// base64编码
	authorization := base64.StdEncoding.EncodeToString([]byte(authUrl))
	
	v := url.Values{}
	v.Add("host", ul.Host)
	v.Add("date", date)
	v.Add("authorization", authorization)
	
	// 将编码后的字符串添加到URL
	callurl := p.baseURL + "?" + v.Encode()
	return callurl, nil
}

// Name 返回提供商名称 (实现ASRProvider接口)
func (p *IflytekASRProvider) Name() string {
	return "iFlytek"
}

// StreamRecognize 实现流式识别接口（暂时不支持）
func (p *IflytekASRProvider) StreamRecognize(ctx context.Context, audioStream <-chan []byte) (<-chan *Transcript, error) {
	return nil, fmt.Errorf("stream recognize not implemented for iFlytek batch ASR")
}