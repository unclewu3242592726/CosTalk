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

	"github.com/zeromicro/go-zero/core/logx"
)

type QiniuASRProvider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// HTTP ASR 请求结构
type QiniuHTTPASRRequest struct {
	Model string         `json:"model"`
	Audio QiniuHTTPAudio `json:"audio"`
}

type QiniuHTTPAudio struct {
	Format string `json:"format"`
	Data   string `json:"data"` // base64编码的音频数据
}

// HTTP ASR 响应结构
type QiniuHTTPASRResponse struct {
	ReqID     string             `json:"reqid"`
	Operation string             `json:"operation"`
	Data      QiniuHTTPASRResult `json:"data"`
}

type QiniuHTTPASRResult struct {
	AudioInfo QiniuAudioInfo  `json:"audio_info"`
	Result    QiniuTextResult `json:"result"`
}

type QiniuAudioInfo struct {
	Duration int `json:"duration"`
}

type QiniuTextResult struct {
	Text      string            `json:"text"`
	Additions map[string]string `json:"additions"`
}

func NewQiniuASRProvider(apiKey string) *QiniuASRProvider {
	return &QiniuASRProvider{
		apiKey:  apiKey,
		baseURL: "https://openai.qiniu.com/v1",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *QiniuASRProvider) Name() string {
	return "qiniu"
}

// StreamRecognize 实现ASRProvider接口，但使用HTTP POST而不是WebSocket
func (p *QiniuASRProvider) StreamRecognize(ctx context.Context, audioStream <-chan []byte) (<-chan *Transcript, error) {
	resultChan := make(chan *Transcript, 1)
	
	go func() {
		defer close(resultChan)
		
		// 收集所有音频数据
		var audioData []byte
		for chunk := range audioStream {
			audioData = append(audioData, chunk...)
		}
		
		if len(audioData) == 0 {
			logx.Error("Empty audio data received")
			return
		}
		
		// 调用HTTP ASR API
		text, err := p.recognizeHTTP(audioData)
		if err != nil {
			logx.Errorf("HTTP ASR recognition failed: %v", err)
			return
		}
		
		if text != "" {
			resultChan <- &Transcript{
				Text:    text,
				IsFinal: true,
			}
		}
	}()
	
	return resultChan, nil
}

// recognizeHTTP 使用HTTP POST接口进行语音识别
func (p *QiniuASRProvider) recognizeHTTP(audioData []byte) (string, error) {
	// 将音频数据编码为base64
	audioBase64 := base64.StdEncoding.EncodeToString(audioData)
	
	// 构建请求
	request := QiniuHTTPASRRequest{
		Model: "asr",
		Audio: QiniuHTTPAudio{
			Format: "pcm",
			Data:   audioBase64,
		},
	}
	
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}
	
	// 创建HTTP请求
	url := p.baseURL + "/voice/asr"
	req, err := http.NewRequest("POST", url, bytes.NewReader(requestBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	
	logx.Infof("Calling Qiniu HTTP ASR API: %s", url)
	logx.Infof("Audio data size: %d bytes (base64: %d chars)", len(audioData), len(audioBase64))
	
	// 发送请求
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()
	
	// 读取响应
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}
	
	logx.Infof("ASR Response Status: %d", resp.StatusCode)
	logx.Infof("ASR Response Body: %s", string(responseBody))
	
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}
	
	// 解析响应
	var asrResponse QiniuHTTPASRResponse
	if err := json.Unmarshal(responseBody, &asrResponse); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %v", err)
	}
	
	return asrResponse.Data.Result.Text, nil
}

// Recognize 实现ASRProvider接口的批量识别方法
func (p *QiniuASRProvider) Recognize(audioData []byte) (string, error) {
	return p.recognizeHTTP(audioData)
}