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

	"github.com/benjaminschubert/locaccel/internal/multistream"
	"github.com/benjaminschubert/locaccel/internal/units"
)

var (
	ErrInitialize          = errors.New("unable to initialize cache")
	ErrCannotOpen          = errors.New("unable to open cached file")
	ErrGCleanupNotRequired = errors.New("no need to remove old entries")
	ErrAlreadyClosed       = errors.New("the file was already closed")
	hashPool               = sync.Pool{
		New: func() any {
			return blake3.New()
		},
	}
	bytesPool = sync.Pool{
		New: func() any {
			buffer := make([]byte, 1024*128)
			return &buffer
		},
	}
)

const tmpDirName = "_tmp"

type FileCache struct {
	root           string
	tmpdir         string
	quotaLow       int64
	quotaHigh      int64
	logger         *zerolog.Logger
	workInFlightWg sync.WaitGroup
}

func NewFileCache(
	root string,
	quotaLow, quotaHigh int64,
	logger *zerolog.Logger,
) (*FileCache, error) {
	tmpdir := path.Join(root, tmpDirName)

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

	return &FileCache{root, tmpdir, quotaLow, quotaHigh, logger, sync.WaitGroup{}}, nil
}

func (f *FileCache) SetupIngestion(
	src io.ReadCloser,
	onIngest func(hash string),
	onCleanup func(),
	logger *zerolog.Logger,
) io.ReadCloser {
	f.workInFlightWg.Add(1)
	dest, err := os.CreateTemp(f.tmpdir, "ingest-XXX")
	if err != nil {
		logger.Error().Err(err).Msg("Unable to create temporary file")
		f.workInFlightWg.Done()
		return src
	}

	hasher := hashPool.Get().(*blake3.Hasher)
	hasher.Reset()

	buf := bytesPool.Get().(*[]byte)
	writer := multistream.NewPublisher(dest)
	stream, err := multistream.NewStreamingFileReader(dest.Name(), writer)
	if err != nil {
		f.workInFlightWg.Done()
		panic("Unable to open file a second time for reading, this should not happen")
	}

	go func() {
		defer f.workInFlightWg.Done()
		defer bytesPool.Put(buf)
		defer hashPool.Put(hasher)
		defer onCleanup()

		written, err := io.CopyBuffer(io.MultiWriter(writer, hasher), src, *buf)
		if err != nil {
			writer.Finish(err)
			logger.Error().Err(err).Msg("an error happened ingesting the file")
			f.cleanup(src, dest, logger)
			return
		}

		writer.Finish(nil)

		if written > (f.quotaHigh / 2) {
			logger.Warn().Int64("size", written).Msg("File is too big for the cache. Skipping")
			f.cleanup(src, dest, logger)
			return
		}

		hash := hex.EncodeToString(hasher.Sum(nil))
		if err := os.Rename(
			dest.Name(),
			path.Join(f.root, hash[:2], hash[2:]),
		); err != nil {
			logger.Error().Err(err).Msg("unable to rename file for ingestion")
			f.cleanup(src, dest, logger)
			return
		}

		if err := dest.Close(); err != nil {
			logger.Error().Err(err).Msg("Unable to close temporary file after ingestion")
			if err2 := src.Close(); err2 != nil {
				logger.Error().Err(err2).Msg("Unable to close upstream connection after ingestion")
			}
			return
		}

		onIngest(hash)
		if err := src.Close(); err != nil {
			logger.Error().Err(err).Msg("Unable to close upstream connection after ingestion")
		}
	}()

	return stream
}

func (f *FileCache) WaitUntilNoJobsInFlight() {
	f.workInFlightWg.Wait()
}

func (f *FileCache) cleanup(src io.ReadCloser, dest *os.File, logger *zerolog.Logger) {
	if e := dest.Close(); e != nil {
		logger.Error().Err(e).Msg("error closing temporary file.")
	}
	if e := os.Remove(dest.Name()); e != nil {
		logger.Error().Err(e).Msg("error removing temporary file.")
	}

	if err := src.Close(); err != nil {
		logger.Error().Err(err).Msg("Error cleaning up ingestion of file")
	}
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
		if dir.Name() == tmpDirName {
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

func (f *FileCache) Prune(
	logger *zerolog.Logger,
) (removed int64, allHashes map[string]struct{}, err error) {
	logger.Info().Msg("Pruning cache")

	totalSize, allHashes, files, err := f.getFilesAndTimestamps()
	if err != nil {
		return 0, nil, fmt.Errorf(
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
		return 0, allHashes, ErrGCleanupNotRequired
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

	removed = int64(0)

outer:
	for _, timestamp := range timestamps {
		for _, filename := range files[timestamp] {
			if totalSize <= f.quotaLow {
				break outer
			}

			fileInfo, err := os.Stat(filename)
			if err != nil {
				logger.Error().Err(err).Msg("An unexpected error happened trying to remove file")
				continue
			}
			if fileInfo.ModTime().UnixNano() != timestamp {
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
				hash := filename[len(filename)-65:len(filename)-63] + filename[len(filename)-62:]
				delete(allHashes, hash)
			}
		}
	}

	logger.Info().Int64("files", removed).Int64("diskUsage", totalSize).Msg("Removed files")
	return removed, allHashes, nil
}

func (f *FileCache) getFilesAndTimestamps() (totalSize int64, allHashes map[string]struct{}, timestampToFiles map[int64][]string, err error) {
	dirs, err := os.ReadDir(f.root)
	if err != nil {
		return 0, nil, nil, err
	}

	timestampToFiles = make(map[int64][]string, 1000)
	allHashes = make(map[string]struct{}, 1000)
	var fileInfo os.FileInfo

	for _, dir := range dirs {
		dirName := dir.Name()
		if dirName == tmpDirName {
			continue
		}

		fullPath := path.Join(f.root, dirName)

		files, err := os.ReadDir(fullPath)
		if err != nil {
			return 0, nil, nil, err
		}

		for _, fp := range files {
			fileInfo, err = fp.Info()
			if err != nil {
				return 0, nil, nil, err
			}

			timestamp := fileInfo.ModTime().UTC().UnixNano()

			timestampToFiles[timestamp] = append(
				timestampToFiles[timestamp],
				path.Join(fullPath, fp.Name()),
			)
			allHashes[dir.Name()+fp.Name()] = struct{}{}
			totalSize += fileInfo.Size()
		}
	}

	return totalSize, allHashes, timestampToFiles, nil
}

func (f *FileCache) GetAllHashes() ([]string, error) {
	dirs, err := os.ReadDir(f.root)
	if err != nil {
		return nil, err
	}

	hashes := make([]string, 0, 1000)

	for _, dir := range dirs {
		if dir.Name() == tmpDirName {
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

func (f *FileCache) Delete(hash string, logger *zerolog.Logger) error {
	return os.Remove(path.Join(f.root, hash[:2], hash[2:]))
}
