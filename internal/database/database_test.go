package database_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/database"
	"github.com/benjaminschubert/locaccel/internal/database/internal/dbtestutils"
	"github.com/benjaminschubert/locaccel/internal/testutils"
)

func TestRetrieveNotFound(t *testing.T) {
	t.Parallel()

	db, err := database.NewDatabase[dbtestutils.TestObj](t.TempDir(), testutils.TestLogger(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	entry, err := db.Get("nonexistent")
	require.ErrorIs(t, err, database.ErrKeyNotFound)
	assert.Nil(t, entry)
}

func TestCanSaveAndRetrieveFromDatabase(t *testing.T) {
	t.Parallel()

	db, err := database.NewDatabase[dbtestutils.TestObj](t.TempDir(), testutils.TestLogger(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	err = db.New("key", dbtestutils.TestObj{Value: "hello"})
	require.NoError(t, err)

	entry, err := db.Get("key")
	require.NoError(t, err)

	assert.Equal(t, dbtestutils.TestObj{Value: "hello"}, entry.Value)
}

func TestRefusesToSaveIfEntryWasUpdated(t *testing.T) {
	t.Parallel()

	db, err := database.NewDatabase[dbtestutils.TestObj](t.TempDir(), testutils.TestLogger(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	err = db.New("key", dbtestutils.TestObj{Value: "hello"})
	require.NoError(t, err)

	// First retrieve
	entry, err := db.Get("key")
	require.NoError(t, err)
	entry.Value = dbtestutils.TestObj{}

	// Second retrieve and update
	entryUpdated, err := db.Get("key")
	require.NoError(t, err)
	entryUpdated.Value = dbtestutils.TestObj{}
	err = db.Save("key", entryUpdated)
	require.NoError(t, err)

	// First update and save
	err = db.Save("key", entry)
	require.ErrorIs(t, err, database.ErrConflict)
}
