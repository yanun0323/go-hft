package marketdata

import (
	"context"
	"fmt"
	"strings"

	"github.com/yanun0323/errors"
	"github.com/yanun0323/logs"
	"github.com/yanun0323/pkg/sys"
	"github.com/yanun0323/pkg/ws"
)

const (
	_binanceBaseWsUrl           = "wss://stream.binance.com:9443/ws"
	_binanceBaseWsUr2           = "wss://stream.binance.com:443/ws"
	_binanceBaseWsUrlMarketOnly = "wss://data-stream.binance.vision"
)

type BinancePub struct {
	wss *ws.WebSocket
}

func NewBinancePub(ctx context.Context) *BinancePub {
	return &BinancePub{
		wss: ws.New(ctx, _binanceBaseWsUrl),
	}
}

func (repo *BinancePub) Len() int {
	return repo.wss.Len()
}

func (repo *BinancePub) Close() {
	repo.wss.Close()
}

func (repo *BinancePub) CloseWhenEmpty() bool {
	if repo.Len() == 0 {
		repo.Close()
		logs.Info("close websocket. reason: empty")
		return true
	}

	return false
}

func (repo *BinancePub) StartWebsocket(ctx context.Context) error {
	if err := repo.wss.Start(ctx); err != nil {
		return errors.Wrap(err, "start wss")
	}

	return nil
}

type BinanceSubscribeRequest struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
	ID     int64    `json:"id"`
}

type BinanceSubscribeResponse struct {
	ID     int64 `json:"id"`
	Result any   `json:"result"`
}

func subscriberResponseParser(m ws.Message) (BinanceSubscribeResponse, bool) {
	var resp BinanceSubscribeResponse
	err := m.Unmarshal(&resp)
	return resp, err == nil
}

// SubscribeDepth subscribes 'Diff. Depth Stream'
func (repo *BinancePub) SubscribeDepth(ctx context.Context, symbol string) error {
	appendIntoRegister := true
	if err := repo.wss.SendAndWait(ctx, ws.Sidecar{
		Sender: func(ctx context.Context, ws *ws.WebSocket) error {
			payload := BinanceSubscribeRequest{
				Method: "SUBSCRIBE",
				Params: []string{
					fmt.Sprintf("%s@depth@100ms", strings.ToLower(symbol)),
				},
				ID: 1,
			}

			if err := ws.WriteJSON(payload); err != nil {
				return errors.Wrap(err, "write subscribe payload").With("payload", payload)
			}

			return nil
		},
		Waiter: func(ctx context.Context, m ws.Message) (bool, error) {
			resp, ok := subscriberResponseParser(m)
			if !ok || resp.ID != 1 {
				return false, nil
			}

			if resp.Result != nil {
				return false, errors.Errorf("subscribe and wait, err: %+v", resp.Result)
			}
			return true, nil
		},
	}, appendIntoRegister); err != nil {
		return errors.Wrap(err, "send and wait")
	}

	return nil
}

type BinanceDepth struct {
	EventType     string      `json:"e"`
	EventTime     int64       `json:"E"`
	Symbol        string      `json:"s"`
	FirstUpdateID int64       `json:"U"`
	FinalUpdateID int64       `json:"u"`
	Bids          [][2]string `json:"b"` // [0]price [1]quantity
	Asks          [][2]string `json:"a"` // [0]price [1]quantity
}

func (repo *BinancePub) ObserveDepth(ctx context.Context, handler func(d BinanceDepth)) (unsubscribe func()) {
	ch, cancel := repo.wss.Subscribe()

	go func() {
		defer cancel()
		for {
			select {
			case <-sys.Shutdown():
				return
			case <-ctx.Done():
				return
			case m, ok := <-ch:
				if !ok {
					return
				}

				resp, ok := ws.ReadMessage[BinanceDepth](m)
				if !ok || resp.EventType != "depthUpdate" {
					return
				}

				handler(resp)
			}
		}
	}()

	return cancel
}

// SubscribeDepth subscribes 'Partial Book Depth Stream'
func (repo *BinancePub) SubscribePartialBookDepth(ctx context.Context, symbol string) error {
	appendIntoRegister := true
	if err := repo.wss.SendAndWait(ctx, ws.Sidecar{
		Sender: func(ctx context.Context, ws *ws.WebSocket) error {
			payload := BinanceSubscribeRequest{
				Method: "SUBSCRIBE",
				Params: []string{
					// depth<5>, depth<10>, depth<20>
					fmt.Sprintf("%s@depth20@100ms", strings.ToLower(symbol)),
				},
				ID: 1,
			}

			if err := ws.WriteJSON(payload); err != nil {
				return errors.Wrap(err, "write subscribe payload").With("payload", payload)
			}

			return nil
		},
		Waiter: func(ctx context.Context, m ws.Message) (bool, error) {
			resp, ok := subscriberResponseParser(m)
			if !ok || resp.ID != 1 {
				return false, nil
			}

			if resp.Result != nil {
				return false, errors.Errorf("subscribe and wait, err: %+v", resp.Result)
			}
			return true, nil
		},
	}, appendIntoRegister); err != nil {
		return errors.Wrap(err, "send and wait")
	}

	return nil
}

type BinancePartialBookDepth struct {
	LastUpdateID int64       `json:"lastUpdateId"`
	Bids         [][2]string `json:"bids"` // [0]price [1]quantity
	Asks         [][2]string `json:"asks"` // [0]price [1]quantity
}

func (repo *BinancePub) ObservePartialBookDepth(ctx context.Context, handler func(d BinancePartialBookDepth)) (unsubscribe func()) {
	ch, cancel := repo.wss.Subscribe()

	go func() {
		defer cancel()
		for {
			select {
			case <-sys.Shutdown():
				return
			case <-ctx.Done():
				return
			case m, ok := <-ch:
				if !ok {
					return
				}

				resp, ok := ws.ReadMessage[BinancePartialBookDepth](m)
				if !ok {
					return
				}

				handler(resp)
			}
		}
	}()

	return cancel
}
