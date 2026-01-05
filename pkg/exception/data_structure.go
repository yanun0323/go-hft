package exception

import "github.com/yanun0323/errors"

var (
	ErrIndexOutOfRange     = errors.New("index out of range")
	ErrNilInstance         = errors.New("nil instance")
	ErrTypeUnsupported     = errors.New("type unsupported")
	ErrArgumentUnsupported = errors.New("argument unsupported")
	ErrInternal            = errors.New("internal error")
)
