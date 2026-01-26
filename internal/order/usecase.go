package order

import (
	"context"
	"main/internal/adapter"
	"main/internal/adapter/enum"
	"main/pkg/exception"
	"sync/atomic"
)

type Usecase struct {
	btccDelegator    Delegator
	binanceDelegator Delegator

	running atomic.Bool
	worker  int
	queue   chan adapter.OrderRequest
}

func NewUsecase(workerCount, workerCap int, btccDelegator, binanceDelegator Delegator) *Usecase {
	return &Usecase{
		btccDelegator:    btccDelegator,
		binanceDelegator: binanceDelegator,
		worker:           workerCount,
		queue:            make(chan adapter.OrderRequest, workerCap),
	}
}

func (use *Usecase) Handle(req adapter.OrderRequest) error {
	// TODO: Asset management risk control
	select {
	case use.queue <- req:
		return nil
	default:
		return exception.ErrOrderQueueFull
	}
}

type Delegator interface {
	Send(context.Context, adapter.OrderRequest) (adapter.OrderResponse, error)
}

func (use *Usecase) Run(ctx context.Context) {
	if use.running.Swap(true) {
		return
	}

	for range use.worker {
		go workerExecuteOrderRequest(ctx, use.queue, use.btccDelegator, use.binanceDelegator)
	}
}

func workerExecuteOrderRequest(ctx context.Context, ch chan adapter.OrderRequest, btccDelegator, binanceDelegator Delegator) {
	for {
		select {
		case req := <-ch:
			var delegator Delegator
			switch req.Intent.Platform {
			case enum.PlatformBTCC:
				delegator = btccDelegator
			case enum.PlatformBinance:
				delegator = binanceDelegator
				panic("not supported yet")
			}

			res, err := delegator.Send(ctx, req)
			if err != nil {
				// TODO: handle error
				continue
			}
			_ = res
			// TODO: save result and send it to ingest normalizer
		case <-ctx.Done():
			// TODO: add log
			return
		}
	}
}
