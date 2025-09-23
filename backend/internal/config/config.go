package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf
	
	// Provider 配置
	Providers ProviderConfig `json:"providers,omitempty"`
}

type ProviderConfig struct {
	// LLM Provider 配置
	Qwen QwenConfig `json:"qwen,omitempty"`
	
	// ASR/TTS Provider 配置
	Iflytek IflytekConfig `json:"iflytek,omitempty"`
	Qiniu   QiniuConfig   `json:"qiniu,omitempty"`
}

type QwenConfig struct {
	APIKey  string `json:"apiKey,omitempty"`
	BaseURL string `json:"baseUrl,omitempty"`
}

type IflytekConfig struct {
	AppID     string `json:"appId,omitempty"`
	APISecret string `json:"apiSecret,omitempty"`
	APIKey    string `json:"apiKey,omitempty"`
}

type QiniuConfig struct {
	AccessKey string `json:"accessKey,omitempty"` // 七牛云存储访问密钥
	SecretKey string `json:"secretKey,omitempty"` // 七牛云存储私钥
	APIKey    string `json:"apiKey,omitempty"`    // 七牛云 AI Token API 密钥
}
