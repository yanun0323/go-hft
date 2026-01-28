package exception

import "errors"

// General errors
var (
	ErrIndexOutOfRange     = errors.New("index out of range")
	ErrNilInstance         = errors.New("nil instance")
	ErrTypeUnsupported     = errors.New("type unsupported")
	ErrArgumentUnsupported = errors.New("argument unsupported")
	ErrInternal            = errors.New("internal error")
	ErrInResponseError     = errors.New("there is an error in response error field")
	ErrInvalidArgument     = errors.New("invalid argument")
	ErrBuffTooSmall  = errors.New("encode buff is too small")
)
