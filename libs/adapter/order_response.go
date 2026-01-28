package adapter

import "main/libs/shared/exception"

const (
	CodeInternalError = -999
)

type OrderResponse struct {
	Request OrderRequest
	OrderID Str64
	Sent    bool
	Code    int
	Msg     Str64
}

func (resp *OrderResponse) SetInternalError(err error) {
	resp.Code = CodeInternalError
	resp.Msg = NewStr64(err.Error())
}

func (resp *OrderResponse) SetError(code int, msg string) {
	resp.Code = code
	resp.Msg = NewStr64(msg)
}

func (resp OrderResponse) Error() error {
	if !resp.Sent {
		return exception.ErrOrderRequestNotSent
	}

	if resp.Code != 0 {
		return exception.ErrOrderResponseBTCCCode
	}

	if resp.Msg.Len() != 0 {
		return exception.ErrOrderResponseBTCCMsg
	}

	return nil
}
