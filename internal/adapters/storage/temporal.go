package storage

import (
	"database/sql"
	"time"
)

func unixTimeOrNil(value *time.Time) any {
	if value == nil {
		return nil
	}
	return float64(value.Unix())
}

func nullableUnixTime(value sql.NullFloat64) *time.Time {
	if !value.Valid {
		return nil
	}
	t := time.Unix(int64(value.Float64), 0)
	return &t
}
