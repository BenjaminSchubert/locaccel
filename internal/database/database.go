package database

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog"
	"github.com/tinylib/msgp/msgp"

	"github.com/benjaminschubert/locaccel/internal/logging"
)

var (
	ErrKeyNotFound = badger.ErrKeyNotFound
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

// TODO: ensure we garbage collect: https://dgraph.io/docs/badger/get-started/#garbage-collection
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

		val, err := item.ValueCopy(nil)
		if err != nil {
			return fmt.Errorf("unexpected error extracting value: %w", err)
		}

		var value TPtr = new(T)
		_, err = value.UnmarshalMsg(val)
		if err != nil {
			return fmt.Errorf(
				"entry in the database is not of the correct format, this should not happen: %w",
				err,
			)
		}

		entry.Value = *value
		entry.version = item.Version()
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
