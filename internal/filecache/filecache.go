package filecache

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/zeebo/blake3"

	"github.com/benjaminschubert/locaccel/internal/teereader"
	"github.com/benjaminschubert/locaccel/internal/units"
)

var (
	ErrInitialize          = errors.New("unable to initialize cache")
	ErrCannotOpen          = errors.New("unable to open cached file")
	ErrGCleanupNotRequired = errors.New("no need to remove old entries")
	hashPool               = sync.Pool{
		New: func() any {
			return blake3.New()
		},
	}
)

type FileCache struct {
	root      string
	tmpdir    string
	quotaLow  int64
	quotaHigh int64
	logger    *zerolog.Logger
}

func NewFileCache(
	root string,
	quotaLow, quotaHigh int64,
	logger *zerolog.Logger,
) (*FileCache, error) {
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

	return &FileCache{root, tmpdir, quotaLow, quotaHigh, logger}, nil
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

	hasher := hashPool.Get().(*blake3.Hasher)
	hasher.Reset()

	return teereader.New(
		src,
		io.MultiWriter(dest, hasher),
		func(totalread int, readErr, writeErr error) error {
			defer hashPool.Put(hasher)

			if totalread > (int(f.quotaHigh) / 2) {
				logger.Warn().Int("size", totalread).Msg("File is too big for the cache. Skipping")
				return f.cleanup(src, dest, logger)
			}

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

func (f *FileCache) Stat(hash string) (os.FileInfo, error) {
	return os.Stat(path.Join(f.root, hash[:2], hash[2:]))
}

func (f *FileCache) GetStatistics() (count int64, totalSize units.Bytes, err error) {
	dirs, err := os.ReadDir(f.root)
	if err != nil {
		return count, totalSize, err
	}

	var files []os.DirEntry
	var fileInfo os.FileInfo

	for _, dir := range dirs {
		if dir.Name() == "_tmp" {
			continue
		}

		files, err = os.ReadDir(path.Join(f.root, dir.Name()))
		if err != nil {
			return count, totalSize, err
		}

		count += int64(len(files))

		for _, fp := range files {
			fileInfo, err = fp.Info()
			if err != nil {
				return count, totalSize, err
			}

			totalSize.Bytes += fileInfo.Size()
		}
	}

	return count, totalSize, err
}

func (f *FileCache) Prune(logger *zerolog.Logger) (int64, error) {
	logger.Info().Msg("Pruning cache")

	totalSize, files, err := f.getFilesAndTimestamps()
	if err != nil {
		return 0, fmt.Errorf(
			"%s: %w",
			"unable to determine whether garbage collection needs to happen",
			err,
		)
	}

	if totalSize < f.quotaHigh {
		logger.Info().
			Int64("diskUsage", totalSize).
			Int64("maxQuota", f.quotaHigh).
			Msg("No need to evict files, under threshold")
		return 0, ErrGCleanupNotRequired
	}

	logger.Info().
		Int64("diskUsage", totalSize).
		Int64("maxQuota", f.quotaHigh).
		Msg("Disk usage above the required quota. Cleaning up")

	timestamps := make([]int64, 0, len(files))
	for t := range files {
		timestamps = append(timestamps, t)
	}
	slices.Sort(timestamps)

	removed := int64(0)

outer:
	for _, timestamp := range timestamps {
		for _, filename := range files[timestamp] {
			if totalSize <= f.quotaLow {
				break outer
			}

			fileInfo, err := os.Stat(filename)
			if err != nil {
				logger.Error().Err(err).Msg("An unexpected error happened trying to remove file")
			}
			if fileInfo.ModTime().Unix() != timestamp {
				logger.Debug().Str("filename", filename).Msg("file got it's timestamp updated since the check started, skipping")
				continue
			}

			size := fileInfo.Size()
			if err := os.Remove(filename); err != nil {
				logger.Error().Err(err).Msg("An unexpected error happened trying to remove file, skipping")
			} else {
				logger.Debug().Str("filename", filename).Int64("size", size).Msg("Removed file from cache")
				totalSize -= size
				removed += 1
			}
		}
	}

	logger.Info().Int64("files", removed).Int64("diskUsage", totalSize).Msg("Removed files")
	return removed, nil
}

func (f *FileCache) getFilesAndTimestamps() (totalSize int64, timestampToFiles map[int64][]string, err error) {
	dirs, err := os.ReadDir(f.root)
	if err != nil {
		return 0, nil, err
	}

	timestampToFiles = make(map[int64][]string, 0)
	var fileInfo os.FileInfo

	for _, dir := range dirs {
		dirName := dir.Name()
		if dirName == "_tmp" {
			continue
		}

		fullPath := path.Join(f.root, dirName)

		files, err := os.ReadDir(fullPath)
		if err != nil {
			return 0, nil, err
		}

		for _, fp := range files {
			fileInfo, err = fp.Info()
			if err != nil {
				return 0, nil, err
			}

			timestamp := fileInfo.ModTime().UTC().Unix()

			timestampToFiles[timestamp] = append(
				timestampToFiles[timestamp],
				path.Join(fullPath, fp.Name()),
			)
			totalSize += fileInfo.Size()
		}
	}

	return totalSize, timestampToFiles, nil
}

func (f *FileCache) GetAllHashes() ([]string, error) {
	dirs, err := os.ReadDir(f.root)
	if err != nil {
		return nil, err
	}

	hashes := make([]string, 0, 1000)

	for _, dir := range dirs {
		if dir.Name() == "_tmp" {
			continue
		}

		files, err := os.ReadDir(path.Join(f.root, dir.Name()))
		if err != nil {
			return nil, err
		}

		for _, fp := range files {
			hashes = append(hashes, dir.Name()+fp.Name())
		}
	}

	return hashes, nil
}
