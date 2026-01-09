package exception

import "errors"

// WS errors
var (
	ErrWebSocketConnectionClose = errors.New("websocket: connection closed")
	ErrWebSocketProtocol        = errors.New("websocket: protocol error")
)
