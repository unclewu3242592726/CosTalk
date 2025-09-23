package chat

import (
	"net/http"

	"github.com/unclewu3242592726/CosTalk/backend/internal/logic/chat"
	"github.com/unclewu3242592726/CosTalk/backend/internal/svc"
	"github.com/gorilla/websocket"
	"github.com/zeromicro/go-zero/core/logx"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// 允许跨域连接，生产环境中应该进行更严格的检查
		return true
	},
}

func ChatStreamHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 升级 HTTP 连接为 WebSocket
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logx.Errorf("WebSocket upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		// 创建 ChatStream logic 并处理 WebSocket 连接
		l := chat.NewChatStreamLogic(r.Context(), svcCtx)
		l.HandleWebSocket(conn)
	}
}
