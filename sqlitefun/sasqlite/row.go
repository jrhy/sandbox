package sasqlite

import (
	"time"

	sasqlitev1 "github.com/jrhy/sandbox/sqlitefun/sasqlite/proto/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

func DeleteUpdateTime(baseTime time.Time, offset *durationpb.Duration) time.Time {
	return baseTime.Add(offset.AsDuration())
}
func UpdateTime(baseTime time.Time, cv *sasqlitev1.ColumnValue) time.Time {
	return baseTime.Add(cv.UpdateOffset.AsDuration())
}
