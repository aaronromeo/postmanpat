package utils

import (
	"errors"
	"fmt"
	"runtime"
)

func WrapError(err error) error {
	_, file, line, _ := runtime.Caller(1)
	return errors.New(fmt.Sprintf("error at %s:%d: %v", file, line, err))
}
