package btcc

type Response[T any] struct {
	ID    int64         `json:"id"`
	Error ResponseError `json:"error,omitempty"`
	Data  T             `json:"result"`
}

type ResponseError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type ResponsePlaceLimitOrder struct {
	ID          int64   `json:"id"`
	Type        int     `json:"type"`
	Side        int     `json:"side"`
	User        int64   `json:"user"`
	Account     int64   `json:"account"`
	Option      int     `json:"option"`
	Ctime       float64 `json:"ctime"`
	Mtime       float64 `json:"mtime"`
	Market      string  `json:"market"`
	Source      string  `json:"source"`
	ClientID    string  `json:"client_id"`
	Price       string  `json:"price"`
	Amount      string  `json:"amount"`
	TakerFee    string  `json:"taker_fee"`
	MakerFee    string  `json:"maker_fee"`
	Left        string  `json:"left"`
	DealStock   string  `json:"deal_stock"`
	DealMoney   string  `json:"deal_money"`
	DealFee     string  `json:"deal_fee"`
	AssetFee    string  `json:"asset_fee"`
	FeeDiscount string  `json:"fee_discount"`
	FeeAsset    string  `json:"fee_asset"`
}
