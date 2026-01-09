package marketdata_old

import (
	"encoding/json"
	"main/internal/errors"
	"main/pkg/exception"
)

const (
	_btccBaseWsUrl    = "wss://spotprice2.btcccdn.com/ws"
	_btccBaseWsUrlDev = "wss://spot.cryptouat.com:8700/ws"

	btccWsMethodAuthID  = 1
	btccWsMethodDepthID = 2
	btccWsMethodOrderID = 3
)

type BtccSubscribeResponse struct {
	ID int `json:"id"`

	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`

	Result struct {
		Status string `json:"status"`
	} `json:"result"`
}

type BtccResponse struct {
	ID     any               `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

func (r BtccResponse) Unmarshal(index int, p any) error {
	if index >= len(r.Params) {
		return exception.ErrIndexOutOfRange
	}

	if err := json.Unmarshal(r.Params[index], p); err != nil {
		return errors.Wrap(err, "unmarshal")
	}

	return nil
}
