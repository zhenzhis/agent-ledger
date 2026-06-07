package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func (s *Server) privacyFor(r *http.Request) config.PrivacyConfig {
	p := s.options.Privacy
	if p.ScreenshotMode || r.URL.Query().Get("privacy") == "1" || r.URL.Query().Get("privacy") == "true" {
		p.RedactPaths = true
		p.HashSessionIDs = true
		p.HideProjectNames = true
	}
	return p
}

func applySessionPagePrivacy(page *storage.SessionPage, privacy config.PrivacyConfig) {
	if page == nil {
		return
	}
	for i := range page.Rows {
		applySessionPrivacy(&page.Rows[i], privacy)
	}
}

func applySessionPrivacy(session *storage.SessionInfo, privacy config.PrivacyConfig) {
	if privacy.HashSessionIDs {
		session.SessionID = hashValue(session.SessionID)
	}
	if privacy.RedactPaths {
		session.CWD = "<redacted>"
	}
	if privacy.HideProjectNames {
		session.Project = "<redacted>"
		session.GitBranch = "<redacted>"
	}
}

func applyChargebackPrivacy(rows []storage.ChargebackRow, privacy config.PrivacyConfig) {
	for i := range rows {
		if privacy.HideProjectNames || privacy.ScreenshotMode {
			rows[i].Project = "<redacted>"
		}
		if privacy.ScreenshotMode {
			rows[i].Team = "team"
		}
	}
}

func applyWrappedPrivacy(report *storage.WrappedReport, privacy config.PrivacyConfig) {
	if report == nil {
		return
	}
	if privacy.HashSessionIDs || privacy.ScreenshotMode {
		report.MostExpensiveSession.SessionID = hashValue(report.MostExpensiveSession.SessionID)
	}
	if privacy.HideProjectNames || privacy.ScreenshotMode {
		report.TopProject.Project = "<redacted>"
		report.MostExpensiveSession.Project = "<redacted>"
		report.MostExpensiveSession.GitBranch = "<redacted>"
		for i := range report.Highlights {
			if report.Highlights[i].Label == "top project" {
				report.Highlights[i].Value = "<redacted>"
			}
		}
	}
}

func applyDoctorPrivacy(report *storage.DoctorReport, privacy config.PrivacyConfig) {
	if report == nil {
		return
	}
	for i := range report.WorkloadStates {
		applyWorkloadStatePrivacy(&report.WorkloadStates[i], privacy)
	}
	if !(privacy.RedactPaths || privacy.ScreenshotMode) {
		return
	}
	for i := range report.Ingestion {
		for j := range report.Ingestion[i].Paths {
			report.Ingestion[i].Paths[j] = "<redacted>"
		}
		for j := range report.Ingestion[i].PathStatus {
			report.Ingestion[i].PathStatus[j].Path = "<redacted>"
		}
	}
	for i := range report.Checks {
		if report.Checks[i].Name == "path.missing" || report.Checks[i].Name == "path.unreadable" {
			report.Checks[i].Message = "<redacted path>"
		}
	}
}

func applyAuditEventPrivacy(rows []storage.AuditEvent, privacy config.PrivacyConfig) {
	if !(privacy.RedactPaths || privacy.HideProjectNames || privacy.HashSessionIDs || privacy.ScreenshotMode) {
		return
	}
	for i := range rows {
		if rows[i].Target != "" {
			rows[i].Target = hashValue(rows[i].Target)
		}
		if rows[i].Params != "" {
			rows[i].Params = "<redacted>"
		}
	}
}

func applyInsightEventPrivacy(rows []storage.InsightEvent, privacy config.PrivacyConfig) {
	if !(privacy.RedactPaths || privacy.HideProjectNames || privacy.HashSessionIDs || privacy.ScreenshotMode) {
		return
	}
	for i := range rows {
		if privacy.HashSessionIDs || privacy.ScreenshotMode {
			rows[i].SessionID = hashValue(rows[i].SessionID)
		}
		if privacy.HideProjectNames || privacy.RedactPaths || privacy.ScreenshotMode {
			rows[i].Project = "<redacted>"
		}
	}
}

func hashValue(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}
