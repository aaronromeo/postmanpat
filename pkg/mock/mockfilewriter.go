package mock

import (
	"bytes"

	"aaronromeo.com/postmanpat/pkg/utils"
)

type MockWriter struct {
	Buffer *bytes.Buffer
	Err    error
}

func (m MockWriter) Write(p []byte) (int, error) {
	if m.Err != nil {
		return 0, m.Err
	}
	n, err := (*m.Buffer).Write(p[:])
	return n, err
}

func (m MockWriter) Flush() error {
	return m.Err
}

type MockFileWriter struct {
	Err     error
	Writers *[]MockWriter
}

func (m MockFileWriter) Create(name string) (utils.Writer, error) {
	writer := MockWriter{Buffer: new(bytes.Buffer)}
	*m.Writers = append(*m.Writers, writer)
	return writer, m.Err
}

func (m MockFileWriter) Close() error {
	return m.Err
}
