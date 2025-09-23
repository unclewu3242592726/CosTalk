package service

import (
	"context"

	"github.com/unclewu3242592726/CosTalk/backend/internal/svc"
	"github.com/unclewu3242592726/CosTalk/backend/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetServiceStatusLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetServiceStatusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetServiceStatusLogic {
	return &GetServiceStatusLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetServiceStatusLogic) GetServiceStatus(serviceType, name string) (resp *types.ServiceStatusResponse, err error) {
	// 获取特定 Provider 的信息
	providerInfo, err := l.svcCtx.Registry.GetProviderInfo(serviceType, name)
	if err != nil {
		return &types.ServiceStatusResponse{
			Code:    404,
			Message: err.Error(),
		}, nil
	}
	
	return &types.ServiceStatusResponse{
		Code:    0,
		Message: "success",
		Data: types.ProviderInfo{
			Name:         providerInfo.Name,
			Type:         providerInfo.Type,
			Status:       providerInfo.Status,
			Capabilities: providerInfo.Capabilities,
			Config:       providerInfo.Config,
		},
	}, nil
}
