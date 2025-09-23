package role

import (
	"context"

	"github.com/unclewu3242592726/CosTalk/backend/internal/svc"
	"github.com/unclewu3242592726/CosTalk/backend/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetRolesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetRolesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRolesLogic {
	return &GetRolesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetRolesLogic) GetRoles() (resp *types.RolesResponse, err error) {
	// todo: add your logic here and delete this line

	return
}
