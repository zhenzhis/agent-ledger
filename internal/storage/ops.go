package storage

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
)

// ResetSource clears persisted scan state and usage for one source.
func (d *DB) ResetSource(source string, paths []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		"DELETE FROM usage_records WHERE source=?",
		"DELETE FROM prompt_events WHERE source=?",
		"DELETE FROM sessions WHERE source=?",
	} {
		if _, err := tx.Exec(q, source); err != nil {
			return err
		}
	}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, err := tx.Exec("DELETE FROM file_state WHERE path=? OR path LIKE ?", p, p+"%"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CountUsageRecords returns usage row count for a source, or all rows when source is empty.
func (d *DB) CountUsageRecords(source string) (int, error) {
	q := "SELECT COUNT(*) FROM usage_records"
	args := []interface{}{}
	if source != "" {
		q += " WHERE source=?"
		args = append(args, source)
	}
	var count int
	err := d.db.QueryRow(q, args...).Scan(&count)
	return count, err
}

// CountPromptEvents returns prompt event count for a source, or all rows when source is empty.
func (d *DB) CountPromptEvents(source string) (int, error) {
	q := "SELECT COUNT(*) FROM prompt_events"
	args := []interface{}{}
	if source != "" {
		q += " WHERE source=?"
		args = append(args, source)
	}
	var count int
	err := d.db.QueryRow(q, args...).Scan(&count)
	return count, err
}

// FileStateStats returns a best-effort file_state count and max offset for configured paths.
func (d *DB) FileStateStats(paths []string) (int, string, error) {
	var total int
	var watermark int64
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		rows, err := d.db.Query("SELECT last_offset FROM file_state WHERE path=? OR path LIKE ?", p, p+"%")
		if err != nil {
			return 0, "", err
		}
		for rows.Next() {
			var offset int64
			if err := rows.Scan(&offset); err != nil {
				rows.Close()
				return 0, "", err
			}
			total++
			if offset > watermark {
				watermark = offset
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return 0, "", err
		}
		rows.Close()
	}
	if watermark == 0 {
		return total, "", nil
	}
	return total, formatInt64(watermark), nil
}

// UpsertIngestionHealth stores the latest health snapshot for one source.
func (d *DB) UpsertIngestionHealth(h IngestionHealth) error {
	paths, err := json.Marshal(h.Paths)
	if err != nil {
		return err
	}
	pathStatus, err := json.Marshal(h.PathStatus)
	if err != nil {
		return err
	}
	_, err = d.db.Exec(`INSERT INTO ingestion_health(source,enabled,paths,path_status,last_scan_at,duration_ms,watermark,files_seen,records_inserted,prompts_inserted,skipped_rows,last_error)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(source) DO UPDATE SET
			enabled=excluded.enabled,
			paths=excluded.paths,
			path_status=excluded.path_status,
			last_scan_at=excluded.last_scan_at,
			duration_ms=excluded.duration_ms,
			watermark=excluded.watermark,
			files_seen=excluded.files_seen,
			records_inserted=excluded.records_inserted,
			prompts_inserted=excluded.prompts_inserted,
			skipped_rows=excluded.skipped_rows,
			last_error=excluded.last_error`,
		h.Source, boolInt(h.Enabled), string(paths), string(pathStatus), h.LastScanAt, h.DurationMS,
		h.Watermark, h.FilesSeen, h.RecordsInserted, h.PromptsInserted, h.SkippedRows, h.LastError)
	return err
}

// GetIngestionHealth returns latest scan health snapshots.
func (d *DB) GetIngestionHealth() ([]IngestionHealth, error) {
	rows, err := d.db.Query(`SELECT source,enabled,paths,path_status,COALESCE(last_scan_at,''),duration_ms,watermark,files_seen,records_inserted,prompts_inserted,skipped_rows,last_error
		FROM ingestion_health ORDER BY source`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []IngestionHealth
	for rows.Next() {
		var h IngestionHealth
		var enabled int
		var paths, pathStatus string
		if err := rows.Scan(&h.Source, &enabled, &paths, &pathStatus, &h.LastScanAt, &h.DurationMS,
			&h.Watermark, &h.FilesSeen, &h.RecordsInserted, &h.PromptsInserted, &h.SkippedRows, &h.LastError); err != nil {
			return nil, err
		}
		h.Enabled = enabled != 0
		_ = json.Unmarshal([]byte(paths), &h.Paths)
		_ = json.Unmarshal([]byte(pathStatus), &h.PathStatus)
		result = append(result, h)
	}
	return result, rows.Err()
}

// UpsertBudgetEvent stores the latest state for a budget rule and period.
func (d *DB) UpsertBudgetEvent(e BudgetEvent) error {
	_, err := d.db.Exec(`INSERT INTO budget_events(event_key,rule_name,period,scope,match,metric,value,limit_value,severity,message,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(event_key) DO UPDATE SET
			value=excluded.value,
			limit_value=excluded.limit_value,
			severity=excluded.severity,
			message=excluded.message,
			created_at=excluded.created_at`,
		e.EventKey, e.RuleName, e.Period, e.Scope, e.Match, e.Metric, e.Value, e.Limit, e.Severity, e.Message, e.CreatedAt)
	return err
}

// GetBudgetEvents returns recent budget events.
func (d *DB) GetBudgetEvents(limit int) ([]BudgetEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := d.db.Query(`SELECT event_key,rule_name,period,scope,match,metric,value,limit_value,severity,message,created_at
		FROM budget_events ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []BudgetEvent
	for rows.Next() {
		var e BudgetEvent
		if err := rows.Scan(&e.EventKey, &e.RuleName, &e.Period, &e.Scope, &e.Match, &e.Metric, &e.Value, &e.Limit, &e.Severity, &e.Message, &e.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	if err := rows.Err(); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return result, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func formatInt64(v int64) string {
	return strconv.FormatInt(v, 10)
}
