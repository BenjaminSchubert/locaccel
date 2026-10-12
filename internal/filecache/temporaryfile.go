package filecache

import (
	"io"
	"os"
	"sync/atomic"
)

type temporaryFileReader struct {
	fp       io.ReadCloser
	notify   chan struct{} // kick: data may have arrived
	doneCh   chan struct{} // closed: ingest finished (err carries the outcome)
	closedCh chan struct{} // closed: reader closed
	err      error
	closed   atomic.Bool
	done     bool
}

func newTemporaryFileReaderWithPoll(fp *os.File, notifier chan struct{}) *temporaryFileReader {
	return &temporaryFileReader{
		fp:       fp,
		notify:   notifier,
		doneCh:   make(chan struct{}),
		closedCh: make(chan struct{}),
	}
}

func (t *temporaryFileReader) Close() error {
	if t.closed.Swap(true) {
		return ErrAlreadyClosed
	}

	close(t.closedCh)
	return t.fp.Close()
}

func (t *temporaryFileReader) markCompleted(err error) {
	t.err = err
	close(t.doneCh)
}

func (t *temporaryFileReader) Read(b []byte) (int, error) {
	for {
		n, err := t.fp.Read(b)
		if n > 0 {
			return n, nil
		}
		if err != nil && (err != io.EOF || t.done) {
			if t.err != nil {
				return n, t.err
			}
			return n, err
		}

		select {
		case <-t.doneCh:
			t.done = true
		case <-t.notify:
			// data might be there now; loop and retry the read
		case <-t.closedCh:
			return 0, os.ErrClosed
		}
	}
}

var _ io.ReadCloser = (*temporaryFileReader)(nil)
