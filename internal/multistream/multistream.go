package multistream

import (
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

type Publisher struct {
	sink io.Writer
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
	done bool
	err  error
}

func NewPublisher(sink io.Writer) *Publisher {
	return &Publisher{sink: sink, subs: make(map[chan struct{}]struct{}, 1)}
}

func (p *Publisher) Write(b []byte) (int, error) {
	n, err := p.sink.Write(b)
	if n > 0 {
		p.mu.Lock()
		for ch := range p.subs {
			select {
			case ch <- struct{}{}:
			default: // A token was not read yet, skip
			}
		}
		p.mu.Unlock()
	}

	return n, err
}

func (p *Publisher) Finish(err error) {
	p.mu.Lock()
	if p.done {
		p.mu.Unlock()
		return
	}

	p.done = true
	p.err = err

	snapshot := make([]chan struct{}, 0, len(p.subs))
	for ch := range p.subs {
		snapshot = append(snapshot, ch)
	}
	p.subs = nil
	p.mu.Unlock()

	for _, ch := range snapshot {
		close(ch)
	}
}

func (p *Publisher) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	p.mu.Lock()
	if p.done {
		// We're closing already, let's not block the other side, nothing will come
		close(ch)
	} else {
		p.subs[ch] = struct{}{}
	}
	p.mu.Unlock()

	return ch
}

func (p *Publisher) Unsubscribe(ch chan struct{}) {
	p.mu.Lock()
	delete(p.subs, ch)
	p.mu.Unlock()
}

type StreamingFileReader struct {
	fp       *os.File
	pub      *Publisher
	notify   chan struct{}
	closed   atomic.Bool
	closedCh chan struct{}
}

func NewStreamingFileReader(path string, pub *Publisher) (*StreamingFileReader, error) {
	fp, err := os.Open(path) //nolint:gosec
	if err != nil {
		return nil, err
	}

	return &StreamingFileReader{
		fp:       fp,
		pub:      pub,
		notify:   pub.subscribe(),
		closedCh: make(chan struct{}),
	}, nil
}

func (s *StreamingFileReader) Clone() (*StreamingFileReader, error) {
	return NewStreamingFileReader(s.fp.Name(), s.pub)
}

func (s *StreamingFileReader) Close() error {
	if s.closed.Swap(true) {
		return os.ErrClosed
	}

	s.pub.Unsubscribe(s.notify)
	close(s.closedCh)
	return s.fp.Close()
}

func (s *StreamingFileReader) Read(b []byte) (int, error) {
	for {
		n, err := s.fp.Read(b)
		if n > 0 {
			return n, nil
		}

		if err != nil && !errors.Is(err, io.EOF) {
			return n, err
		}

		// At EOF, we need to wait
		select {
		case _, ok := <-s.notify:
			if ok {
				// new data? retry
				continue
			}
			// Closed, let's finish consuming
			n, err := s.fp.Read(b)
			if n > 0 {
				return n, nil
			}
			if err != nil && !errors.Is(err, io.EOF) {
				return n, err
			}
			if s.pub.err != nil {
				return 0, s.pub.err
			}
			return 0, io.EOF
		case <-s.closedCh:
			return 0, os.ErrClosed
		}
	}
}
