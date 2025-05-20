package teereader_test

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/teereader"
)

var (
	errMaxSizedReached = errors.New("max sized reached")
	errTest            = errors.New("test error")
)

type MaxSizeWriter struct{}

func (m MaxSizeWriter) Write(p []byte) (n int, err error) {
	return 0, errMaxSizedReached
}

func TestReadCopiesCorrectly(t *testing.T) {
	t.Parallel()

	data := "hello world!"

	src := bytes.NewBufferString(data)
	writer := bytes.NewBufferString("")

	reader := teereader.New(src, writer, func(size int, readErr, writeErr error) error {
		assert.Equal(t, len(data), size)
		assert.NoError(t, readErr)
		assert.NoError(t, writeErr)

		return nil
	})

	output, err := io.ReadAll(reader)
	require.NoError(t, err)

	assert.Equal(t, data, string(output))
	assert.Equal(t, data, writer.String())

	err = reader.Close()
	require.NoError(t, err)
}

func TestReadReportsErrors(t *testing.T) {
	t.Parallel()

	src := iotest.ErrReader(errTest)
	writer := bytes.NewBufferString("")

	reader := teereader.New(src, writer, func(_ int, readErr, writeErr error) error {
		require.NoError(t, writeErr)
		assert.ErrorIs(t, errTest, readErr)

		return nil
	})

	output, err := io.ReadAll(reader)
	require.ErrorIs(t, errTest, err)
	require.Equal(t, []byte{}, output)

	err = reader.Close()
	require.NoError(t, err)
}

func TestReaderHandlesPartialReads(t *testing.T) {
	t.Parallel()

	data := "hello world!"

	src := iotest.HalfReader(bytes.NewBufferString(data))
	writer := bytes.NewBufferString("")

	reader := teereader.New(src, writer, func(size int, readErr, writeErr error) error {
		assert.Equal(t, len(data), size)
		assert.NoError(t, readErr)
		assert.NoError(t, writeErr)

		return nil
	})

	err := iotest.TestReader(reader, []byte(data))
	require.NoError(t, err)

	err = reader.Close()
	require.NoError(t, err)
}

func TestReadsAllEvenOnWriteError(t *testing.T) {
	t.Parallel()

	data := "hello world!"

	src := bytes.NewBufferString(data)
	writer := MaxSizeWriter{}

	reader := teereader.New(src, writer, func(size int, readErr, writeErr error) error {
		require.NoError(t, readErr)
		assert.Equal(t, len(data), size)
		assert.ErrorIs(t, writeErr, errMaxSizedReached)

		return nil
	})

	err := iotest.TestReader(reader, []byte(data))
	require.NoError(t, err)

	err = reader.Close()
	require.NoError(t, err)
}
