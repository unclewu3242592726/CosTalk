package service

import (
	"net/http"

	"github.com/unclewu3242592726/CosTalk/backend/internal/logic/service"
	"github.com/unclewu3242592726/CosTalk/backend/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/rest/pathvar"
)

func GetServicesByTypeHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := service.NewGetServicesByTypeLogic(r.Context(), svcCtx)
		
		// 从路径中提取 type 参数
		serviceType := pathvar.Vars(r)["type"]
		
		resp, err := l.GetServicesByType(serviceType)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
