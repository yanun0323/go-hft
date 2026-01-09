package exception

import "errors"

// UDS errors
var (
	// ErrEmptyPathUDS is returned when a socket path is empty.
	ErrEmptyPathUDS = errors.New("uds: empty path")

	// ErrNilClientUDS is returned when a nil client receiver is used.
	ErrNilClientUDS = errors.New("uds: nil client")
)
