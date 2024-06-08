package utils

import (
	"bufio"
	"os"
)

type Writer interface {
	Write(p []byte) (n int, err error)
	Flush() error
}

type FileWriter interface {
	Create(name string) (Writer, error)
	Close() error
}

type OSFileWriter struct {
	Outfile *os.File
	Writer  Writer
}

func (osfc OSFileWriter) Create(name string) (Writer, error) {
	var err error
	osfc.Outfile, err = os.Create(name)
	if err != nil {
		return nil, err
	}
	osfc.Writer = bufio.NewWriter(osfc.Outfile)
	return osfc.Writer, nil
}

func (osfc OSFileWriter) Close() error {
	if err := osfc.Writer.Flush(); err != nil {
		return err
	}
	if err := osfc.Outfile.Close(); err != nil {
		return err
	}

	return nil
}
