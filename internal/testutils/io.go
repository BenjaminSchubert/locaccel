package testutils

import (
	"io"
	"time"
)

type SlowReader struct {
	data  []byte
	delay time.Duration
}

func NewSlowReader(data []byte, delay time.Duration) *SlowReader {
	return &SlowReader{data, delay}
}

func (s *SlowReader) Read(out []byte) (int, error) {
	time.Sleep(s.delay)

	if len(s.data) == 0 {
		return 0, io.EOF
	}

	nWrite := min(len(out), len(s.data))
	copy(out, s.data[:nWrite])
	s.data = s.data[nWrite:]

	return nWrite, nil
}

func (s *SlowReader) Close() error {
	return nil
}

var _ io.ReadCloser = (*SlowReader)(nil)

type ReadCloserNotifier struct {
	io.Reader
	Notify chan struct{}
}

func (r ReadCloserNotifier) Close() error {
	r.Notify <- struct{}{}
	return nil
}

var _ io.ReadCloser = (*ReadCloserNotifier)(nil)
