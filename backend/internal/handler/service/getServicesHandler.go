package service

import (
	"net/http"

	"github.com/unclewu3242592726/CosTalk/backend/internal/logic/service"
	"github.com/unclewu3242592726/CosTalk/backend/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func GetServicesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := service.NewGetServicesLogic(r.Context(), svcCtx)
		resp, err := l.GetServices()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
