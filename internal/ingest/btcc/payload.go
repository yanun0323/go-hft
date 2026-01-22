package btcc

import (
	"errors"
	"main/internal/adapter"
	"main/internal/adapter/enum"
	"strconv"
	"time"
)

var (
	ErrInvalidPayload = errors.New("btcc: invalid payload")
	ErrInvalidRequest = errors.New("btcc: invalid request")
)

const maxUint16 = int(^uint16(0))

var (
	keyAsks        = []byte(`"asks"`)
	keyBids        = []byte(`"bids"`)
	keyTime        = []byte(`"time"`)
	keyOrderID     = []byte(`"id"`)
	keyOrderSide   = []byte(`"side"`)
	keyOrderType   = []byte(`"type"`)
	keyOrderPrice  = []byte(`"price"`)
	keyOrderAmount = []byte(`"amount"`)
	keyOrderLeft   = []byte(`"left"`)
	keyOrderDeal   = []byte(`"deal_stock"`)
	keyOrderCTime  = []byte(`"ctime"`)
	keyOrderMTime  = []byte(`"mtime"`)
	keyOrderOption = []byte(`"option"`)
)

// DecodeMarketDataPayload converts a websocket payload into an encoded market data payload.
func DecodeMarketDataPayload(topic enum.Topic, symbol adapter.Symbol, payload []byte) ([]byte, error) {
	if !topic.IsAvailable() {
		return nil, ErrInvalidRequest
	}
	if len(payload) == 0 {
		return nil, ErrInvalidPayload
	}
	return decodeBtccPayload(topic, symbol, payload)
}

func decodeBtccPayload(topic enum.Topic, symbol adapter.Symbol, payload []byte) ([]byte, error) {
	switch topic {
	case enum.TopicDepth:
		depth, err := decodeBtccDepth(payload, symbol)
		if err != nil {
			return nil, err
		}
		return depth.Encode(nil), nil
	case enum.TopicOrder:
		order, err := decodeBtccOrder(payload, symbol)
		if err != nil {
			return nil, err
		}
		return order.Encode(nil), nil
	default:
		return nil, ErrInvalidRequest
	}
}

func decodeBtccDepth(payload []byte, symbol adapter.Symbol) (adapter.Depth, error) {
	var depth adapter.Depth
	depth.Symbol = symbol
	depth.Platform = enum.PlatformBTCC
	depth.RecvTsNano = time.Now().UnixNano()

	orderbook, ok := scanParamsValue(payload, 1)
	if !ok {
		return depth, ErrInvalidPayload
	}
	if eventText, ok := scanNumberField(orderbook, keyTime); ok {
		if eventTs, ok := parseJSONTimestamp(eventText); ok {
			depth.EventTsNano = eventTs
		}
	}

	bids, ok := scanDepthSide(orderbook, keyBids, depth.Bids[:])
	if !ok {
		bids = 0
	}
	asks, ok := scanDepthSide(orderbook, keyAsks, depth.Asks[:])
	if !ok {
		asks = 0
	}
	depth.BidsLength = bids
	depth.AsksLength = asks
	if depth.BidsLength == 0 && depth.AsksLength == 0 {
		return depth, ErrInvalidPayload
	}
	return depth, nil
}

func decodeBtccOrder(payload []byte, symbol adapter.Symbol) (adapter.Order, error) {
	var order adapter.Order
	order.Symbol = symbol
	order.Platform = enum.PlatformBTCC
	order.RecvTsNano = time.Now().UnixNano()

	if statusVal, ok := scanParamsInt(payload, 0); ok {
		order.Status = mapBtccOrderStatus(statusVal)
	}

	orderValue, ok := scanParamsValue(payload, 1)
	if !ok {
		return order, ErrInvalidPayload
	}
	if idText, ok := scanNumberField(orderValue, keyOrderID); ok {
		if id, ok := parseJSONInt(idText); ok {
			order.ID = id
		}
	}
	if sideText, ok := scanNumberField(orderValue, keyOrderSide); ok {
		if side, ok := parseJSONInt(sideText); ok {
			order.Side = mapBtccSide(side)
		}
	}
	if typeText, ok := scanNumberField(orderValue, keyOrderType); ok {
		if typ, ok := parseJSONInt(typeText); ok {
			order.Type = mapBtccOrderType(typ)
		}
	}
	if optionText, ok := scanNumberField(orderValue, keyOrderOption); ok {
		if tif, ok := parseJSONInt(optionText); ok {
			order.TimeInForce = mapBtccTimeInForce(tif)
		}
	}
	if priceText, ok := scanNumberField(orderValue, keyOrderPrice); ok {
		if price, err := parseDecimal(priceText); err == nil {
			order.Price = adapter.Price(price)
		}
	}

	var qty adapter.Decimal
	var hasQty bool
	if qtyText, ok := scanNumberField(orderValue, keyOrderAmount); ok {
		if parsed, err := parseDecimal(qtyText); err == nil {
			qty = parsed
			hasQty = true
			order.Quantity = adapter.Quantity(parsed)
		}
	}

	if leftText, ok := scanNumberField(orderValue, keyOrderLeft); ok {
		if parsed, err := parseDecimal(leftText); err == nil {
			order.LeftQuantity = adapter.Quantity(parsed)
		}
	} else if dealText, ok := scanNumberField(orderValue, keyOrderDeal); ok {
		if parsed, err := parseDecimal(dealText); err == nil && hasQty {
			left := decimalSub(qty, parsed)
			order.LeftQuantity = adapter.Quantity(left)
		}
	}

	if ctimeText, ok := scanNumberField(orderValue, keyOrderCTime); ok {
		if ts, ok := parseJSONTimestamp(ctimeText); ok {
			order.EventTsNano = ts
		}
	}
	if mtimeText, ok := scanNumberField(orderValue, keyOrderMTime); ok {
		if ts, ok := parseJSONTimestamp(mtimeText); ok {
			order.UpdatedTime = ts
		}
	}
	if order.UpdatedTime == 0 {
		order.UpdatedTime = order.EventTsNano
	}
	copy(order.Source[:], "btcc")

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
		priceText, ok := scanNextValue(payload, &i)
		if !ok {
			return count, false
		}
		qtyText, ok := scanNextValue(payload, &i)
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
		rows[count] = adapter.DepthRow{Price: adapter.Price(price), Quantity: adapter.Quantity(qty)}
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

func scanNextValue(payload []byte, idx *int) ([]byte, bool) {
	i := *idx
	for i < len(payload) && isSpace(payload[i]) {
		i++
	}
	if i >= len(payload) {
		*idx = i
		return nil, false
	}
	if payload[i] == '"' {
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
	start := i
	for i < len(payload) && payload[i] != ',' && payload[i] != ']' && !isSpace(payload[i]) {
		i++
	}
	if start == i {
		*idx = i
		return nil, false
	}
	*idx = i
	return payload[start:i], true
}

func scanStringField(payload []byte, key []byte) ([]byte, bool) {
	idx := indexOfFrom(payload, key, 0)
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
	if i >= len(payload) || payload[i] != '"' {
		return nil, false
	}
	i++
	start := i
	for i < len(payload) && payload[i] != '"' {
		i++
	}
	if i >= len(payload) {
		return nil, false
	}
	return payload[start:i], true
}

func scanNumberField(payload []byte, key []byte) ([]byte, bool) {
	idx := indexOfFrom(payload, key, 0)
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
	if payload[i] == '"' {
		i++
		start := i
		for i < len(payload) && payload[i] != '"' {
			i++
		}
		if i >= len(payload) {
			return nil, false
		}
		return payload[start:i], true
	}
	start := i
	if payload[i] == '-' {
		i++
	}
	for i < len(payload) {
		c := payload[i]
		if (c < '0' || c > '9') && c != '.' {
			break
		}
		i++
	}
	if start == i {
		return nil, false
	}
	return payload[start:i], true
}

func scanParamsValue(payload []byte, index int) ([]byte, bool) {
	if index < 0 {
		return nil, false
	}
	idx := indexOfFrom(payload, []byte(`"params"`), 0)
	if idx < 0 {
		return nil, false
	}
	i := idx + len(`"params"`)
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
	if i >= len(payload) || payload[i] != '[' {
		return nil, false
	}
	i++
	for idx := 0; i < len(payload); idx++ {
		for i < len(payload) && (isSpace(payload[i]) || payload[i] == ',') {
			i++
		}
		if i >= len(payload) || payload[i] == ']' {
			return nil, false
		}
		start := i
		end := skipValue(payload, i)
		if end <= i {
			return nil, false
		}
		if idx == index {
			return payload[start:end], true
		}
		i = end
	}
	return nil, false
}

func scanParamsString(payload []byte, index int) ([]byte, bool) {
	raw, ok := scanParamsValue(payload, index)
	if !ok {
		return nil, false
	}
	raw = trimSpace(raw)
	if len(raw) < 2 || raw[0] != '"' {
		return nil, false
	}
	start := 1
	i := start
	for i < len(raw) && raw[i] != '"' {
		if raw[i] == '\\' && i+1 < len(raw) {
			i += 2
			continue
		}
		i++
	}
	if i >= len(raw) {
		return nil, false
	}
	return raw[start:i], true
}

func scanParamsInt(payload []byte, index int) (int64, bool) {
	raw, ok := scanParamsValue(payload, index)
	if !ok {
		return 0, false
	}
	return parseJSONInt(raw)
}

func skipValue(payload []byte, i int) int {
	i = skipSpaces(payload, i)
	if i >= len(payload) {
		return i
	}
	switch payload[i] {
	case '"':
		return skipString(payload, i)
	case '{':
		return skipObject(payload, i)
	case '[':
		return skipArray(payload, i)
	default:
		for i < len(payload) {
			c := payload[i]
			if c == ',' || c == ']' || c == '}' {
				return i
			}
			i++
		}
		return i
	}
}

func skipString(payload []byte, i int) int {
	i++
	for i < len(payload) {
		if payload[i] == '\\' {
			i += 2
			continue
		}
		if payload[i] == '"' {
			return i + 1
		}
		i++
	}
	return i
}

func skipObject(payload []byte, i int) int {
	depth := 0
	for i < len(payload) {
		switch payload[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		case '"':
			i = skipString(payload, i) - 1
		}
		i++
	}
	return i
}

func skipArray(payload []byte, i int) int {
	depth := 0
	for i < len(payload) {
		switch payload[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i + 1
			}
		case '"':
			i = skipString(payload, i) - 1
		}
		i++
	}
	return i
}

func skipSpaces(payload []byte, i int) int {
	for i < len(payload) && isSpace(payload[i]) {
		i++
	}
	return i
}

func trimSpace(payload []byte) []byte {
	start := 0
	end := len(payload)
	for start < end && isSpace(payload[start]) {
		start++
	}
	for end > start && isSpace(payload[end-1]) {
		end--
	}
	return payload[start:end]
}

func parseJSONInt(raw []byte) (int64, bool) {
	raw = trimSpace(raw)
	if len(raw) == 0 {
		return 0, false
	}
	if raw[0] == '"' {
		if len(raw) < 2 {
			return 0, false
		}
		raw = raw[1:]
		end := 0
		for end < len(raw) && raw[end] != '"' {
			end++
		}
		raw = raw[:end]
	}
	v, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseJSONTimestamp(raw []byte) (int64, bool) {
	raw = trimSpace(raw)
	if len(raw) == 0 {
		return 0, false
	}
	if raw[0] == '"' {
		if len(raw) < 2 {
			return 0, false
		}
		raw = raw[1:]
		end := 0
		for end < len(raw) && raw[end] != '"' {
			end++
		}
		raw = raw[:end]
	}
	v, err := strconv.ParseFloat(string(raw), 64)
	if err != nil {
		return 0, false
	}
	if v <= 0 {
		return 0, false
	}
	if v > 1e12 {
		return int64(v) * int64(time.Millisecond), true
	}
	return int64(v * float64(time.Second)), true
}

const maxInt64 = int64(^uint64(0) >> 1)

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

func mapBtccSide(value int64) enum.OrderSide {
	switch value {
	case 1:
		return enum.OrderSideBuy
	case 2:
		return enum.OrderSideSell
	default:
		return 0
	}
}

func mapBtccOrderType(value int64) enum.OrderType {
	switch value {
	case 1:
		return enum.OrderTypeLimit
	case 2:
		return enum.OrderTypeMarket
	default:
		return 0
	}
}

func mapBtccOrderStatus(value int64) enum.OrderStatus {
	switch value {
	case 1:
		return enum.OrderStatusPlaced
	case 2:
		return enum.OrderStatusPartialFilled
	case 3:
		return enum.OrderStatusFilled
	case 4:
		return enum.OrderStatusCanceled
	case 5:
		return enum.OrderStatusExpired
	default:
		return 0
	}
}

func mapBtccTimeInForce(value int64) enum.OrderTimeInForce {
	switch value {
	case 1:
		return enum.OrderTimeInForceGTC
	case 2:
		return enum.OrderTimeInForceIOC
	case 3:
		return enum.OrderTimeInForceFOK
	default:
		return 0
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

func hashBytes(data []byte) uint64 {
	const offset64 = 14695981039346656037
	const prime64 = 1099511628211
	var hash uint64 = offset64
	for i := range data {
		hash ^= uint64(data[i])
		hash *= prime64
	}
	return hash
}

func appendUint(dst []byte, v uint64) []byte {
	if v == 0 {
		return append(dst, '0')
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return append(dst, buf[i:]...)
}
