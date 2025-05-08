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

var ErrMaxSizedReached = errors.New("max sized reached")

type MaxSizeWriter struct{}

func (m MaxSizeWriter) Write(p []byte) (n int, err error) {
	return 0, ErrMaxSizedReached
}

func TestReadCopiesCorrectly(t *testing.T) {
	data := "hello world!"

	src := bytes.NewBufferString(data)
	writer := bytes.NewBufferString("")

	reader := teereader.New(src, writer, func(readErr, writeErr error) error {
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
	testErr := errors.New("Test error")
	src := iotest.ErrReader(testErr)
	writer := bytes.NewBufferString("")

	reader := teereader.New(src, writer, func(readErr, writeErr error) error {
		assert.NoError(t, writeErr)
		assert.ErrorIs(t, testErr, readErr)

		return nil
	})

	output, err := io.ReadAll(reader)
	require.ErrorIs(t, testErr, err)
	require.Equal(t, []byte{}, output)

	err = reader.Close()
	require.NoError(t, err)
}

func TestReaderHandlesPartialReads(t *testing.T) {
	data := "hello world!"

	src := iotest.HalfReader(bytes.NewBufferString(data))
	writer := bytes.NewBufferString("")

	reader := teereader.New(src, writer, func(readErr, writeErr error) error {
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
	data := "hello world!"

	src := bytes.NewBufferString(data)
	writer := MaxSizeWriter{}

	reader := teereader.New(src, writer, func(readErr, writeErr error) error {
		assert.NoError(t, readErr)
		assert.ErrorIs(t, writeErr, ErrMaxSizedReached)

		return nil
	})

	err := iotest.TestReader(reader, []byte(data))
	require.NoError(t, err)

	err = reader.Close()
	require.NoError(t, err)
}
