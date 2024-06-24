package utils

import (
	"fmt"
	"runtime"
)

func WrapError(err error) error {
	_, file, line, _ := runtime.Caller(1)
	return fmt.Errorf("error at %s:%d: %v", file, line, err)
}
