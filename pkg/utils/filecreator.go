package utils

import "os"

type FileCreator interface {
	Create(name string) (*os.File, error)
}

type OSFileCreator struct{}

func (OSFileCreator) Create(name string) (*os.File, error) {
	return os.Create(name)
}
