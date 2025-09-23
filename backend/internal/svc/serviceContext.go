package svc

import (
	"os"
	
	"github.com/unclewu3242592726/CosTalk/backend/internal/config"
	"github.com/unclewu3242592726/CosTalk/backend/pkg/provider"
)

type ServiceContext struct {
	Config   config.Config
	Registry *provider.Registry
}

func NewServiceContext(c config.Config) *ServiceContext {
	// 创建 Provider Registry
	registry := provider.NewRegistry()
	
	// 注册 Qwen LLM Provider
	qwenAPIKey := c.Providers.Qwen.APIKey
	if qwenAPIKey == "" {
		qwenAPIKey = os.Getenv("QWEN_API_KEY")
	}
	if qwenAPIKey != "" {
		qwenProvider := provider.NewQwenLLMProvider(qwenAPIKey)
		registry.RegisterLLM("qwen", qwenProvider)
	}
	
	// 注册科大讯飞 ASR/TTS Provider
	iflytekAppID := c.Providers.Iflytek.AppID
	iflytekAPISecret := c.Providers.Iflytek.APISecret
	iflytekAPIKey := c.Providers.Iflytek.APIKey
	
	if iflytekAppID == "" {
		iflytekAppID = os.Getenv("IFLYTEK_APP_ID")
	}
	if iflytekAPISecret == "" {
		iflytekAPISecret = os.Getenv("IFLYTEK_API_SECRET")
	}
	if iflytekAPIKey == "" {
		iflytekAPIKey = os.Getenv("IFLYTEK_API_KEY")
	}
	
	if iflytekAppID != "" && iflytekAPISecret != "" && iflytekAPIKey != "" {
		asrProvider := provider.NewIflytekASRProvider(iflytekAppID, iflytekAPISecret, iflytekAPIKey)
		ttsProvider := provider.NewIflytekTTSProvider(iflytekAppID, iflytekAPISecret, iflytekAPIKey)
		
		registry.RegisterASR("iflytek", asrProvider)
		registry.RegisterTTS("iflytek", ttsProvider)
	}
	
	// 注册七牛云 LLM/ASR/TTS Provider
	qiniuAPIKey := c.Providers.Qiniu.APIKey
	
	if qiniuAPIKey == "" {
		qiniuAPIKey = os.Getenv("QINIU_API_KEY")
	}
	
	if qiniuAPIKey != "" {
		// 注册七牛云 LLM Provider
		qiniuLLMProvider := provider.NewQiniuLLMProvider(qiniuAPIKey)
		registry.RegisterLLM("qiniu", qiniuLLMProvider)
		
		// 注册七牛云 ASR Provider
		qiniuASRProvider := provider.NewQiniuASRProvider(qiniuAPIKey)
		registry.RegisterASR("qiniu", qiniuASRProvider)
		
		// 注册七牛云 TTS Provider
		qiniuTTSProvider := provider.NewQiniuTTSProvider(qiniuAPIKey)
		registry.RegisterTTS("qiniu", qiniuTTSProvider)
	}
	
	return &ServiceContext{
		Config:   c,
		Registry: registry,
	}
}
