package multistream_test

import (
	"io"
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/multistream"
)

func setup(t *testing.T) (*multistream.Publisher, *multistream.StreamingFileReader) {
	t.Helper()

	w, err := os.Create(path.Join(t.TempDir(), "test.txt"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, w.Close()) })

	pub := multistream.NewPublisher(w)
	reader, err := multistream.NewStreamingFileReader(w.Name(), pub)
	require.NoError(t, err)

	return pub, reader
}

func TestCanReadCompleteFile(t *testing.T) {
	t.Parallel()

	pub, reader := setup(t)

	n, err := pub.Write([]byte("1234567890"))
	require.NoError(t, err)
	require.Equal(t, 10, n)
	pub.Finish(nil)

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "1234567890", string(data))
}

func TestCanIssueMultipleWriteBeforeReads(t *testing.T) {
	t.Parallel()

	pub, reader := setup(t)

	n, err := pub.Write([]byte("12345"))
	require.NoError(t, err)
	require.Equal(t, 5, n)
	// And a second write
	n, err = pub.Write([]byte("67890"))
	require.NoError(t, err)
	require.Equal(t, 5, n)

	pub.Finish(nil)

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "1234567890", string(data))
}

func TestReadsBlockUntilDataArrives(t *testing.T) {
	t.Parallel()

	pub, reader := setup(t)

	buf := make([]byte, 4)
	errCh := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(reader, buf)
		errCh <- err
	}()

	require.Never(t, func() bool { // data not readable before the write
		select {
		case <-errCh:
			return true
		default:
			return false
		}
	}, 50*time.Millisecond, 10*time.Millisecond)

	_, err := pub.Write([]byte("1234567890"))
	require.NoError(t, err)
	require.NoError(t, <-errCh)
	require.Equal(t, "1234", string(buf))

	pub.Finish(nil)
}

func TestNoReaderDoesNotBlockThePublisher(t *testing.T) {
	t.Parallel()

	pub, reader := setup(t)

	require.NoError(t, reader.Close())

	n, err := pub.Write([]byte("1234"))
	require.NoError(t, err)
	require.Equal(t, 4, n)

	pub.Finish(nil)
}

func TestForwardsErrorFromWriter(t *testing.T) {
	t.Parallel()

	pub, reader := setup(t)

	_, err := pub.Write([]byte("1234"))
	require.NoError(t, err)

	pub.Finish(os.ErrInvalid)

	data, err := io.ReadAll(reader)
	require.ErrorIs(t, err, os.ErrInvalid)
	require.Equal(t, "1234", string(data))
}

func TestReportsClosedOnReadingAfterClose(t *testing.T) {
	t.Parallel()

	pub, reader := setup(t)
	t.Cleanup(func() { pub.Finish(nil) })

	require.NoError(t, reader.Close())
	require.ErrorIs(t, reader.Close(), os.ErrClosed)
}

func TestCanReadAfterClose(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w, err := os.Create(path.Join(dir, "test.txt")) //nolint:gosec
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, w.Close()) })

	pub := multistream.NewPublisher(w)
	_, err = pub.Write([]byte("1234"))
	require.NoError(t, err)
	pub.Finish(nil)

	reader, err := multistream.NewStreamingFileReader(w.Name(), pub)
	require.NoError(t, err)

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "1234", string(data))

	require.NoError(t, reader.Close())
}

func TestFinishingTwiceIsIgnored(t *testing.T) {
	t.Parallel()

	pub, reader := setup(t)
	pub.Finish(os.ErrInvalid)
	pub.Finish(nil)

	_, err := io.ReadAll(reader)
	require.ErrorIs(t, err, os.ErrInvalid)
}

func TestCanCloneReaderMidFlight(t *testing.T) {
	t.Parallel()

	pub, reader := setup(t)

	_, err := pub.Write([]byte("1234"))
	require.NoError(t, err)

	buf := make([]byte, 10)
	n, err := reader.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, "1234\x00\x00\x00\x00\x00\x00", string(buf))

	otherReader, err := reader.Clone()
	require.NoError(t, err)

	_, err = pub.Write([]byte("567890"))
	require.NoError(t, err)

	pub.Finish(nil)

	data, err := io.ReadAll(otherReader)
	require.NoError(t, err)
	require.Equal(t, "1234567890", string(data))

	require.NoError(t, otherReader.Close())

	data, err = io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "567890", string(data))
	require.NoError(t, reader.Close())
}

func TestCanCloneAfterConsumptionIfFileExists(t *testing.T) {
	t.Parallel()

	pub, reader := setup(t)

	_, err := pub.Write([]byte("1234"))
	require.NoError(t, err)
	pub.Finish(nil)
	require.NoError(t, reader.Close())

	otherReader, err := reader.Clone()
	require.NoError(t, err)

	data, err := io.ReadAll(otherReader)
	require.NoError(t, err)
	require.Equal(t, "1234", string(data))

	require.NoError(t, otherReader.Close())
}

func TestReturnsErrorOnCloningRemovedFile(t *testing.T) {
	t.Parallel()

	file := path.Join(t.TempDir(), "test.txt")
	w, err := os.Create(file) //nolint:gosec
	require.NoError(t, err)

	pub := multistream.NewPublisher(w)
	require.NoError(t, w.Close())
	require.NoError(t, os.Remove(file))

	reader, err := multistream.NewStreamingFileReader(w.Name(), pub)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Nil(t, reader)
}
