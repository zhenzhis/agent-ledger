package storage

import "time"

func utcTimestamp(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.UTC()
}

func utcRange(from, to time.Time) (time.Time, time.Time) {
	return utcTimestamp(from), utcTimestamp(to)
}
