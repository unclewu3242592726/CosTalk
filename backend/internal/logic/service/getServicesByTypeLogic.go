package service

import (
	"context"

	"github.com/unclewu3242592726/CosTalk/backend/internal/svc"
	"github.com/unclewu3242592726/CosTalk/backend/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetServicesByTypeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetServicesByTypeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetServicesByTypeLogic {
	return &GetServicesByTypeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetServicesByTypeLogic) GetServicesByType(serviceType string) (resp *types.ServiceListResponse, err error) {
	// 根据类型获取 Provider 信息
	providers := l.svcCtx.Registry.GetProvidersByType(serviceType)
	
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
