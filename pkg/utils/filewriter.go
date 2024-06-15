package utils

import (
	"bufio"
	"os"
)

type Writer interface {
	Write(p []byte) (n int, err error)
	Flush() error
}

type FileManager interface {
	Close() error
	Create(name string) (Writer, error)
	MkdirAll(path string, perm os.FileMode) error
	WriteFile(filename string, data []byte, perm os.FileMode) error
}

type OSFileManager struct {
	Outfile *os.File
	Writer  Writer
}

func (osfc OSFileManager) Create(name string) (Writer, error) {
	var err error
	osfc.Outfile, err = os.Create(name)
	if err != nil {
		return nil, err
	}
	osfc.Writer = bufio.NewWriter(osfc.Outfile)
	return osfc.Writer, nil
}

func (osfc OSFileManager) Close() error {
	if err := osfc.Writer.Flush(); err != nil {
		return err
	}
	if err := osfc.Outfile.Close(); err != nil {
		return err
	}

	return nil
}

func (osfc OSFileManager) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (osfc OSFileManager) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filename, data, perm)
}
