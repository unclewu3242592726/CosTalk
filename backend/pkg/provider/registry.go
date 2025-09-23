package provider

import (
	"context"
	"fmt"
)

// Registry manages all providers with unified interfaces
type Registry struct {
	llmProviders        map[string]LLMProvider
	asrProviders        map[string]ASRProvider
	ttsProviders        map[string]TTSProvider
	moderationProviders map[string]ModerationProvider
}

func NewRegistry() *Registry {
	return &Registry{
		llmProviders:        make(map[string]LLMProvider),
		asrProviders:        make(map[string]ASRProvider),
		ttsProviders:        make(map[string]TTSProvider),
		moderationProviders: make(map[string]ModerationProvider),
	}
}

// LLM Provider Interface
type LLMProvider interface {
	Name() string
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, req *ChatRequest) (<-chan *ChatDelta, error)
}

// ASR Provider Interface
type ASRProvider interface {
	Name() string
	StreamRecognize(ctx context.Context, audioStream <-chan []byte) (<-chan *Transcript, error)
}

// TTS Provider Interface
type TTSProvider interface {
	Name() string
	SynthesizeStream(ctx context.Context, textStream <-chan string, opts *TTSOptions) (<-chan *AudioChunk, error)
}

// Moderation Provider Interface
type ModerationProvider interface {
	Name() string
	CheckText(ctx context.Context, text string) (*ModerationResult, error)
}

// Data structures
type ChatRequest struct {
	Model       string     `json:"model"`
	Messages    []*Message `json:"messages"`
	Temperature float64    `json:"temperature,omitempty"`
	TopP        float64    `json:"top_p,omitempty"`
	MaxTokens   int        `json:"max_tokens,omitempty"`
	Stream      bool       `json:"stream"`
}

type Message struct {
	Role    string `json:"role"`    // system|user|assistant
	Content string `json:"content"`
}

type ChatResponse struct {
	Text         string `json:"text"`
	FinishReason string `json:"finish_reason"`
	Usage        *Usage `json:"usage"`
}

type ChatDelta struct {
	Text         string `json:"text"`
	FinishReason string `json:"finish_reason,omitempty"`
	Usage        *Usage `json:"usage,omitempty"`
}

type Usage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	Cost             float64 `json:"cost,omitempty"`
}

type Transcript struct {
	Text      string  `json:"text"`
	IsFinal   bool    `json:"is_final"`
	Confidence float64 `json:"confidence"`
}

type AudioChunk struct {
	Data   []byte `json:"data"`
	Format string `json:"format"` // mp3|pcm
	SeqNum int    `json:"seq_num"`
}

type TTSOptions struct {
	Voice string `json:"voice"`
	Style string `json:"style,omitempty"`
	Speed float64 `json:"speed,omitempty"`
}

type ModerationResult struct {
	Level   string   `json:"level"`   // block|rewrite|warn|pass
	Score   float64  `json:"score"`   // 0.0-1.0
	Labels  []string `json:"labels"`  // detected categories
	Reason  string   `json:"reason"`  // explanation
}

// Registry methods
func (r *Registry) RegisterLLM(name string, provider LLMProvider) {
	r.llmProviders[name] = provider
}

func (r *Registry) RegisterASR(name string, provider ASRProvider) {
	r.asrProviders[name] = provider
}

func (r *Registry) RegisterTTS(name string, provider TTSProvider) {
	r.ttsProviders[name] = provider
}

func (r *Registry) RegisterModeration(name string, provider ModerationProvider) {
	r.moderationProviders[name] = provider
}

func (r *Registry) GetLLM(name string) (LLMProvider, error) {
	if provider, ok := r.llmProviders[name]; ok {
		return provider, nil
	}
	return nil, fmt.Errorf("LLM provider '%s' not found", name)
}

func (r *Registry) GetASR(name string) (ASRProvider, error) {
	if provider, ok := r.asrProviders[name]; ok {
		return provider, nil
	}
	return nil, fmt.Errorf("ASR provider '%s' not found", name)
}

func (r *Registry) GetTTS(name string) (TTSProvider, error) {
	if provider, ok := r.ttsProviders[name]; ok {
		return provider, nil
	}
	return nil, fmt.Errorf("TTS provider '%s' not found", name)
}

func (r *Registry) GetModeration(name string) (ModerationProvider, error) {
	if provider, ok := r.moderationProviders[name]; ok {
		return provider, nil
	}
	return nil, fmt.Errorf("Moderation provider '%s' not found", name)
}

// 服务发现相关方法

// ProviderInfo 表示 Provider 信息
type ProviderInfo struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Status       string            `json:"status"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Config       map[string]string `json:"config,omitempty"`
}

// GetAllProviders 获取所有 Provider 信息
func (r *Registry) GetAllProviders() []ProviderInfo {
	var providers []ProviderInfo
	
	// LLM Providers
	for name, _ := range r.llmProviders {
		providers = append(providers, ProviderInfo{
			Name:         name,
			Type:         "llm",
			Status:       "online",
			Capabilities: []string{"chat", "stream"},
		})
	}
	
	// ASR Providers
	for name, _ := range r.asrProviders {
		providers = append(providers, ProviderInfo{
			Name:         name,
			Type:         "asr",
			Status:       "online",
			Capabilities: []string{"stream_recognize"},
		})
	}
	
	// TTS Providers
	for name, _ := range r.ttsProviders {
		providers = append(providers, ProviderInfo{
			Name:         name,
			Type:         "tts",
			Status:       "online",
			Capabilities: []string{"synthesize_stream"},
		})
	}
	
	// Moderation Providers
	for name, _ := range r.moderationProviders {
		providers = append(providers, ProviderInfo{
			Name:         name,
			Type:         "moderation",
			Status:       "online",
			Capabilities: []string{"check_text"},
		})
	}
	
	return providers
}

// GetProvidersByType 根据类型获取 Provider 信息
func (r *Registry) GetProvidersByType(providerType string) []ProviderInfo {
	var providers []ProviderInfo
	
	switch providerType {
	case "llm":
		for name, _ := range r.llmProviders {
			providers = append(providers, ProviderInfo{
				Name:         name,
				Type:         "llm",
				Status:       "online",
				Capabilities: []string{"chat", "stream"},
			})
		}
	case "asr":
		for name, _ := range r.asrProviders {
			providers = append(providers, ProviderInfo{
				Name:         name,
				Type:         "asr",
				Status:       "online",
				Capabilities: []string{"stream_recognize"},
			})
		}
	case "tts":
		for name, _ := range r.ttsProviders {
			providers = append(providers, ProviderInfo{
				Name:         name,
				Type:         "tts",
				Status:       "online",
				Capabilities: []string{"synthesize_stream"},
			})
		}
	case "moderation":
		for name, _ := range r.moderationProviders {
			providers = append(providers, ProviderInfo{
				Name:         name,
				Type:         "moderation",
				Status:       "online",
				Capabilities: []string{"check_text"},
			})
		}
	}
	
	return providers
}

// GetProviderInfo 获取特定 Provider 的信息
func (r *Registry) GetProviderInfo(providerType, name string) (*ProviderInfo, error) {
	switch providerType {
	case "llm":
		if _, ok := r.llmProviders[name]; ok {
			return &ProviderInfo{
				Name:         name,
				Type:         "llm",
				Status:       "online",
				Capabilities: []string{"chat", "stream"},
			}, nil
		}
	case "asr":
		if _, ok := r.asrProviders[name]; ok {
			return &ProviderInfo{
				Name:         name,
				Type:         "asr",
				Status:       "online",
				Capabilities: []string{"stream_recognize"},
			}, nil
		}
	case "tts":
		if _, ok := r.ttsProviders[name]; ok {
			return &ProviderInfo{
				Name:         name,
				Type:         "tts",
				Status:       "online",
				Capabilities: []string{"synthesize_stream"},
			}, nil
		}
	case "moderation":
		if _, ok := r.moderationProviders[name]; ok {
			return &ProviderInfo{
				Name:         name,
				Type:         "moderation",
				Status:       "online",
				Capabilities: []string{"check_text"},
			}, nil
		}
	}
	
	return nil, fmt.Errorf("provider '%s' of type '%s' not found", name, providerType)
}