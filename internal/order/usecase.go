package order

import (
	"context"
	"main/internal/adapter"
	"main/internal/adapter/enum"
	"main/pkg/exception"
	"main/pkg/uds"
	"sync/atomic"

	"github.com/rs/zerolog/log"
)

type Usecase struct {
	btccDelegator       Delegator
	binanceDelegator    Delegator
	normalizerClientMap map[enum.Platform]*uds.Client

	running atomic.Bool
	worker  int
	queue   chan adapter.OrderRequest
}

func NewUsecase(workerCount, workerCap int, normalizerClientMap map[enum.Platform]*uds.Client, btccDelegator, binanceDelegator Delegator) *Usecase {
	return &Usecase{
		btccDelegator:       btccDelegator,
		binanceDelegator:    binanceDelegator,
		normalizerClientMap: normalizerClientMap,
		worker:              workerCount,
		queue:               make(chan adapter.OrderRequest, workerCap),
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
		log.Warn().Msg("order usecase already running")
		return
	}

	for range use.worker {
		go (&orderRequestWorker{
			normalizerClientMap: use.normalizerClientMap,
		}).run(ctx, use.queue, use.btccDelegator, use.binanceDelegator)
	}
}

type orderRequestWorker struct {
	normalizerClientMap map[enum.Platform]*uds.Client
}

func (w *orderRequestWorker) run(ctx context.Context, ch chan adapter.OrderRequest, btccDelegator, binanceDelegator Delegator) {
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
				log.Error().Err(err).Msg("send request by delegator")
				continue
			}
			_ = res
			// TODO: save result and send it to ingest normalizer
		case <-ctx.Done():
			log.Info().Msg("worker shutdown")
			return
		}
	}
}
