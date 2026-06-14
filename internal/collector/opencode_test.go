package collector

import (
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenCodeCollectorUsesSourceCost(t *testing.T) {
	appDB := tempDB(t)
	dir, err := os.MkdirTemp("", "agent-ledger-opencode-source-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() {
		removeCollectorTestDirWithRetry(t, dir)
	})

	sourcePath := filepath.Join(dir, "opencode.db")
	src, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := src.Close(); err != nil {
			t.Errorf("Close source DB: %v", err)
		}
	})

	_, err = src.Exec(`
		CREATE TABLE session (id TEXT PRIMARY KEY, directory TEXT);
		CREATE TABLE message (data TEXT, session_id TEXT, time_created INTEGER);
		INSERT INTO session (id, directory) VALUES ('sess-1', '/work/project');
	`)
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(map[string]interface{}{
		"role":       "assistant",
		"modelID":    "custom-opencode-model",
		"providerID": "custom",
		"cost":       0.1234,
		"tokens": map[string]interface{}{
			"input":  100,
			"output": 50,
			"cache": map[string]interface{}{
				"read":  20,
				"write": 5,
			},
		},
		"time": map[string]interface{}{
			"created": int64(1780736000000),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Exec(`INSERT INTO message (data, session_id, time_created) VALUES (?, 'sess-1', 1780736000000)`, string(data)); err != nil {
		t.Fatal(err)
	}

	c := NewOpenCodeCollector(appDB, []string{sourcePath})
	if err := c.Scan(); err != nil {
		t.Fatal(err)
	}

	details, err := appDB.GetSessionDetail("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(details) != 1 {
		t.Fatalf("expected one session detail row, got %d", len(details))
	}
	if math.Abs(details[0].CostUSD-0.1234) > 0.0000001 {
		t.Fatalf("expected source cost 0.1234, got %f", details[0].CostUSD)
	}
}

func TestOpenCodeCollectorDoesNotDuplicatePromptsOnIncrementalScan(t *testing.T) {
	appDB := tempDB(t)
	dir, err := os.MkdirTemp("", "agent-ledger-opencode-source-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() {
		removeCollectorTestDirWithRetry(t, dir)
	})

	sourcePath := filepath.Join(dir, "opencode.db")
	src, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := src.Close(); err != nil {
			t.Errorf("Close source DB: %v", err)
		}
	})

	if _, err := src.Exec(`
		CREATE TABLE session (id TEXT PRIMARY KEY, directory TEXT);
		CREATE TABLE message (data TEXT, session_id TEXT, time_created INTEGER);
		INSERT INTO session (id, directory) VALUES ('sess-1', '/work/project');
	`); err != nil {
		t.Fatal(err)
	}

	insertMsg := func(role string, created int64, input int64) {
		t.Helper()
		data := map[string]interface{}{
			"role": role,
			"time": map[string]interface{}{"created": created},
		}
		if role == "assistant" {
			data["modelID"] = "opencode-model"
			data["tokens"] = map[string]interface{}{"input": input, "output": int64(2)}
		}
		raw, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := src.Exec(`INSERT INTO message (data, session_id, time_created) VALUES (?, 'sess-1', ?)`, string(raw), created); err != nil {
			t.Fatal(err)
		}
	}

	insertMsg("user", 1000, 0)
	insertMsg("assistant", 1100, 10)

	c := NewOpenCodeCollector(appDB, []string{sourcePath})
	if err := c.Scan(); err != nil {
		t.Fatal(err)
	}

	insertMsg("user", 1200, 0)
	insertMsg("assistant", 1300, 20)
	if err := c.Scan(); err != nil {
		t.Fatal(err)
	}

	from := time.UnixMilli(0)
	to := time.UnixMilli(2000)
	sessions, err := appDB.GetSessions(from, to, "opencode", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(sessions))
	}
	if sessions[0].Prompts != 2 {
		t.Fatalf("expected 2 deduped prompts, got %d", sessions[0].Prompts)
	}
}
