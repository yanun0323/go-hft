package binance

import (
	"bytes"
	"errors"
	"main/libs/adapter"
	"main/libs/adapter/enum"
	"time"
)

var (
	ErrInvalidPayload = errors.New("binance: invalid payload")
	ErrInvalidRequest = errors.New("binance: invalid request")
)

var (
	keyEventTime   = []byte(`"E"`)
	keyTradeTime   = []byte(`"T"`)
	keyOrderID     = []byte(`"i"`)
	keySide        = []byte(`"S"`)
	keyOrderType   = []byte(`"o"`)
	keyOrderStatus = []byte(`"X"`)
	keyOrderPrice  = []byte(`"p"`)
	keyOrderQty    = []byte(`"q"`)
	keyCumQty      = []byte(`"z"`)
	keyTimeInForce = []byte(`"f"`)
	keyBids        = []byte(`"b"`)
	keyAsks        = []byte(`"a"`)
	keyBidsAlt     = []byte(`"bids"`)
	keyAsksAlt     = []byte(`"asks"`)
)

var (
	valueBuy       = []byte("BUY")
	valueSell      = []byte("SELL")
	valueLimit     = []byte("LIMIT")
	valueMarket    = []byte("MARKET")
	valueNew       = []byte("NEW")
	valuePartial   = []byte("PARTIALLY_FILLED")
	valueFilled    = []byte("FILLED")
	valueCanceled  = []byte("CANCELED")
	valueCancelled = []byte("CANCELLED")
	valueExpired   = []byte("EXPIRED")
	valueGTC       = []byte("GTC")
	valueIOC       = []byte("IOC")
	valueFOK       = []byte("FOK")
)

const maxInt64 = int64(^uint64(0) >> 1)

// DecodeMarketDataPayload converts a websocket payload into an encoded market data payload.
func DecodeMarketDataPayload(topic enum.Topic, symbol adapter.Symbol, payload []byte) ([]byte, error) {
	if !topic.IsAvailable() {
		return nil, ErrInvalidRequest
	}
	if len(payload) == 0 {
		return nil, ErrInvalidPayload
	}
	return decodeBinancePayload(topic, symbol, payload)
}

func decodeBinancePayload(topic enum.Topic, symbol adapter.Symbol, payload []byte) ([]byte, error) {
	switch topic {
	case enum.TopicDepth:
		depth, err := decodeBinanceDepth(payload, symbol)
		if err != nil {
			return nil, err
		}
		return depth.Encode(nil), nil
	case enum.TopicOrder:
		order, err := decodeBinanceOrder(payload, symbol)
		if err != nil {
			return nil, err
		}
		return order.Encode(nil), nil
	default:
		return nil, ErrInvalidRequest
	}
}

func decodeBinanceDepth(payload []byte, symbol adapter.Symbol) (adapter.Depth, error) {
	var depth adapter.Depth
	depth.Symbol = symbol
	depth.Platform = enum.PlatformBinance
	depth.RecvTsNano = time.Now().UnixNano()

	if eventMs, ok := scanUintField(payload, keyEventTime); ok {
		depth.EventTsNano = int64(eventMs) * int64(time.Millisecond)
	}

	bids, ok := scanDepthSide(payload, keyBids, depth.Bids[:])
	if !ok {
		bids, _ = scanDepthSide(payload, keyBidsAlt, depth.Bids[:])
	}
	asks, ok := scanDepthSide(payload, keyAsks, depth.Asks[:])
	if !ok {
		asks, _ = scanDepthSide(payload, keyAsksAlt, depth.Asks[:])
	}
	depth.BidsLength = bids
	depth.AsksLength = asks
	if depth.BidsLength == 0 && depth.AsksLength == 0 {
		return depth, ErrInvalidPayload
	}
	return depth, nil
}

func decodeBinanceOrder(payload []byte, symbol adapter.Symbol) (adapter.Order, error) {
	var order adapter.Order
	order.Symbol = symbol
	order.Platform = enum.PlatformBinance
	order.RecvTsNano = time.Now().UnixNano()

	if eventMs, ok := scanUintField(payload, keyEventTime); ok {
		order.EventTsNano = int64(eventMs) * int64(time.Millisecond)
	}
	if updateMs, ok := scanUintField(payload, keyTradeTime); ok {
		order.UpdatedTime = int64(updateMs) * int64(time.Millisecond)
	} else if order.EventTsNano != 0 {
		order.UpdatedTime = order.EventTsNano
	}
	if id, ok := scanUintField(payload, keyOrderID); ok {
		order.ID = int64(id)
	}
	if side, ok := scanStringField(payload, keySide); ok {
		order.Side = mapBinanceSide(side)
	}
	if typ, ok := scanStringField(payload, keyOrderType); ok {
		order.Type = mapBinanceOrderType(typ)
	}
	if status, ok := scanStringField(payload, keyOrderStatus); ok {
		order.Status = mapBinanceOrderStatus(status)
	}
	if tif, ok := scanStringField(payload, keyTimeInForce); ok {
		order.TimeInForce = mapBinanceTimeInForce(tif)
	}

	var qty adapter.Decimal
	var hasQty bool
	if qtyText, ok := scanStringField(payload, keyOrderQty); ok {
		parsed, err := parseDecimal(qtyText)
		if err != nil {
			return order, err
		}
		qty = parsed
		hasQty = true
		order.Quantity = parsed
	}
	if priceText, ok := scanStringField(payload, keyOrderPrice); ok {
		parsed, err := parseDecimal(priceText)
		if err != nil {
			return order, err
		}
		order.Price = parsed
	}

	leftQty := qty
	if cumText, ok := scanStringField(payload, keyCumQty); ok {
		parsed, err := parseDecimal(cumText)
		if err != nil {
			return order, err
		}
		if hasQty {
			leftQty = decimalSub(qty, parsed)
		}
	}
	if hasQty {
		order.LeftQuantity = leftQty
	}

	copy(order.Source[:], "binance")
	if order.ID == 0 && !hasQty {
		return order, ErrInvalidPayload
	}
	return order, nil
}

func scanDepthSide(payload []byte, key []byte, rows []adapter.DepthRow) (int, bool) {
	idx := indexOfFrom(payload, key, 0)
	if idx < 0 {
		return 0, false
	}
	i := idx + len(key)
	for i < len(payload) && payload[i] != ':' {
		i++
	}
	if i >= len(payload) {
		return 0, false
	}
	i++
	for i < len(payload) && isSpace(payload[i]) {
		i++
	}
	if i >= len(payload) || payload[i] != '[' {
		return 0, false
	}
	i++
	count := 0
	for i < len(payload) && count < len(rows) {
		for i < len(payload) && (isSpace(payload[i]) || payload[i] == ',') {
			i++
		}
		if i >= len(payload) || payload[i] == ']' {
			break
		}
		if payload[i] != '[' {
			i++
			continue
		}
		i++
		priceText, ok := scanNextString(payload, &i)
		if !ok {
			return count, false
		}
		qtyText, ok := scanNextString(payload, &i)
		if !ok {
			return count, false
		}
		price, err := parseDecimal(priceText)
		if err != nil {
			return count, false
		}
		qty, err := parseDecimal(qtyText)
		if err != nil {
			return count, false
		}
		rows[count] = adapter.DepthRow{Price: price, Quantity: qty}
		count++
		for i < len(payload) && payload[i] != ']' {
			i++
		}
		if i < len(payload) {
			i++
		}
	}
	return count, true
}

func scanNextString(payload []byte, idx *int) ([]byte, bool) {
	i := *idx
	for i < len(payload) && payload[i] != '"' {
		i++
	}
	if i >= len(payload) {
		*idx = i
		return nil, false
	}
	i++
	start := i
	for i < len(payload) && payload[i] != '"' {
		i++
	}
	if i >= len(payload) {
		*idx = i
		return nil, false
	}
	*idx = i + 1
	return payload[start:i], true
}

func scanStringField(payload []byte, key []byte) ([]byte, bool) {
	start := 0
	for {
		idx := indexOfFrom(payload, key, start)
		if idx < 0 {
			return nil, false
		}
		i := idx + len(key)
		for i < len(payload) && payload[i] != ':' {
			i++
		}
		if i >= len(payload) {
			return nil, false
		}
		i++
		for i < len(payload) && isSpace(payload[i]) {
			i++
		}
		if i >= len(payload) {
			return nil, false
		}
		if payload[i] != '"' {
			start = idx + len(key)
			continue
		}
		i++
		valueStart := i
		for i < len(payload) && payload[i] != '"' {
			i++
		}
		if i >= len(payload) {
			return nil, false
		}
		return payload[valueStart:i], true
	}
}

func scanUintField(payload []byte, key []byte) (uint64, bool) {
	start := 0
	for {
		idx := indexOfFrom(payload, key, start)
		if idx < 0 {
			return 0, false
		}
		i := idx + len(key)
		for i < len(payload) && payload[i] != ':' {
			i++
		}
		if i >= len(payload) {
			return 0, false
		}
		i++
		for i < len(payload) && isSpace(payload[i]) {
			i++
		}
		if i >= len(payload) || payload[i] < '0' || payload[i] > '9' {
			start = idx + len(key)
			continue
		}
		var v uint64
		for i < len(payload) && payload[i] >= '0' && payload[i] <= '9' {
			v = v*10 + uint64(payload[i]-'0')
			i++
		}
		return v, true
	}
}

func indexOfFrom(payload []byte, key []byte, start int) int {
	if len(key) == 0 || start < 0 || start >= len(payload) {
		return -1
	}
	if len(payload)-start < len(key) {
		return -1
	}
outer:
	for i := start; i <= len(payload)-len(key); i++ {
		for j := 0; j < len(key); j++ {
			if payload[i+j] != key[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func parseDecimal(src []byte) (adapter.Decimal, error) {
	if len(src) == 0 {
		return adapter.Decimal{}, ErrInvalidPayload
	}
	neg := false
	i := 0
	if src[0] == '-' {
		neg = true
		i++
	}
	var intPart int64
	var fracPart int64
	scale := 0
	seenDot := false
	for ; i < len(src); i++ {
		c := src[i]
		if c == '.' {
			if seenDot {
				return adapter.Decimal{}, ErrInvalidPayload
			}
			seenDot = true
			continue
		}
		if c < '0' || c > '9' {
			return adapter.Decimal{}, ErrInvalidPayload
		}
		digit := int64(c - '0')
		if !seenDot {
			if intPart > (maxInt64-digit)/10 {
				return adapter.Decimal{}, ErrInvalidPayload
			}
			intPart = intPart*10 + digit
			continue
		}
		if fracPart > (maxInt64-digit)/10 {
			return adapter.Decimal{}, ErrInvalidPayload
		}
		fracPart = fracPart*10 + digit
		scale++
	}
	value := intPart
	if scale > 0 {
		mul := int64(1)
		for i := 0; i < scale; i++ {
			if mul > maxInt64/10 {
				return adapter.Decimal{}, ErrInvalidPayload
			}
			mul *= 10
		}
		if intPart > maxInt64/mul {
			return adapter.Decimal{}, ErrInvalidPayload
		}
		value = intPart*mul + fracPart
	}
	if neg {
		value = -value
	}
	return adapter.Decimal{Integer: value, Scale: scale}, nil
}

func decimalSub(a adapter.Decimal, b adapter.Decimal) adapter.Decimal {
	scale := a.Scale
	if b.Scale > scale {
		scale = b.Scale
	}
	ai, ok := rescaleDecimal(a, scale)
	if !ok {
		return adapter.Decimal{Integer: 0, Scale: scale}
	}
	bi, ok := rescaleDecimal(b, scale)
	if !ok {
		return adapter.Decimal{Integer: 0, Scale: scale}
	}
	return adapter.Decimal{Integer: ai - bi, Scale: scale}
}

func rescaleDecimal(d adapter.Decimal, scale int) (int64, bool) {
	if scale < d.Scale {
		return 0, false
	}
	mul := int64(1)
	for i := 0; i < scale-d.Scale; i++ {
		if mul > maxInt64/10 {
			return 0, false
		}
		mul *= 10
	}
	if d.Integer > maxInt64/mul || d.Integer < -maxInt64/mul {
		return 0, false
	}
	return d.Integer * mul, true
}

func mapBinanceSide(value []byte) enum.OrderSide {
	switch {
	case bytes.Equal(value, valueBuy):
		return enum.OrderSideBuy
	case bytes.Equal(value, valueSell):
		return enum.OrderSideSell
	default:
		return 0
	}
}

func mapBinanceOrderType(value []byte) enum.OrderType {
	switch {
	case bytes.Equal(value, valueLimit):
		return enum.OrderTypeLimit
	case bytes.Equal(value, valueMarket):
		return enum.OrderTypeMarket
	default:
		return 0
	}
}

func mapBinanceOrderStatus(value []byte) enum.OrderStatus {
	switch {
	case bytes.Equal(value, valueNew):
		return enum.OrderStatusPlaced
	case bytes.Equal(value, valuePartial):
		return enum.OrderStatusPartialFilled
	case bytes.Equal(value, valueFilled):
		return enum.OrderStatusFilled
	case bytes.Equal(value, valueCanceled), bytes.Equal(value, valueCancelled):
		return enum.OrderStatusCanceled
	case bytes.Equal(value, valueExpired):
		return enum.OrderStatusExpired
	default:
		return 0
	}
}

func mapBinanceTimeInForce(value []byte) enum.OrderTimeInForce {
	switch {
	case bytes.Equal(value, valueGTC):
		return enum.OrderTimeInForceGTC
	case bytes.Equal(value, valueIOC):
		return enum.OrderTimeInForceIOC
	case bytes.Equal(value, valueFOK):
		return enum.OrderTimeInForceFOK
	default:
		return 0
	}
}
