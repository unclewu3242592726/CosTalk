package model

import "time"

// Role represents a character with personality and skills
type Role struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Avatar       string            `json:"avatar"`
	Description  string            `json:"description"`
	SystemPrompt string            `json:"systemPrompt"`
	Guardrails   []string          `json:"guardrails"`
	TTSDefault   map[string]string `json:"ttsDefault"` // voice, style settings
	Skills       []string          `json:"skills"`    // knowledge_qa, storytelling, emotion_expression
}

// Conversation represents a chat session
type Conversation struct {
	ID          string     `json:"id"`
	RoleID      string     `json:"roleId"`
	Messages    []*Message `json:"messages"`
	LastSummary string     `json:"lastSummary,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// Message represents a single chat message
type Message struct {
	Role      string    `json:"role"`      // system|user|assistant
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Memory management for conversations
type Memory struct {
	ConversationID string   `json:"conversationId"`
	Summary        string   `json:"summary"`
	KeyPoints      []string `json:"keyPoints"`
	WindowSize     int      `json:"windowSize"` // recent messages to keep
}

// WebSocket frame types for real-time communication
type WSFrame struct {
	Type    string      `json:"type"`
	SeqNum  int         `json:"seq,omitempty"`
	Content interface{} `json:"content,omitempty"`
}

type TextDeltaFrame struct {
	Text string `json:"text"`
}

type AudioChunkFrame struct {
	Format string `json:"format"` // mp3|pcm
	Data   string `json:"data"`   // base64 encoded
}

type MetaFrame struct {
	Usage    *Usage    `json:"usage,omitempty"`
	Warnings []string  `json:"warnings,omitempty"`
}

type ErrorFrame struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Usage struct {
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	TotalTokens      int     `json:"totalTokens"`
	Cost             float64 `json:"cost,omitempty"`
}

// Safety and moderation structures
type SafetyResult struct {
	Action  string   `json:"action"`  // block|rewrite|warn|pass
	Score   float64  `json:"score"`
	Labels  []string `json:"labels"`
	Reason  string   `json:"reason"`
}

// Constants for frame types
const (
	FrameTypeTextDelta   = "text_delta"
	FrameTypeAudioChunk  = "audio_chunk"
	FrameTypeMeta        = "meta"
	FrameTypeEnd         = "end"
	FrameTypeError       = "error"
)

// Constants for roles
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Constants for safety actions
const (
	SafetyActionBlock    = "block"
	SafetyActionRewrite  = "rewrite"
	SafetyActionWarn     = "warn"
	SafetyActionPass     = "pass"
)