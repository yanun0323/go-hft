package errors

import (
	"errors"
)

var (
	_ error = (*wrappedError)(nil)
)

func New(text string) error {
	return errors.New(text)
}

func Wrap(err error, text string) error {
	if err == nil {
		return nil
	}

	if len(text) == 0 {
		return err
	}

	return &wrappedError{
		err: err,
		msg: text,
	}
}

type wrappedError struct {
	err error
	msg string
}

const sep = ", err: "

func (err wrappedError) Error() string {
	if err.err == nil {
		return err.msg
	}

	return err.msg + sep + err.err.Error()
}

func (err wrappedError) Unwrap() error {
	if err.err == nil {
		return errors.New(err.msg)
	}

	return err.err
}
