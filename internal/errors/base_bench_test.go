package errors

import (
	"errors"
	"testing"
)

var errWrapped = errors.New("wrapped error")

func BenchmarkWrap(b *testing.B) {
	b.Run("wrap nil", func(b *testing.B) {
		for b.Loop() {
			err := Wrap(nil, "Hello, Nil Error!")
			_ = err
		}
	})

	b.Run("wrap error", func(b *testing.B) {
		for b.Loop() {
			err := Wrap(errWrapped, "Hello, Wrapped!")
			_ = err.Error()
		}
	})

	b.Run("new error", func(b *testing.B) {
		for b.Loop() {
			err := errors.New("Hello, Error!")
			_ = err.Error()
		}
	})
}
