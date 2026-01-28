package exception

import "errors"

var (
	ErrOrderUnsupportedAction    = errors.New("order: unsupported action")
	ErrOrderNilUsecase           = errors.New("order: nil usecase")
	ErrOrderInvalidRequest       = errors.New("order: invalid request")
	ErrOrderMismatchPlatform     = errors.New("order: mismatch platform")
	ErrOrderUnsupportedPlatform  = errors.New("order: unsupported platform")
	ErrOrderUnsupportedType      = errors.New("order: unsupported type")
	ErrOrderNilEncoder           = errors.New("order: nil encoder")
	ErrOrderNilWorkerPool        = errors.New("order: nil worker pool")
	ErrOrderInvalidWorkerConfig  = errors.New("order: invalid worker config")
	ErrOrderNilDialer            = errors.New("order: nil dialer")
	ErrOrderQueueFull            = errors.New("order: queue full")
	ErrOrderDecodeResponseBody   = errors.New("order: decode response body")
	ErrOrderEmptyResponseOrderID = errors.New("order: empty response order id")
)

var (
	ErrOrderRequestNotSent   = errors.New("order: request did not send")
	ErrOrderResponseBTCCCode = errors.New("order: response code is not zero")
	ErrOrderResponseBTCCMsg  = errors.New("order: response error message is not empty")
)
