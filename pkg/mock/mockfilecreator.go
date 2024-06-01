package mock

import "os"

type MockFileCreator struct {
	Err error
}

func (m MockFileCreator) Create(name string) (*os.File, error) {
	return nil, m.Err
}
