package service

import (
	"context"

	"github.com/unclewu3242592726/CosTalk/backend/internal/svc"
	"github.com/unclewu3242592726/CosTalk/backend/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetServicesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetServicesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetServicesLogic {
	return &GetServicesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetServicesLogic) GetServices() (resp *types.ServiceListResponse, err error) {
	// 获取所有可用的 Provider 信息
	providers := l.svcCtx.Registry.GetAllProviders()
	
	// 转换为 API 响应格式
	var providerInfos []types.ProviderInfo
	for _, p := range providers {
		providerInfos = append(providerInfos, types.ProviderInfo{
			Name:         p.Name,
			Type:         p.Type,
			Status:       p.Status,
			Capabilities: p.Capabilities,
			Config:       p.Config,
		})
	}
	
	return &types.ServiceListResponse{
		Code:    0,
		Message: "success",
		Data:    providerInfos,
	}, nil
}
