package database

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/ristretto/v2/z"
	"github.com/rs/zerolog"
	"github.com/tinylib/msgp/msgp"

	"github.com/benjaminschubert/locaccel/internal/logging"
)

var (
	ErrKeyNotFound = badger.ErrKeyNotFound
	ErrNoRewrite   = badger.ErrNoRewrite
	ErrInvalidKey  = errors.New("invalid entry key")
	ErrConflict    = errors.New("trying to update an entry that got updated already")
)

type encodable interface {
	msgp.Marshaler
}

type Ptr[T encodable] interface {
	*T
	msgp.Unmarshaler
}

type Response struct {
	Headers      http.Header
	ResponseHash string
	StatusCode   int
}

type Entry[T any] struct {
	Value   T
	version uint64
}

type Database[T encodable, TPtr Ptr[T]] struct {
	db *badger.DB
}

func NewDatabase[T encodable, TPtr Ptr[T]](
	path string,
	logger *zerolog.Logger,
) (*Database[T, TPtr], error) {
	badgerDB, err := badger.Open(
		badger.DefaultOptions(path).WithLogger(logging.NewLoggerAdapter(logger)),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to open the database, it might be corrupted: %w", err)
	}

	return &Database[T, TPtr]{badgerDB}, nil
}

func (d *Database[T, TPtr]) Close() error {
	if err := d.db.Close(); err != nil {
		return fmt.Errorf("unable to close the database, it might be corrupted: %w", err)
	}
	return nil
}

func (d *Database[T, TPtr]) Get(key string) (*Entry[T], error) {
	var entry Entry[T]

	err := d.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			if errors.Is(err, ErrKeyNotFound) {
				return ErrKeyNotFound
			}
			return fmt.Errorf("unexpected error loading key: %w", err)
		}

		err = item.Value(func(val []byte) error {
			entry.version = item.Version()
			entry.Value, err = d.unmarshal(val)
			return err
		})
		if err != nil {
			return fmt.Errorf("unexpected error extracting value: %w", err)
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, ErrKeyNotFound) {
			return nil, ErrKeyNotFound
		}
		return nil, fmt.Errorf("unable to load key: %w", err)
	}

	return &entry, nil
}

func (d *Database[T, TPtr]) Save(key string, entry *Entry[T]) error {
	data, err := entry.Value.MarshalMsg(nil)
	if err != nil {
		return fmt.Errorf(
			"entry in the database is not of the correct format, this should not happen: %w",
			err,
		)
	}

	err = d.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			if !errors.Is(err, badger.ErrKeyNotFound) {
				return fmt.Errorf("unable to check for previous entry with same key: %w", err)
			}
		} else if item.Version() != entry.version {
			return ErrConflict
		}

		return txn.Set([]byte(key), data)
	})
	if err != nil {
		return fmt.Errorf("unable to save entry in database: %w", err)
	}
	return nil
}

func (d *Database[T, TPtr]) New(key string, value T) error {
	return d.Save(key, &Entry[T]{Value: value})
}

func (d *Database[T, TPtr]) Delete(key string, entry *Entry[T]) error {
	err := d.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return nil // all good, nothing to remove
			}
			return err
		}

		if item.Version() != entry.version {
			return ErrConflict
		}

		return txn.Delete([]byte(key))
	})
	if err != nil {
		return fmt.Errorf("unable to delete entry from database: %w", err)
	}
	return nil
}

func (d *Database[T, TPtr]) GetStatistics() (count, totalSize int64, err error) {
	err = d.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			count++
			totalSize += it.Item().EstimatedSize()
		}
		return nil
	})

	return count, totalSize, err
}

func (d *Database[T, TPtr]) Iterate(
	ctx context.Context,
	apply func(key string, value *Entry[T]) error,
	logId string,
) error {
	stream := d.db.NewStream()
	stream.LogPrefix = logId

	stream.Send = func(buf *z.Buffer) error {
		list, err := badger.BufferToKVList(buf)
		if err != nil {
			return err
		}

		for _, kv := range list.Kv {
			val, err := d.unmarshal(kv.Value)
			if err != nil {
				return err
			}
			err = apply(string(kv.Key), &Entry[T]{val, kv.Version})
			if err != nil {
				return err
			}
		}
		return nil
	}

	return stream.Orchestrate(ctx)
}

func (d *Database[T, TPtr]) unmarshal(val []byte) (T, error) {
	var value TPtr = new(T)
	if _, err := value.UnmarshalMsg(val); err != nil {
		return *value, fmt.Errorf(
			"entry in the database is not of the correct format, this should not happen: %w",
			err,
		)
	}

	return *value, nil
}

func (d *Database[T, TPtr]) RunGarbageCollector() error {
	return d.db.RunValueLogGC(0.8)
}
