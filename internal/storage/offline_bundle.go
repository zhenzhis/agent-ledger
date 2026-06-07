package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// OfflineBundle is an air-gap friendly export for merging local Agent Ledger
// instances without cloud sync. It includes metadata-only canonical events plus
// summary snapshots for human review.
type OfflineBundle struct {
	SchemaVersion string                 `json:"schema_version"`
	Product       string                 `json:"product"`
	BundleID      string                 `json:"bundle_id"`
	GeneratedAt   string                 `json:"generated_at"`
	Window        map[string]string      `json:"window"`
	Filters       map[string]string      `json:"filters"`
	Privacy       string                 `json:"privacy"`
	Data          OfflineBundleData      `json:"data"`
	Integrity     OfflineBundleIntegrity `json:"integrity"`
}

// OfflineBundleData contains both ingestible events and display summaries.
type OfflineBundleData struct {
	CanonicalEvents []CanonicalEvent       `json:"canonical_events"`
	Stats           *DashboardStats        `json:"stats"`
	Workloads       []WorkloadSummary      `json:"workloads"`
	ModelCalls      []ModelCallRow         `json:"model_calls"`
	Daily           []TokenTimeSeriesPoint `json:"daily"`
	Quality         *DataQualityReport     `json:"quality,omitempty"`
}

// OfflineBundleIntegrity records payload hash and optional HMAC signature.
type OfflineBundleIntegrity struct {
	HashAlgorithm      string `json:"hash_algorithm"`
	PayloadSHA256      string `json:"payload_sha256"`
	SignatureAlgorithm string `json:"signature_algorithm,omitempty"`
	Signature          string `json:"signature,omitempty"`
	KeyID              string `json:"key_id,omitempty"`
}

// OfflineBundleImportResult describes an import and merge outcome.
type OfflineBundleImportResult struct {
	BundleID          string `json:"bundle_id"`
	EventsSeen        int    `json:"events_seen"`
	EventsInserted    int    `json:"events_inserted"`
	EventsDuplicate   int    `json:"events_duplicate"`
	PayloadSHA256     string `json:"payload_sha256"`
	SignatureVerified bool   `json:"signature_verified"`
}

// BuildOfflineBundle builds a privacy-safe offline bundle. signingKey is optional.
func (d *DB) BuildOfflineBundle(from, to time.Time, source, model, project, privacyLabel, signingKey, keyID string, limit int) (*OfflineBundle, []byte, error) {
	if limit <= 0 {
		limit = 10000
	}
	if limit > 50000 {
		limit = 50000
	}
	events, err := d.GetCanonicalEvents(from, to, source, model, project, limit)
	if err != nil {
		return nil, nil, err
	}
	stats, err := d.GetDashboardStatsFiltered(from, to, source, model, project)
	if err != nil {
		return nil, nil, err
	}
	workloads, err := d.GetWorkloadsPage(from, to, source, model, project, "", "", bundleMinInt(limit, 10000), 0)
	if err != nil {
		return nil, nil, err
	}
	modelCalls, err := d.GetModelCalls(from, to, source, model, project, bundleMinInt(limit, 5000))
	if err != nil {
		return nil, nil, err
	}
	daily, err := d.GetTokensOverTimeFiltered(from, to, "1d", source, model, project, 0)
	if err != nil {
		return nil, nil, err
	}
	quality, _ := d.GetDataQuality(24 * time.Hour)
	bundle := &OfflineBundle{
		SchemaVersion: "v1",
		Product:       "Agent Ledger",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Window:        map[string]string{"from": from.UTC().Format(time.RFC3339), "to": to.UTC().Format(time.RFC3339)},
		Filters:       map[string]string{"source": source, "model": model, "project": project},
		Privacy:       firstNonEmptyStorage(privacyLabel, "metadata-only"),
		Data: OfflineBundleData{
			CanonicalEvents: events,
			Stats:           stats,
			Workloads:       workloads.Rows,
			ModelCalls:      modelCalls,
			Daily:           daily,
			Quality:         quality,
		},
	}
	if shouldRedactOfflineBundle(privacyLabel) {
		redactOfflineBundle(bundle)
	}
	rawForHash, err := marshalBundleForIntegrity(bundle)
	if err != nil {
		return nil, nil, err
	}
	sum := sha256.Sum256(rawForHash)
	payloadSHA := hex.EncodeToString(sum[:])
	bundle.BundleID = "bundle_" + payloadSHA[:16]
	bundle.Integrity.HashAlgorithm = "sha256"
	bundle.Integrity.PayloadSHA256 = payloadSHA
	if strings.TrimSpace(signingKey) != "" {
		mac := hmac.New(sha256.New, []byte(signingKey))
		_, _ = mac.Write(rawForHash)
		bundle.Integrity.SignatureAlgorithm = "hmac-sha256"
		bundle.Integrity.Signature = hex.EncodeToString(mac.Sum(nil))
		bundle.Integrity.KeyID = keyID
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		return nil, nil, err
	}
	if err := d.RecordOfflineBundle(bundle.BundleID, raw, "json"); err != nil {
		return nil, nil, err
	}
	return bundle, raw, nil
}

// ImportOfflineBundle verifies a bundle and replays canonical events into this DB.
func (d *DB) ImportOfflineBundle(raw []byte, signingKey string, requireSignature bool) (*OfflineBundleImportResult, error) {
	var bundle OfflineBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return nil, err
	}
	verified, payloadSHA, err := VerifyOfflineBundle(&bundle, signingKey, requireSignature)
	if err != nil {
		return nil, err
	}
	result := &OfflineBundleImportResult{
		BundleID:          bundle.BundleID,
		EventsSeen:        len(bundle.Data.CanonicalEvents),
		PayloadSHA256:     payloadSHA,
		SignatureVerified: verified,
	}
	for _, event := range bundle.Data.CanonicalEvents {
		ingested, err := d.IngestCanonicalEvent(event)
		if err != nil {
			return nil, err
		}
		if ingested.Status == "duplicate" {
			result.EventsDuplicate++
		} else {
			result.EventsInserted++
		}
	}
	if err := d.RecordOfflineBundle(bundle.BundleID, raw, "json"); err != nil {
		return nil, err
	}
	return result, nil
}

// VerifyOfflineBundle validates payload hash and optional HMAC signature.
func VerifyOfflineBundle(bundle *OfflineBundle, signingKey string, requireSignature bool) (bool, string, error) {
	if bundle == nil {
		return false, "", fmt.Errorf("bundle is required")
	}
	if bundle.SchemaVersion != "v1" {
		return false, "", fmt.Errorf("unsupported bundle schema %q", bundle.SchemaVersion)
	}
	rawForHash, err := marshalBundleForIntegrity(bundle)
	if err != nil {
		return false, "", err
	}
	sum := sha256.Sum256(rawForHash)
	payloadSHA := hex.EncodeToString(sum[:])
	if !strings.EqualFold(bundle.Integrity.PayloadSHA256, payloadSHA) {
		return false, payloadSHA, fmt.Errorf("bundle payload hash mismatch")
	}
	if bundle.Integrity.Signature == "" {
		if requireSignature {
			return false, payloadSHA, fmt.Errorf("bundle signature required")
		}
		return false, payloadSHA, nil
	}
	if strings.TrimSpace(signingKey) == "" {
		if requireSignature {
			return false, payloadSHA, fmt.Errorf("bundle signing key is required")
		}
		return false, payloadSHA, nil
	}
	if bundle.Integrity.SignatureAlgorithm != "hmac-sha256" {
		return false, payloadSHA, fmt.Errorf("unsupported signature algorithm %q", bundle.Integrity.SignatureAlgorithm)
	}
	mac := hmac.New(sha256.New, []byte(signingKey))
	_, _ = mac.Write(rawForHash)
	expected := mac.Sum(nil)
	actual, err := hex.DecodeString(bundle.Integrity.Signature)
	if err != nil {
		return false, payloadSHA, fmt.Errorf("invalid bundle signature encoding")
	}
	if !hmac.Equal(expected, actual) {
		return false, payloadSHA, fmt.Errorf("bundle signature mismatch")
	}
	return true, payloadSHA, nil
}

// GetCanonicalEvents returns metadata-only canonical events for offline export.
func (d *DB) GetCanonicalEvents(from, to time.Time, source, model, project string, limit int) ([]CanonicalEvent, error) {
	if limit <= 0 {
		limit = 10000
	}
	if limit > 50000 {
		limit = 50000
	}
	filter, args := buildCanonicalEventFilter(source, model, project)
	queryArgs := append([]interface{}{from, to}, args...)
	queryArgs = append(queryArgs, limit)
	rows, err := d.db.Query(`SELECT event_id,source,event_type,source_event_id,workload_id,agent_run_id,session_id,model,project,git_branch,timestamp,payload_hash,payload,confidence
		FROM canonical_events WHERE timestamp >= ? AND timestamp < ?`+filter+` ORDER BY timestamp,id LIMIT ?`, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CanonicalEvent
	for rows.Next() {
		var event CanonicalEvent
		var timestampRaw, payloadRaw string
		if err := rows.Scan(&event.EventID, &event.Source, &event.EventType, &event.SourceEventID, &event.WorkloadID, &event.AgentRunID,
			&event.SessionID, &event.Model, &event.Project, &event.GitBranch, &timestampRaw, &event.PayloadHash, &payloadRaw, &event.Confidence); err != nil {
			return nil, err
		}
		if t, ok := parseDBTime(timestampRaw); ok {
			event.Timestamp = t.UTC()
		}
		if payloadRaw != "" {
			event.Payload = json.RawMessage(payloadRaw)
		} else {
			event.Payload = json.RawMessage(`{}`)
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func buildCanonicalEventFilter(source, model, project string) (string, []interface{}) {
	var clauses []string
	var args []interface{}
	if source != "" {
		clauses = append(clauses, "source = ?")
		args = append(args, source)
	}
	if model != "" {
		clauses = append(clauses, "model = ?")
		args = append(args, model)
	}
	if project != "" {
		clauses = append(clauses, "project LIKE ?")
		args = append(args, "%"+project+"%")
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

func marshalBundleForIntegrity(bundle *OfflineBundle) ([]byte, error) {
	copy := *bundle
	copy.BundleID = ""
	copy.Integrity = OfflineBundleIntegrity{}
	return json.Marshal(copy)
}

func shouldRedactOfflineBundle(privacyLabel string) bool {
	switch strings.ToLower(strings.TrimSpace(privacyLabel)) {
	case "privacy", "private", "redacted", "strict", "screenshot", "team-share":
		return true
	default:
		return false
	}
}

func redactOfflineBundle(bundle *OfflineBundle) {
	if bundle == nil {
		return
	}
	for i := range bundle.Data.Workloads {
		bundle.Data.Workloads[i].Goal = "<redacted>"
		bundle.Data.Workloads[i].Project = "<redacted>"
		bundle.Data.Workloads[i].Repo = "<redacted>"
		bundle.Data.Workloads[i].GitBranch = "<redacted>"
		bundle.Data.Workloads[i].Owner = "<redacted>"
		bundle.Data.Workloads[i].Team = "<redacted>"
	}
	for i := range bundle.Data.ModelCalls {
		bundle.Data.ModelCalls[i].Project = "<redacted>"
	}
	for i := range bundle.Data.CanonicalEvents {
		event := &bundle.Data.CanonicalEvents[i]
		event.SessionID = hashPayload(event.SessionID)
		event.Project = "<redacted>"
		event.GitBranch = "<redacted>"
		var payload map[string]interface{}
		if len(event.Payload) > 0 && json.Unmarshal(event.Payload, &payload) == nil {
			for _, key := range []string{"goal", "project", "repo", "git_branch", "owner", "team", "cwd", "command", "label"} {
				if _, ok := payload[key]; ok {
					payload[key] = "<redacted>"
				}
			}
			raw, _ := json.Marshal(payload)
			event.Payload = raw
			event.PayloadHash = hashPayload(string(raw))
		}
	}
}

func bundleMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
