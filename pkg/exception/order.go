package exception

import "errors"

var (
	ErrOrderNilUsecase            = errors.New("order: nil usecase")
	ErrOrderInvalidRequest        = errors.New("order: invalid request")
	ErrOrderUnsupportedPlatform   = errors.New("order: unsupported platform")
	ErrOrderNilEncoder            = errors.New("order: nil encoder")
	ErrOrderNilWorkerPool         = errors.New("order: nil worker pool")
	ErrOrderInvalidWorkerConfig   = errors.New("order: invalid worker config")
	ErrOrderNilDialer             = errors.New("order: nil dialer")
	ErrOrderQueueFull             = errors.New("order: queue full")
)
