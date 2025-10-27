package database_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/database"
	"github.com/benjaminschubert/locaccel/internal/database/internal/dbtestutils"
	"github.com/benjaminschubert/locaccel/internal/testutils"
	"github.com/benjaminschubert/locaccel/internal/units"
)

func TestRetrieveNotFound(t *testing.T) {
	t.Parallel()

	db, err := database.NewDatabase[dbtestutils.TestObj](t.TempDir(), testutils.TestLogger(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	entry, err := db.Get([]byte("nonexistent"))
	require.ErrorIs(t, err, database.ErrKeyNotFound)
	assert.Nil(t, entry)
}

func TestCanSaveAndRetrieveFromDatabase(t *testing.T) {
	t.Parallel()

	db, err := database.NewDatabase[dbtestutils.TestObj](t.TempDir(), testutils.TestLogger(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	err = db.New([]byte("key"), dbtestutils.TestObj{Value: "hello"})
	require.NoError(t, err)

	entry, err := db.Get([]byte("key"))
	require.NoError(t, err)

	assert.Equal(t, dbtestutils.TestObj{Value: "hello"}, entry.Value)
}

func TestRefusesToSaveIfEntryWasUpdated(t *testing.T) {
	t.Parallel()

	db, err := database.NewDatabase[dbtestutils.TestObj](t.TempDir(), testutils.TestLogger(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	err = db.New([]byte("key"), dbtestutils.TestObj{Value: "hello"})
	require.NoError(t, err)

	// First retrieve
	entry, err := db.Get([]byte("key"))
	require.NoError(t, err)
	entry.Value = dbtestutils.TestObj{}

	// Second retrieve and update
	entryUpdated, err := db.Get([]byte("key"))
	require.NoError(t, err)
	entryUpdated.Value = dbtestutils.TestObj{}
	err = db.Save([]byte("key"), entryUpdated)
	require.NoError(t, err)

	// First update and save
	err = db.Save([]byte("key"), entry)
	require.ErrorIs(t, err, database.ErrConflict)
}

func TestCanRetrieveStatistics(t *testing.T) {
	t.Parallel()

	db, err := database.NewDatabase[dbtestutils.TestObj](t.TempDir(), testutils.TestLogger(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	for _, value := range []string{"one", "two", "three", "four", "five"} {
		err = db.New([]byte(value), dbtestutils.TestObj{Value: value})
		require.NoError(t, err)
	}

	count, totalSize, err := db.GetStatistics()
	assert.Equal(t, int64(5), count)
	assert.Equal(t, units.Bytes{Bytes: 78}, totalSize)
	require.NoError(t, err)
}

func TestCanIterateOverEntries(t *testing.T) {
	t.Parallel()

	db, err := database.NewDatabase[dbtestutils.TestObj](t.TempDir(), testutils.TestLogger(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	for _, value := range []string{"one", "two", "three", "four", "five"} {
		err = db.New([]byte(value), dbtestutils.TestObj{Value: value})
		require.NoError(t, err)
	}

	collectedKeys := []string{}
	collectedValues := []dbtestutils.TestObj{}

	err = db.Iterate(
		t.Context(),
		func(key []byte, entry *database.Entry[dbtestutils.TestObj]) error {
			collectedKeys = append(collectedKeys, string(key))
			collectedValues = append(collectedValues, entry.Value)
			return nil
		},
		"test",
	)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"one", "two", "three", "four", "five"}, collectedKeys)
	assert.ElementsMatch(
		t,
		[]dbtestutils.TestObj{
			{Value: "one"},
			{Value: "two"},
			{Value: "three"},
			{Value: "four"},
			{Value: "five"},
		},
		collectedValues,
	)
}

func TestCanDeleteEntry(t *testing.T) {
	t.Parallel()

	db, err := database.NewDatabase[dbtestutils.TestObj](t.TempDir(), testutils.TestLogger(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	err = db.New([]byte("one"), dbtestutils.TestObj{Value: "one"})
	require.NoError(t, err)

	val, err := db.Get([]byte("one"))
	require.NoError(t, err)

	require.NoError(t, db.Delete([]byte("one"), val))
	// No error when not found
	require.NoError(t, db.Delete([]byte("one"), val))
}
