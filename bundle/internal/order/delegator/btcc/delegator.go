package btcc

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"main/libs/adapter"
	"main/libs/adapter/enum"
	"main/libs/shared/exception"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
)

const (
	_btccBaseUrl    = "https://spotapi2.btcccdn.com"
	_btccBaseUrlDev = "https://spot.cryptouat.com:9910"

	_btccBaseWsUrl    = "wss://spotprice2.btcccdn.com/ws"
	_btccBaseWsUrlDev = "wss://spot.cryptouat.com:8700/ws"

	_btccBaseWsUrlV2    = "ws://stream.cryptouat.com/ws/spot/v1/public"
	_btccBaseWsUrlDevV2 = "ws://stream.cryptouat.com/ws/spot/v1/private"
)

const (
	_pathPlaceLimitOrder  = "/btcc_api_trade/order/limit"
	_pathPlaceMarketOrder = "/btcc_api_trade/order/market"
	_pathCancelOrder      = "/btcc_api_trade/order/cancel"
)

type Delegator struct {
	restBaseUrl string
	wsBaseUrl   string
	client      *http.Client
}

func NewDelegator(client *http.Client, isTestnet ...bool) *Delegator {
	restBaseUrl := _btccBaseUrl
	if len(isTestnet) != 0 && isTestnet[0] {
		restBaseUrl = _btccBaseUrlDev
	}

	return &Delegator{
		restBaseUrl: restBaseUrl,
		client:      client,
	}
}

func (d Delegator) Send(ctx context.Context, req adapter.OrderRequest) (adapter.OrderResponse, error) {
	switch req.Intent.Action {
	case enum.OrderActionPlaceOrder:
		res, err := d.placeOrder(ctx, req)
		if err != nil {
			return res, fmt.Errorf("place order, err: %w", err)
		}

		return res, nil
	case enum.OrderActionCancelOrder:
		res, err := d.cancelOrder(ctx, req)
		if err != nil {
			return res, fmt.Errorf("cancel order, err: %w", err)
		}

		return res, nil
	default:
		return adapter.OrderResponse{}, fmt.Errorf("action %d, err: %w", req.Intent.Action, exception.ErrOrderUnsupportedAction)
	}
}

func btccSide(side enum.OrderSide) int {
	switch side {
	case enum.OrderSideSell:
		return 2
	case enum.OrderSideBuy:
		return 1
	default:
		return btccSide(enum.OrderSideBuy)
	}
}

func btccTimeInForce(tif enum.OrderTimeInForce) int {
	switch tif {
	case enum.OrderTimeInForceGTC:
		return 0
	case enum.OrderTimeInForceIOC:
		return 8
	case enum.OrderTimeInForceFOK:
		return 16
	default:
		return btccTimeInForce(enum.OrderTimeInForceGTC)
	}
}

func (d Delegator) placeOrder(ctx context.Context, req adapter.OrderRequest) (adapter.OrderResponse, error) {
	response := adapter.OrderResponse{
		Request: req,
	}
	if req.Intent.Platform != enum.PlatformBTCC {
		return response, fmt.Errorf("request platform should be %d but got %d, err: %w",
			enum.PlatformBTCC, req.Intent.Platform, exception.ErrOrderMismatchPlatform)
	}

	var (
		body map[string]any
		path string
		buf  = make([]byte, 0, 64)
	)
	switch req.Intent.Type {
	case enum.OrderTypeLimit:
		path = _pathPlaceLimitOrder
		body = map[string]any{
			"access_id": string(req.Token.Key.AppendBytes(buf)),
			"tm":        time.Now().Unix(),
			"market":    string(req.Intent.Symbol.AppendBytes(buf)),
			"side":      btccSide(req.Intent.Side),
			"price":     string(req.Intent.Price.AppendBytes(buf)),
			"amount":    string(req.Intent.Quantity.AppendBytes(buf)),
			"source":    string(req.Intent.Source.AppendBytes(buf)),
			"option":    btccTimeInForce(req.Intent.TimeInForce),
			"client_id": string(req.Intent.ClientOrderID.AppendBytes(buf)),
		}
	case enum.OrderTypeMarket:
		path = _pathPlaceMarketOrder
		body = map[string]any{
			"access_id": string(req.Token.Key.AppendBytes(buf)),
			"tm":        time.Now().Unix(),
			"market":    string(req.Intent.Symbol.AppendBytes(buf)),
			"side":      btccSide(req.Intent.Side),
			"amount":    string(req.Intent.Quantity.AppendBytes(buf)),
			"source":    string(req.Intent.Source.AppendBytes(buf)),
			"client_id": string(req.Intent.ClientOrderID.AppendBytes(buf)),
		}
	default:
		return response, fmt.Errorf("order type: %d, err: %w", req.Intent.Type, exception.ErrOrderUnsupportedType)
	}

	payload, err := sonic.ConfigFastest.Marshal(body)
	if err != nil {
		return response, fmt.Errorf("marshal body, err: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		d.restBaseUrl+path,
		bytes.NewReader(payload),
	)
	if err != nil {
		return response, fmt.Errorf("create request, err: %w", err)
	}

	var signed string
	{
		pairs := make([]string, 0, len(body))
		for k, v := range body {
			pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
		}
		pairs = append(pairs, fmt.Sprintf("secret_key=%s", req.Token.Secret.String()))
		sort.Strings(pairs)
		paramStr := strings.Join(pairs, "&")
		hash := md5.Sum([]byte(paramStr))
		signed = hex.EncodeToString(hash[:])
	}
	r.Header.Set("authorization", signed)
	r.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(r)
	if err != nil {
		return response, fmt.Errorf("send request, err: %w", err)
	}
	defer resp.Body.Close()
	defer io.Copy(io.Discard, resp.Body)

	response.Sent = true

	var data Response[ResponsePlaceOrder]
	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&data); err != nil && !errors.Is(err, io.EOF) {
		response.SetInternalError(exception.ErrOrderDecodeResponseBody)
		return response, fmt.Errorf("decode response body, err: %w, body: %s", err, string(buf))
	}

	if data.Error.Code != 0 {
		response.SetError(data.Error.Code, data.Error.Message)
		return response, nil
	}

	if data.Data.ID == 0 {
		response.SetInternalError(exception.ErrOrderEmptyResponseOrderID)
		return response, exception.ErrOrderEmptyResponseOrderID
	}

	response.OrderID = adapter.NewStr64(strconv.FormatInt(data.Data.ID, 10))

	return response, nil
}

func (d Delegator) cancelOrder(ctx context.Context, req adapter.OrderRequest) (adapter.OrderResponse, error) {
	response := adapter.OrderResponse{
		Request: req,
	}
	if req.Intent.Platform != enum.PlatformBTCC {
		return response, fmt.Errorf("request platform should be %d but got %d, err: %w",
			enum.PlatformBTCC, req.Intent.Platform, exception.ErrOrderMismatchPlatform)
	}

	var (
		buf   = make([]byte, 0, 64)
		id, _ = strconv.ParseInt(string(req.Intent.ID.AppendBytes(buf)), 10, 64)
		body  = map[string]any{
			"access_id": string(req.Token.Key.AppendBytes(buf)),
			"tm":        time.Now().Unix(),
			"market":    string(req.Intent.Symbol.AppendBytes(buf)),
			"id":        id,
		}
	)

	if id == 0 {
		return response, fmt.Errorf("request id should be available but got %s", req.Intent.ID.String())
	}

	payload, err := sonic.ConfigFastest.Marshal(body)
	if err != nil {
		return response, fmt.Errorf("marshal body, err: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		d.restBaseUrl+_pathCancelOrder,
		bytes.NewReader(payload),
	)
	if err != nil {
		return response, fmt.Errorf("create request, err: %w", err)
	}

	var signed string
	{
		pairs := make([]string, 0, len(body))
		for k, v := range body {
			pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
		}
		pairs = append(pairs, fmt.Sprintf("secret_key=%s", req.Token.Secret.String()))
		sort.Strings(pairs)
		paramStr := strings.Join(pairs, "&")
		hash := md5.Sum([]byte(paramStr))
		signed = hex.EncodeToString(hash[:])
	}
	r.Header.Set("authorization", signed)
	r.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(r)
	if err != nil {
		return response, fmt.Errorf("send request, err: %w", err)
	}
	defer resp.Body.Close()
	defer io.Copy(io.Discard, resp.Body)

	response.Sent = true

	var data Response[ResponseCancelOrder]
	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&data); err != nil && !errors.Is(err, io.EOF) {
		response.SetInternalError(exception.ErrOrderDecodeResponseBody)
		return response, fmt.Errorf("decode response body, err: %w", err)
	}

	if data.Error.Code != 0 {
		response.SetError(data.Error.Code, data.Error.Message)
		return response, nil
	}

	if data.Data.ID == 0 {
		response.SetInternalError(exception.ErrOrderEmptyResponseOrderID)
		return response, exception.ErrOrderEmptyResponseOrderID
	}

	response.OrderID = adapter.NewStr64(strconv.FormatInt(data.Data.ID, 10))

	return response, nil
}
