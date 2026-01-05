package exception

import "github.com/yanun0323/errors"

var (
	ErrInResponseError = errors.New("there is an error in response error field")
	ErrConnectionClose = errors.New("connection closed")
	ErrInvalidArgument = errors.New("invalid argument")
)
