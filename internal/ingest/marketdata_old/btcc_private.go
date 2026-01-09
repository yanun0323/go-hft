package marketdata_old

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"main/internal/errors"

	"github.com/yanun0323/pkg/sys"
	"github.com/yanun0323/pkg/ws"
)

type BtccPrivate struct {
	wss       *ws.WebSocket
	accessID  string
	secretKey string
}

func NewBtccPrivate(ctx context.Context, devMode bool, accessID, secretKey string) *BtccPrivate {
	wsURL := _btccBaseWsUrl
	if devMode {
		wsURL = _btccBaseWsUrlDev
	}

	return &BtccPrivate{
		wss:       ws.New(ctx, wsURL),
		accessID:  accessID,
		secretKey: secretKey,
	}
}

func (repo *BtccPrivate) StartWebsocketAndAuth(ctx context.Context) error {
	if err := repo.wss.Start(ctx, ws.Sidecar{
		Sender: func(ctx context.Context, client *ws.WebSocket) error {
			sum := sha256.Sum256([]byte(repo.secretKey))
			signData := hex.EncodeToString(sum[:])
			authPayload := map[string]any{
				"id":     btccWsMethodAuthID,
				"method": "server.accessid_auth",
				"params": []any{
					repo.accessID, signData,
				},
			}

			if err := client.WriteJSON(authPayload); err != nil {
				return errors.Wrap(err, "write auth payload")
			}

			return nil
		},
		Waiter: func(ctx context.Context, m ws.Message) (bool, error) {
			resp, ok := ws.ReadMessage[BtccSubscribeResponse](m)
			if !ok || resp.ID != btccWsMethodAuthID {
				return false, nil
			}

			if resp.Error != nil || resp.Result.Status != "success" {
				return false, errors.New("invalid authenticate")
			}

			return true, nil
		},
	}); err != nil {
		return errors.Wrap(err, "start wss")
	}

	return nil
}

type BtccOrderUpdate struct {
	OrderStatus int
	Order       struct {
		ID             int     `json:"id"`
		Type           int     `json:"type"`
		Side           int     `json:"side"`
		UserID         int     `json:"user_id"`
		AccountID      int     `json:"account"`
		Option         int     `json:"option"`
		CTime          float64 `json:"ctime"`
		MTime          float64 `json:"mtime"`
		Market         string  `json:"market"`
		Source         string  `json:"source"`
		ClientID       string  `json:"client_id"`
		Price          string  `json:"price"`
		Amount         string  `json:"amount"`
		TakerFee       string  `json:"taker_fee"`
		MakerFee       string  `json:"maker_fee"`
		Left           string  `json:"left"`
		DealStock      string  `json:"deal_stock"`
		DealMoney      string  `json:"deal_money"`
		DealFee        string  `json:"deal_fee"`
		AssetFee       string  `json:"asset_fee"`
		FeeDiscount    string  `json:"fee_discount"`
		LastDealAmount string  `json:"last_deal_amount"`
		LastDealPrice  string  `json:"last_deal_price"`
		LastDealTime   float64 `json:"last_deal_time"`
		LastDealID     int     `json:"last_deal_i_d"`
		LastRole       int     `json:"last_role"`
		FeeAsset       string  `json:"fee_asset"`
	}
}

func (repo *BtccPrivate) ObserveOrder(ctx context.Context, handler func(o BtccOrderUpdate)) (unsubscribe func()) {
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
				if !ok || resp.Method != "order.update" {
					continue
				}

				var order BtccOrderUpdate
				if err := resp.Unmarshal(0, &order.OrderStatus); err != nil {
					continue
				}

				if err := resp.Unmarshal(1, &order.Order); err != nil {
					continue
				}

				handler(order)
			}
		}
	}()

	return cancel
}

func (repo *BtccPrivate) SubscribeOrder(ctx context.Context, market string) error {
	appendIntoRegister := true
	if err := repo.wss.SendAndWait(ctx, ws.Sidecar{
		Sender: func(ctx context.Context, client *ws.WebSocket) error {
			if err := client.WriteJSON(map[string]any{
				"id":     btccWsMethodOrderID,
				"method": "order.subscribe",
				"params": []any{
					market,
				},
			}); err != nil {
				return errors.Wrap(err, "write subscribe depth payload")
			}

			return nil
		},
		Waiter: func(ctx context.Context, m ws.Message) (bool, error) {
			resp, ok := ws.ReadMessage[BtccSubscribeResponse](m)
			if !ok || resp.ID != btccWsMethodOrderID {
				return false, nil
			}

			if resp.Error != nil || resp.Result.Status != "success" {
				return false, errors.New("invalid authenticate")
			}

			return true, nil
		},
	}, appendIntoRegister); err != nil {
		return errors.Wrap(err, "send and wait")
	}

	return nil
}
