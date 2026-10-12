package filecache

import "io"

type notifyingWriter struct {
	w      io.Writer
	notify chan struct{}
}

func newNotifyingWriter(w io.Writer) *notifyingWriter {
	return &notifyingWriter{w, make(chan struct{}, 1)}
}

func (n notifyingWriter) Write(p []byte) (int, error) {
	written, err := n.w.Write(p)
	if written > 0 {
		select {
		case n.notify <- struct{}{}:
		default:
		}
	}
	return written, err
}
