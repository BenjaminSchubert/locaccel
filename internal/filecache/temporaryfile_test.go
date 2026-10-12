package filecache

import (
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTempFile(t *testing.T) (wfp, rfp *os.File) {
	t.Helper()
	wfp, err := os.CreateTemp(t.TempDir(), "reader-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, wfp.Close()) })

	rfp, err = os.Open(wfp.Name())
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := rfp.Close(); !errors.Is(err, os.ErrClosed) {
			require.NoError(t, err)
		}
	})

	return wfp, rfp
}

func TestReadsFileAfterCompletion(t *testing.T) {
	t.Parallel()

	wfp, rfp := newTestTempFile(t)
	_, err := wfp.WriteString("1234567890")
	require.NoError(t, err)

	notifier := make(chan struct{}, 1)

	r := newTemporaryFileReaderWithPoll(rfp, notifier)
	r.markCompleted(nil)

	data, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "1234567890", string(data))
}

func TestDoesNotReturnEOFBeforeCompletion(t *testing.T) {
	t.Parallel()

	wfp, rfp := newTestTempFile(t)
	_, err := wfp.WriteString("1234567890")
	require.NoError(t, err)

	notifier := make(chan struct{}, 1)

	r := newTemporaryFileReaderWithPoll(rfp, notifier)

	// Data written so far must be readable...
	buf := make([]byte, 10)
	_, err = r.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "1234567890", string(buf))

	// ...but EOF may not surface until MarkCompleted.
	readDone := make(chan error, 1)
	go func() {
		_, err := r.Read(make([]byte, 10))
		readDone <- err
	}()

	select {
	case err := <-readDone:
		t.Fatalf("Read returned %v before MarkCompleted", err)
	case <-time.After(15 * time.Millisecond):
	}

	r.markCompleted(nil)
	require.ErrorIs(t, <-readDone, io.EOF)
}

func TestReadsChunksAsTheyArrive(t *testing.T) {
	t.Parallel()

	notifier := make(chan struct{}, 1)
	wfp, rfp := newTestTempFile(t)
	r := newTemporaryFileReaderWithPoll(rfp, notifier)

	readDone := make(chan string, 1)
	go func() {
		data, err := io.ReadAll(r)
		if err != nil {
			readDone <- err.Error()
			return
		}
		readDone <- string(data)
	}()

	for _, chunk := range []string{"12345", "678", "90"} {
		// Give the reader time to hit EOF and start polling in between chunks.
		_, err := wfp.WriteString(chunk)
		require.NoError(t, err)
		notifier <- struct{}{}
		time.Sleep(10 * time.Millisecond)

	}
	r.markCompleted(nil)

	select {
	case got := <-readDone:
		assert.Equal(t, "1234567890", got)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("reader did not finish after MarkCompleted")
	}
}

func TestCloseAbortsPendingRead(t *testing.T) {
	t.Parallel()

	notifier := make(chan struct{}, 1)

	_, rfp := newTestTempFile(t)
	r := newTemporaryFileReaderWithPoll(rfp, notifier)

	readDone := make(chan error, 1)
	go func() {
		_, err := r.Read(make([]byte, 10))
		readDone <- err
	}()

	time.Sleep(20 * time.Millisecond) // let the read enter the poll loop
	require.NoError(t, r.Close())

	select {
	case err := <-readDone:
		require.ErrorIs(t, err, os.ErrClosed)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Close did not unblock the pending Read")
	}
}

func TestCloseReturnsErrorOnDoubleClose(t *testing.T) {
	t.Parallel()

	notifier := make(chan struct{}, 1)

	_, rfp := newTestTempFile(t)
	r := newTemporaryFileReaderWithPoll(rfp, notifier)
	r.markCompleted(nil)

	require.NoError(t, r.Close())
	require.ErrorIs(t, r.Close(), ErrAlreadyClosed)
}

func TestReadAfterCloseFails(t *testing.T) {
	t.Parallel()

	notifier := make(chan struct{}, 1)

	_, rfp := newTestTempFile(t)
	r := newTemporaryFileReaderWithPoll(rfp, notifier)
	r.markCompleted(nil)
	require.NoError(t, r.Close())

	_, err := r.Read(make([]byte, 10))
	require.ErrorIs(t, err, os.ErrClosed)
}
