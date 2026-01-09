package marketdata_old

import (
	"context"
	"main/internal/errors"
	"strconv"

	"github.com/yanun0323/decimal"
	"github.com/yanun0323/logs"
	"github.com/yanun0323/pkg/sys"
	"github.com/yanun0323/pkg/ws"
)

type BtccPub struct {
	wss *ws.WebSocket
}

func NewBtccPub(ctx context.Context, devMode bool) *BtccPub {
	wsURL := _btccBaseWsUrl
	if devMode {
		wsURL = _btccBaseWsUrlDev
	}

	return &BtccPub{
		wss: ws.New(ctx, wsURL),
	}
}

func (repo *BtccPub) Len() int {
	return repo.wss.Len()
}

func (repo *BtccPub) Close() {
	repo.wss.Close()
}

func (repo *BtccPub) CloseWhenEmpty() bool {
	if repo.Len() == 0 {
		repo.Close()
		return true
	}

	return false
}

func (repo *BtccPub) StartWebsocket(ctx context.Context) error {
	if err := repo.wss.Start(ctx); err != nil {
		return errors.Wrap(err, "start wss")
	}

	return nil
}

type BtccDepth struct {
	Market    string
	Full      bool
	Orderbook struct {
		Asks     [][]decimal.Decimal `json:"asks"`
		Bids     [][]decimal.Decimal `json:"bids"`
		Last     decimal.Decimal     `json:"last"`
		Time     int64               `json:"time"`
		Checksum int64               `json:"checksum"`
	}
}

func (repo *BtccPub) ObserveDepth(ctx context.Context, handler func(d BtccDepth)) (unsubscribe func()) {
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

				resp, ok := ws.ReadMessage[BtccResponse](m)
				if !ok || resp.Method != "depth.update" {
					continue
				}

				var depth BtccDepth
				if err := resp.Unmarshal(2, &depth.Market); err != nil {
					logs.Errorf("unmarshal depth market, err: %+v", err)
					continue
				}

				if err := resp.Unmarshal(0, &depth.Full); err != nil {
					logs.Errorf("unmarshal depth full, err: %+v", err)
					continue
				}

				if err := resp.Unmarshal(1, &depth.Orderbook); err != nil {
					logs.Errorf("unmarshal depth orderbook, err: %+v", err)
					continue
				}

				handler(depth)
			}
		}
	}()

	return cancel
}

func (repo *BtccPub) SubscribeDepth(ctx context.Context, market string, depth int, precision float64) error {
	if err := repo.wss.SendAndWait(ctx, ws.Sidecar{
		Sender: func(ctx context.Context, client *ws.WebSocket) error {
			if err := client.WriteJSON(map[string]any{
				"id":     btccWsMethodDepthID,
				"method": "depth.subscribe",
				"params": []any{
					market, depth, strconv.FormatFloat(precision, 'f', 8, 64),
				},
			}); err != nil {
				return errors.Wrap(err, "write subscribe depth payload")
			}

			return nil
		},
		Waiter: func(ctx context.Context, m ws.Message) (bool, error) {
			logs.Infof("%s", m)
			resp, ok := ws.ReadMessage[BtccSubscribeResponse](m)
			if !ok || resp.ID != btccWsMethodDepthID {
				return false, nil
			}

			if resp.Error != nil || resp.Result.Status != "success" {
				return false, nil
			}

			return true, nil
		},
	}); err != nil {
		return errors.Wrap(err, "send and wait")
	}

	return nil
}
