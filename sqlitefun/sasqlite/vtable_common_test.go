package sasqlite

import (
	"testing"
	"time"

	sasqlitev1 "github.com/jrhy/sandbox/sqlitefun/sasqlite/proto/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestMergeRows_LastDeleteWins(t *testing.T) {
	tm := time.Now()
	res := MergeRows(nil,
		tm.Add(time.Duration(-1000*time.Second)),
		&sasqlitev1.Row{
			Deleted:            true,
			DeleteUpdateOffset: durationpb.New(time.Duration(1 * time.Hour)),
		},
		tm.Add(time.Duration(-2000*time.Second)),
		&sasqlitev1.Row{
			Deleted:            true,
			DeleteUpdateOffset: durationpb.New(time.Duration(2 * time.Hour)),
		},
		tm)
	assert.True(t, res.Deleted)
	assert.Equal(t,
		tm.Add(time.Duration(2*time.Hour)).Add(time.Duration(-2000*time.Second)),
		tm.Add(res.DeleteUpdateOffset.AsDuration()))
}

func TestMergeRows_LastWriteWins(t *testing.T) {
	tm := time.Now()
	res := MergeRows(nil,
		tm.Add(time.Duration(-1000*time.Second)),
		&sasqlitev1.Row{
			ColumnValues: map[string]*sasqlitev1.ColumnValue{
				"col": {
					Value:        toSQLiteValue("hi"),
					UpdateOffset: durationpb.New(time.Duration(time.Hour)),
				},
			},
		},
		tm.Add(time.Duration(-2000*time.Second)),
		&sasqlitev1.Row{
			ColumnValues: map[string]*sasqlitev1.ColumnValue{
				"col": {
					Value:        toSQLiteValue("there"),
					UpdateOffset: durationpb.New(time.Duration(2 * time.Hour)),
				},
			},
		},
		tm)
	require.Equal(t, 1, len(res.ColumnValues))
	require.Equal(t,
		toSQLiteValue("there"),
		res.ColumnValues["col"].Value)
	require.Equal(t,
		tm.Add(time.Duration(2*time.Hour)).Add(time.Duration(-2000*time.Second)),
		tm.Add(res.ColumnValues["col"].UpdateOffset.AsDuration()))
}

func TestToSQLiteValue(t *testing.T) {
	require.Equal(t,
		&sasqlitev1.SQLiteValue{Type: sasqlitev1.Type_TEXT, Text: "hi"},
		toSQLiteValue("hi"))
	require.Equal(t,
		&sasqlitev1.SQLiteValue{Type: sasqlitev1.Type_TEXT},
		toSQLiteValue(""))
	require.Equal(t,
		&sasqlitev1.SQLiteValue{Type: sasqlitev1.Type_BLOB, Blob: []byte{}},
		toSQLiteValue([]byte{}))
}

func TestMergeRows_UnifyColumns(t *testing.T) {
	tm := time.Now()
	res := MergeRows(nil,
		tm.Add(time.Duration(-1000*time.Second)),
		&sasqlitev1.Row{
			ColumnValues: map[string]*sasqlitev1.ColumnValue{
				"col0": {
					Value:        toSQLiteValue("hi"),
					UpdateOffset: durationpb.New(time.Duration(time.Hour)),
				},
			},
		},
		tm.Add(time.Duration(-2000*time.Second)),
		&sasqlitev1.Row{
			ColumnValues: map[string]*sasqlitev1.ColumnValue{
				"col1": {
					Value:        toSQLiteValue("there"),
					UpdateOffset: durationpb.New(time.Duration(2 * time.Hour)),
				},
			},
		},
		tm)
	require.Equal(t, 2, len(res.ColumnValues))
	require.Equal(t, toSQLiteValue("hi"),
		res.ColumnValues["col0"].Value)
	require.Equal(t, toSQLiteValue("there"),
		res.ColumnValues["col1"].Value)
	require.Equal(t,
		tm.Add(time.Duration(-1000*time.Second)).Add(time.Duration(time.Hour)),
		UpdateTime(tm, res.ColumnValues["col0"]),
	)
	require.Equal(t,
		tm.Add(time.Duration(-2000*time.Second)).Add(time.Duration(2*time.Hour)),
		UpdateTime(tm, res.ColumnValues["col1"]),
	)
}

func TestMergeRows_InsertAfterDelete(t *testing.T) {
	tm := time.Now()
	res := MergeRows(nil,
		tm.Add(time.Duration(-1000*time.Second)),
		&sasqlitev1.Row{
			Deleted:            true,
			DeleteUpdateOffset: durationpb.New(time.Duration(time.Hour)),
			ColumnValues: map[string]*sasqlitev1.ColumnValue{
				"getnulledonnextinsert": {
					Value:        toSQLiteValue("hi"),
					UpdateOffset: durationpb.New(time.Duration(time.Hour)),
				},
			},
		},
		tm.Add(time.Duration(-2000*time.Second)),
		&sasqlitev1.Row{
			DeleteUpdateOffset: durationpb.New(time.Duration(2 * time.Hour)),
			ColumnValues: map[string]*sasqlitev1.ColumnValue{
				"version2": {
					Value:        toSQLiteValue("there"),
					UpdateOffset: durationpb.New(time.Duration(2 * time.Hour)),
				},
			},
		},
		tm)
	require.Equal(t, 1, len(res.ColumnValues))
	require.Nil(t, res.ColumnValues["getnulledonnextinsert"])
	require.Equal(t, toSQLiteValue("there"),
		res.ColumnValues["version2"].Value)
	require.Equal(t,
		tm.Add(time.Duration(-2000*time.Second)).Add(time.Duration(2*time.Hour)),
		UpdateTime(tm, res.ColumnValues["version2"]),
	)
}
