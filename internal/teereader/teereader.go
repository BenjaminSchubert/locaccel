package teereader

import "io"

type TeeReader struct {
	src          io.Reader
	dest         io.Writer
	onClose      func(totalRead int, readErr, writeErr error) error
	lastReadErr  error
	lastWriteErr error
	hasError     bool
	totalRead    int
}

func New(
	src io.Reader,
	dest io.Writer,
	onClose func(totalRead int, readErr, writeErr error) error,
) *TeeReader {
	return &TeeReader{src: src, dest: dest, onClose: onClose}
}

func (t *TeeReader) Read(p []byte) (int, error) {
	if t.hasError {
		n, err := t.src.Read(p)
		t.totalRead += n
		return n, err
	}

	n, readErr := t.src.Read(p)
	if readErr != nil && readErr != io.EOF {
		t.hasError = true
		t.lastReadErr = readErr
	}

	if n > 0 {
		if _, writeErr := t.dest.Write(p[:n]); writeErr != nil {
			t.hasError = true
			t.lastWriteErr = writeErr
		}
	}

	t.totalRead += n
	return n, readErr
}

func (t *TeeReader) Close() error {
	return t.onClose(t.totalRead, t.lastReadErr, t.lastWriteErr)
}
