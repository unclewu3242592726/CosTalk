package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// 七牛云 LLM Provider 实现
type QiniuLLMProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewQiniuLLMProvider(apiKey string) *QiniuLLMProvider {
	return &QiniuLLMProvider{
		apiKey:  apiKey,
		baseURL: "https://openai.qiniu.com/v1",
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (p *QiniuLLMProvider) Name() string {
	return "qiniu-llm"
}

// 七牛云 API 请求结构（兼容 OpenAI）
type qiniuChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	TopP        float64   `json:"top_p,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream"`
}

// 七牛云 API 响应结构（兼容 OpenAI）
type qiniuChatResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []qiniuChoice    `json:"choices"`
	Usage   *qiniuUsage      `json:"usage,omitempty"`
}

type qiniuChoice struct {
	Index        int      `json:"index"`
	Message      *Message `json:"message,omitempty"`
	Delta        *Message `json:"delta,omitempty"`
	FinishReason string   `json:"finish_reason,omitempty"`
}

type qiniuUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (p *QiniuLLMProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// 转换消息格式
	var messages []Message
	for _, msg := range req.Messages {
		messages = append(messages, *msg)
	}

	// 转换请求格式
	qiniuReq := qiniuChatRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
	}

	// 如果没有指定模型，使用默认模型
	if qiniuReq.Model == "" {
		qiniuReq.Model = "deepseek-v3"
	}

	reqBody, err := json.Marshal(qiniuReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建 HTTP 请求
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	// 发送请求
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var qiniuResp qiniuChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&qiniuResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// 转换为统一格式
	if len(qiniuResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := qiniuResp.Choices[0]
	var usage *Usage
	if qiniuResp.Usage != nil {
		usage = &Usage{
			PromptTokens:     qiniuResp.Usage.PromptTokens,
			CompletionTokens: qiniuResp.Usage.CompletionTokens,
			TotalTokens:      qiniuResp.Usage.TotalTokens,
		}
	}

	return &ChatResponse{
		Text:         choice.Message.Content,
		FinishReason: choice.FinishReason,
		Usage:        usage,
	}, nil
}

func (p *QiniuLLMProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan *ChatDelta, error) {
	// 转换消息格式
	var messages []Message
	for _, msg := range req.Messages {
		messages = append(messages, *msg)
	}

	// 转换请求格式
	qiniuReq := qiniuChatRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Stream:      true,
	}

	// 如果没有指定模型，使用默认模型
	if qiniuReq.Model == "" {
		qiniuReq.Model = "deepseek-v3"
	}

	reqBody, err := json.Marshal(qiniuReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建 HTTP 请求
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	// 发送请求
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// 创建流式响应通道
	deltaStream := make(chan *ChatDelta, 100)

	go func() {
		defer resp.Body.Close()
		defer close(deltaStream)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			
			// 跳过空行和注释行
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}

			// 处理 SSE 数据
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				
				// 结束标记
				if data == "[DONE]" {
					return
				}

				// 解析 JSON 数据
				var streamResp qiniuChatResponse
				if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
					fmt.Printf("Failed to parse stream data: %v\n", err)
					continue
				}

				// 转换为 ChatDelta
				if len(streamResp.Choices) > 0 {
					choice := streamResp.Choices[0]
					var deltaText string
					if choice.Delta != nil {
						deltaText = choice.Delta.Content
					}

					var usage *Usage
					if streamResp.Usage != nil {
						usage = &Usage{
							PromptTokens:     streamResp.Usage.PromptTokens,
							CompletionTokens: streamResp.Usage.CompletionTokens,
							TotalTokens:      streamResp.Usage.TotalTokens,
						}
					}

					delta := &ChatDelta{
						Text:         deltaText,
						FinishReason: choice.FinishReason,
						Usage:        usage,
					}

					select {
					case deltaStream <- delta:
					case <-ctx.Done():
						return
					}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			fmt.Printf("Stream reading error: %v\n", err)
		}
	}()

	return deltaStream, nil
}