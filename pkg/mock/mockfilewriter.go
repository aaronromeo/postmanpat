package mock

import (
	"bytes"
	"os"

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
	Writers map[string]MockWriter
	Mkdirs  map[string]os.FileMode
}

func (m MockFileWriter) Create(name string) (utils.Writer, error) {
	writer := MockWriter{Buffer: new(bytes.Buffer)}
	m.Writers[name] = writer
	return writer, m.Err
}

func (m MockFileWriter) Close() error {
	return m.Err
}

func (m MockFileWriter) MkdirAll(path string, perm os.FileMode) error {
	if m.Mkdirs == nil {
		m.Mkdirs = make(map[string]os.FileMode)
	}
	m.Mkdirs[path] = perm
	return m.Err
}

func (m MockFileWriter) WriteFile(filename string, data []byte, perm os.FileMode) error {
	if m.Writers == nil {
		m.Writers = make(map[string]MockWriter)
	}
	m.Writers[filename] = MockWriter{Buffer: bytes.NewBuffer(data)}
	return m.Err
}
