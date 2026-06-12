package storage

// ControlIdempotencyOperationStats summarizes retry-safe control writes by operation.
type ControlIdempotencyOperationStats struct {
	Operation  string `json:"operation"`
	Keys       int    `json:"keys"`
	Replays    int    `json:"replays"`
	LastSeenAt string `json:"last_seen_at"`
}

// ControlIdempotencyStats is privacy-safe; it never exposes raw idempotency keys
// or request hashes.
type ControlIdempotencyStats struct {
	TotalKeys    int                                `json:"total_keys"`
	ReplayedKeys int                                `json:"replayed_keys"`
	ReplayCount  int                                `json:"replay_count"`
	LastSeenAt   string                             `json:"last_seen_at"`
	Operations   []ControlIdempotencyOperationStats `json:"operations"`
}

// GetControlIdempotencyStats returns metadata-only control write retry stats.
func (d *DB) GetControlIdempotencyStats() (*ControlIdempotencyStats, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	stats := &ControlIdempotencyStats{}
	if err := d.db.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN replay_count > 0 THEN 1 ELSE 0 END),0),
		COALESCE(SUM(replay_count),0),
		COALESCE(MAX(last_seen_at),'')
		FROM control_idempotency`).Scan(&stats.TotalKeys, &stats.ReplayedKeys, &stats.ReplayCount, &stats.LastSeenAt); err != nil {
		return nil, err
	}
	rows, err := d.db.Query(`SELECT operation, COUNT(*), COALESCE(SUM(replay_count),0), COALESCE(MAX(last_seen_at),'')
		FROM control_idempotency GROUP BY operation ORDER BY operation`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var row ControlIdempotencyOperationStats
		if err := rows.Scan(&row.Operation, &row.Keys, &row.Replays, &row.LastSeenAt); err != nil {
			return nil, err
		}
		stats.Operations = append(stats.Operations, row)
	}
	return stats, rows.Err()
}
