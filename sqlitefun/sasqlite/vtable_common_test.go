package sasqlite

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeRows_LastDeleteWins(t *testing.T) {
	tm := time.Now()
	res := mergeRows(nil,
		Row{
			Deleted:          true,
			DeleteUpdateTime: tm.Add(time.Duration(1 * time.Hour)),
		},
		Row{
			Deleted:          true,
			DeleteUpdateTime: tm.Add(time.Duration(2 * time.Hour)),
		})
	assert.True(t, res.Deleted)
	assert.Equal(t, tm.Add(time.Duration(2*time.Hour)), res.DeleteUpdateTime)
}

func TestMergeRows_LastWriteWins(t *testing.T) {
	tm := time.Now()
	res := mergeRows(nil,
		Row{
			ColumnValues: map[string]ColumnValue{
				"col": {
					Value:      "hi",
					UpdateTime: tm.Add(time.Duration(time.Hour)),
				},
			},
		},
		Row{
			ColumnValues: map[string]ColumnValue{
				"col": {
					Value:      "there",
					UpdateTime: tm.Add(time.Duration(2 * time.Hour)),
				},
			},
		})
	require.Equal(t, 1, len(res.ColumnValues))
	require.Equal(t,
		"there",
		res.ColumnValues["col"].Value)
	require.Equal(t,
		tm.Add(time.Duration(2*time.Hour)),
		res.ColumnValues["col"].UpdateTime,
	)
}

func TestMergeRows_UnifyColumns(t *testing.T) {
	tm := time.Now()
	res := mergeRows(nil,
		Row{
			ColumnValues: map[string]ColumnValue{
				"col0": {
					Value:      "hi",
					UpdateTime: tm.Add(time.Duration(time.Hour)),
				},
			},
		},
		Row{
			ColumnValues: map[string]ColumnValue{
				"col1": {
					Value:      "there",
					UpdateTime: tm.Add(time.Duration(2 * time.Hour)),
				},
			},
		})
	require.Equal(t, 2, len(res.ColumnValues))
	require.Equal(t, "hi",
		res.ColumnValues["col0"].Value)
	require.Equal(t, "there",
		res.ColumnValues["col1"].Value)
	require.Equal(t,
		tm.Add(time.Duration(time.Hour)),
		res.ColumnValues["col0"].UpdateTime,
	)
	require.Equal(t,
		tm.Add(time.Duration(2*time.Hour)),
		res.ColumnValues["col1"].UpdateTime,
	)
}

func TestMergeRows_InsertAfterDelete(t *testing.T) {
	tm := time.Now()
	res := mergeRows(nil,
		Row{
			Deleted:          true,
			DeleteUpdateTime: tm.Add(time.Duration(time.Hour)),
			ColumnValues: map[string]ColumnValue{
				"getnulledonnextinsert": {
					Value:      "hi",
					UpdateTime: tm.Add(time.Duration(time.Hour)),
				},
			},
		},
		Row{
			DeleteUpdateTime: tm.Add(time.Duration(2 * time.Hour)),
			ColumnValues: map[string]ColumnValue{
				"version2": {
					Value:      "there",
					UpdateTime: tm.Add(time.Duration(2 * time.Hour)),
				},
			},
		})
	require.Equal(t, 2, len(res.ColumnValues))
	require.Nil(t, res.ColumnValues["getnulledonnextinsert"].Value)
	require.Equal(t, "there",
		res.ColumnValues["version2"].Value)
	require.Equal(t,
		tm.Add(time.Duration(2*time.Hour)),
		res.ColumnValues["version2"].UpdateTime,
	)
	require.Equal(t,
		tm.Add(time.Duration(2*time.Hour)),
		res.ColumnValues["version2"].UpdateTime,
	)
}
