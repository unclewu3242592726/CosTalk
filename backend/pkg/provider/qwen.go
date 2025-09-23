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

// 通义千问 LLM Provider 实现
type QwenLLMProvider struct {
	apiKey string
	baseURL string
	model  string
	client *http.Client
}

func NewQwenLLMProvider(apiKey string) *QwenLLMProvider {
	return &QwenLLMProvider{
		apiKey:  apiKey,
		baseURL: "https://dashscope.aliyuncs.com/api/v1/services/aigc/text-generation/generation",
		model:   "qwen-turbo",
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (p *QwenLLMProvider) Name() string {
	return "qwen"
}

// 通义千问请求结构
type qwenRequest struct {
	Model      string        `json:"model"`
	Input      qwenInput     `json:"input"`
	Parameters qwenParams    `json:"parameters"`
}

type qwenInput struct {
	Messages []qwenMessage `json:"messages"`
}

type qwenMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type qwenParams struct {
	ResultFormat      string  `json:"result_format"`
	Seed              int     `json:"seed,omitempty"`
	MaxTokens         int     `json:"max_tokens,omitempty"`
	TopP              float64 `json:"top_p,omitempty"`
	TopK              int     `json:"top_k,omitempty"`
	RepetitionPenalty float64 `json:"repetition_penalty,omitempty"`
	Temperature       float64 `json:"temperature,omitempty"`
	Stop              []string `json:"stop,omitempty"`
	EnableSearch      bool    `json:"enable_search,omitempty"`
	IncrementalOutput bool    `json:"incremental_output,omitempty"`
}

// 通义千问响应结构
type qwenResponse struct {
	Output qwenOutput `json:"output"`
	Usage  qwenUsage  `json:"usage"`
	RequestID string  `json:"request_id"`
}

type qwenOutput struct {
	Text         string `json:"text"`
	FinishReason string `json:"finish_reason"`
}

type qwenUsage struct {
	OutputTokens int `json:"output_tokens"`
	InputTokens  int `json:"input_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func (p *QwenLLMProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// 转换消息格式
	qwenMessages := make([]qwenMessage, len(req.Messages))
	for i, msg := range req.Messages {
		qwenMessages[i] = qwenMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// 构建请求
	qwenReq := qwenRequest{
		Model: p.model,
		Input: qwenInput{
			Messages: qwenMessages,
		},
		Parameters: qwenParams{
			ResultFormat:      "text",
			MaxTokens:         req.MaxTokens,
			Temperature:       req.Temperature,
			TopP:              req.TopP,
			IncrementalOutput: false,
		},
	}

	// 发送请求
	respData, err := p.sendRequest(ctx, qwenReq)
	if err != nil {
		return nil, err
	}

	var qwenResp qwenResponse
	if err := json.Unmarshal(respData, &qwenResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &ChatResponse{
		Text:         qwenResp.Output.Text,
		FinishReason: qwenResp.Output.FinishReason,
		Usage: &Usage{
			PromptTokens:     qwenResp.Usage.InputTokens,
			CompletionTokens: qwenResp.Usage.OutputTokens,
			TotalTokens:      qwenResp.Usage.TotalTokens,
		},
	}, nil
}

func (p *QwenLLMProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan *ChatDelta, error) {
	// 转换消息格式
	qwenMessages := make([]qwenMessage, len(req.Messages))
	for i, msg := range req.Messages {
		qwenMessages[i] = qwenMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// 构建流式请求
	qwenReq := qwenRequest{
		Model: p.model,
		Input: qwenInput{
			Messages: qwenMessages,
		},
		Parameters: qwenParams{
			ResultFormat:      "text",
			MaxTokens:         req.MaxTokens,
			Temperature:       req.Temperature,
			TopP:              req.TopP,
			IncrementalOutput: true, // 启用流式输出
		},
	}

	return p.sendStreamRequest(ctx, qwenReq)
}

func (p *QwenLLMProvider) sendRequest(ctx context.Context, req qwenRequest) ([]byte, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

func (p *QwenLLMProvider) sendStreamRequest(ctx context.Context, req qwenRequest) (<-chan *ChatDelta, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("X-DashScope-SSE", "enable")

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
	deltaChan := make(chan *ChatDelta, 100)

	go func() {
		defer resp.Body.Close()
		defer close(deltaChan)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			
			// 解析 SSE 事件
			if strings.HasPrefix(line, "data:") {
				data := strings.TrimPrefix(line, "data:")
				data = strings.TrimSpace(data)
				
				if data == "[DONE]" {
					deltaChan <- &ChatDelta{
						Text:         "",
						FinishReason: "stop",
					}
					return
				}

				var qwenResp qwenResponse
				if err := json.Unmarshal([]byte(data), &qwenResp); err != nil {
					// 忽略解析错误，继续处理下一行
					continue
				}

				delta := &ChatDelta{
					Text:         qwenResp.Output.Text,
					FinishReason: qwenResp.Output.FinishReason,
				}

				if qwenResp.Usage.TotalTokens > 0 {
					delta.Usage = &Usage{
						PromptTokens:     qwenResp.Usage.InputTokens,
						CompletionTokens: qwenResp.Usage.OutputTokens,
						TotalTokens:      qwenResp.Usage.TotalTokens,
					}
				}

				select {
				case deltaChan <- delta:
				case <-ctx.Done():
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			// 可以考虑通过 error channel 返回错误
			fmt.Printf("Error reading stream: %v\n", err)
		}
	}()

	return deltaChan, nil
}