package marketdata

import (
	"encoding/json"
	"main/pkg/exception"

	"github.com/yanun0323/errors"
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
		return errors.Wrapf(exception.ErrIndexOutOfRange, "index: %d, len: %d", index, len(r.Params))
	}

	if err := json.Unmarshal(r.Params[index], p); err != nil {
		return errors.Wrapf(err, "unmarshal from index: %d", index)
	}

	return nil
}
