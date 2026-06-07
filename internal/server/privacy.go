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

func hashValue(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}
