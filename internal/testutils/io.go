package testutils

import (
	"io"
	"sync"
)

type StuckReader struct {
	released chan struct{}
	once     sync.Once
	data     []byte
}

func NewStuckReader(data []byte) *StuckReader {
	return &StuckReader{released: make(chan struct{}), data: data}
}

func (s *StuckReader) Read(out []byte) (int, error) {
	<-s.released

	if len(s.data) == 0 {
		return 0, io.EOF
	}

	nWrite := min(len(out), len(s.data))
	copy(out, s.data[:nWrite])
	s.data = s.data[nWrite:]

	return nWrite, nil
}

func (s *StuckReader) Free() {
	s.once.Do(func() { close(s.released) })
}

func (s *StuckReader) Close() error {
	return nil
}

var _ io.ReadCloser = (*StuckReader)(nil)

type ReadCloserNotifier struct {
	io.Reader
	Notify chan struct{}
}

func (r ReadCloserNotifier) Close() error {
	r.Notify <- struct{}{}
	return nil
}

var _ io.ReadCloser = (*ReadCloserNotifier)(nil)
