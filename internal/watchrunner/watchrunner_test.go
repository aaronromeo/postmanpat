package watchrunner

import "testing"

func TestIsBenignIdleError(t *testing.T) {
	if !IsBenignIdleError(nil) {
		t.Fatal("expected nil error to be benign")
	}
	if !IsBenignIdleError(errString("use of closed network connection")) {
		t.Fatal("expected closed network connection error to be benign")
	}
	if IsBenignIdleError(errString("some other error")) {
		t.Fatal("expected other error to be non-benign")
	}
}

type errString string

func (e errString) Error() string {
	return string(e)
}
