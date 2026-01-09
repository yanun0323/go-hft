package errors

import "testing"

func TestWrap(t *testing.T) {
	err := Wrap(errWrapped, "Hello, Wrapped!")
	if err.Error() != "Hello, Wrapped!, err: wrapped error" {
		t.Fatalf("error mismatch: %+v", err)
	}
}
