package btcc

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"main/internal/adapter"
	"main/internal/adapter/enum"
	"main/pkg/exception"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
)

const (
	_btccBaseUrl    = "https://spotapi2.btcccdn.com/"
	_btccBaseUrlDev = "https://spot.cryptouat.com:9910/"

	_btccBaseWsUrl    = "wss://spotprice2.btcccdn.com/ws"
	_btccBaseWsUrlDev = "wss://spot.cryptouat.com:8700/ws"

	_btccBaseWsUrlV2    = "ws://stream.cryptouat.com/ws/spot/v1/public"
	_btccBaseWsUrlDevV2 = "ws://stream.cryptouat.com/ws/spot/v1/private"
)

type Delegator struct {
	client *http.Client
}

func NewDelegator(client *http.Client) *Delegator {
	return &Delegator{
		client: client,
	}
}

func (d Delegator) Send(ctx context.Context, req adapter.OrderRequest) (adapter.OrderResponse, error) {
	switch req.Intent.Action {
	case enum.OrderActionPlaceOrder:
		return d.placeOrder(ctx, req)
	case enum.OrderActionCancelOrder:
		return d.cancelOrder(ctx, req)
	default:
		return adapter.OrderResponse{}, exception.ErrOrderUnsupportedAction
	}
}

func btccSide(side enum.OrderSide) string {
	switch side {
	case enum.OrderSideSell:
		return "2"
	case enum.OrderSideBuy:
		return "1"
	default:
		return btccSide(enum.OrderSideBuy)
	}
}

func btccTimeInForce(tif enum.OrderTimeInForce) string {
	switch tif {
	case enum.OrderTimeInForceGTC:
		return "0"
	case enum.OrderTimeInForceIOC:
		return "8"
	case enum.OrderTimeInForceFOK:
		return "16"
	default:
		return btccTimeInForce(enum.OrderTimeInForceGTC)
	}
}

func (d Delegator) placeOrder(ctx context.Context, req adapter.OrderRequest) (adapter.OrderResponse, error) {
	var response adapter.OrderResponse
	if req.Intent.Platform != enum.PlatformBTCC {
		return response, exception.ErrOrderMismatchPlatform
	}

	buf := make([]byte, 0, 64)

	// TODO: Support place market order
	if req.Intent.Type != enum.OrderTypeLimit {
		return response, exception.ErrOrderUnsupportedType
	}

	body := map[string]string{
		"access_id": string(req.Token.Key.AppendBytes(buf)),
		"tm":        strconv.FormatInt(time.Now().Unix(), 10),
		"market":    string(req.Intent.SymbolID.AppendBytes(buf)),
		"side":      btccSide(req.Intent.Side),
		"price":     string(req.Intent.Price.AppendBytes(buf)),
		"amount":    string(req.Intent.Quantity.AppendBytes(buf)),
		"source":    string(req.Intent.Source.AppendBytes(buf)),
		"option":    btccTimeInForce(req.Intent.TimeInForce),
		"client_id": string(req.Intent.ClientOrderID.AppendBytes(buf)),
	}

	payload, err := sonic.ConfigFastest.Marshal(body)
	if err != nil {
		return response, err
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		_btccBaseUrl+"/btcc_api_trade/order/limit",
		bytes.NewReader(payload),
	)
	if err != nil {
		return response, err
	}
	r.Header.Set("Content-Type", "application/json")

	var signed string
	{
		pairs := make([]string, 0, len(body))
		for k, v := range body {
			pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
		}
		pairs = append(pairs, fmt.Sprintf("secret_key=%s", req.Token.Secret.String()))
		sort.Strings(pairs)
		paramStr := strings.Join(pairs, "&")
		hash := md5.Sum([]byte(paramStr))
		signed = hex.EncodeToString(hash[:])
	}
	r.Header.Set("authorization", signed)

	resp, err := d.client.Do(r)
	if err != nil {
		return response, err
	}

	var data Response[ResponsePlaceLimitOrder]
	if err := sonic.ConfigFastest.NewDecoder(resp.Body).Decode(&data); err != nil {
		return response, err
	}

	// BUG: Fix me
	panic("Implement me")
}

func (d Delegator) cancelOrder(ctx context.Context, req adapter.OrderRequest) (adapter.OrderResponse, error) {
	// BUG: Fix me
	panic("Implement me")
}
