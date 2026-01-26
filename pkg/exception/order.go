package exception

import "errors"

var (
	ErrOrderUnsupportedAction   = errors.New("order: unsupported action")
	ErrOrderNilUsecase          = errors.New("order: nil usecase")
	ErrOrderInvalidRequest      = errors.New("order: invalid request")
	ErrOrderMismatchPlatform    = errors.New("order: mismatch platform")
	ErrOrderUnsupportedPlatform = errors.New("order: unsupported platform")
	ErrOrderUnsupportedType     = errors.New("order: unsupported type")
	ErrOrderNilEncoder          = errors.New("order: nil encoder")
	ErrOrderNilWorkerPool       = errors.New("order: nil worker pool")
	ErrOrderInvalidWorkerConfig = errors.New("order: invalid worker config")
	ErrOrderNilDialer           = errors.New("order: nil dialer")
	ErrOrderQueueFull           = errors.New("order: queue full")
)
