package service

import (
	"net/http"

	"github.com/unclewu3242592726/CosTalk/backend/internal/logic/service"
	"github.com/unclewu3242592726/CosTalk/backend/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/rest/pathvar"
)

func GetServiceStatusHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := service.NewGetServiceStatusLogic(r.Context(), svcCtx)
		
		// 从路径中提取参数
		vars := pathvar.Vars(r)
		serviceType := vars["type"]
		name := vars["name"]
		
		resp, err := l.GetServiceStatus(serviceType, name)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
