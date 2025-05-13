package filecache

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/rs/zerolog"
	"github.com/zeebo/blake3"

	"github.com/benjaminschubert/locaccel/internal/teereader"
)

var (
	ErrInitialize = errors.New("unable to initialize cache")
	ErrCannotOpen = errors.New("unable to open cached file")
)

type FileCache struct {
	root   string
	tmpdir string
}

func NewFileCache(root string) (*FileCache, error) {
	tmpdir := path.Join(root, "_tmp")

	// Ensure the tempdir exists
	if err := os.MkdirAll(tmpdir, 0o750); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInitialize, err)
	}

	tmpdirFiles, err := os.ReadDir(tmpdir)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInitialize, err)
	}

	for _, file := range tmpdirFiles {
		err = os.RemoveAll(path.Join(tmpdir, file.Name()))
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInitialize, err)
		}
	}

	for i := range int64(16 * 16) {
		err = os.Mkdir(path.Join(root, fmt.Sprintf("%02x", i)), 0o750)
		if err != nil && !os.IsExist(err) {
			return nil, fmt.Errorf("%w: %w", ErrInitialize, err)
		}
	}

	return &FileCache{root, tmpdir}, nil
}

func (f *FileCache) SetupIngestion(
	src io.ReadCloser,
	onIngest func(hash string),
	logger *zerolog.Logger,
) io.ReadCloser {
	dest, err := os.CreateTemp(f.tmpdir, "ingest-XXX")
	if err != nil {
		logger.Error().Err(err).Msg("Unable to create temporary file")
		return src
	}

	hasher := blake3.New()

	return teereader.New(
		src,
		io.MultiWriter(dest, hasher),
		func(readErr, writeErr error) error {
			if readErr != nil || writeErr != nil {
				err := readErr
				reason := "Read Error"
				if err == nil {
					err = writeErr
					reason = "Write Error"
				}

				logger.Error().
					Str("reason", reason).
					Err(err).
					Msg("an error happened ingesting the file")
				return f.cleanup(src, dest, logger)
			}

			hash := hex.EncodeToString(hasher.Sum(nil))

			if err := os.Rename(dest.Name(), path.Join(f.root, hash[:2], hash[2:])); err != nil {
				logger.Error().Err(err).Msg("unable to rename file for ingestion")
				return f.cleanup(src, dest, logger)
			}

			if err := dest.Close(); err != nil {
				logger.Error().Err(err).Msg("Unable to close temporary file after ingestion")
				return src.Close()
			}

			onIngest(hash)
			return src.Close()
		},
	)
}

func (f *FileCache) cleanup(src io.ReadCloser, dest *os.File, logger *zerolog.Logger) error {
	if e := dest.Close(); e != nil {
		logger.Error().Err(e).Msg("error closing temporary file.")
	}
	if e := os.Remove(dest.Name()); e != nil {
		logger.Error().Err(e).Msg("error removing temporary file.")
	}

	return src.Close()
}

func (f *FileCache) Open(hash string, logger *zerolog.Logger) (io.ReadCloser, error) {
	fp, err := os.Open(path.Join(f.root, hash[:2], hash[2:]))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotOpen, err)
	}
	if err := os.Chtimes(fp.Name(), time.Time{}, time.Now()); err != nil {
		logger.Warn().Err(err).Msg("unable to update mtime for cached file")
	}
	return fp, nil
}
