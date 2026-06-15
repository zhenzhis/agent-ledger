package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	ContractName = "agent-ledger.ui-contract"
	Version      = "v1"
	staticDir    = "internal/server/static"
)

type ContractReport struct {
	Product        string          `json:"product"`
	Slug           string          `json:"slug"`
	Contract       string          `json:"contract"`
	Version        string          `json:"version"`
	GeneratedAt    string          `json:"generated_at"`
	OK             bool            `json:"ok"`
	Checked        int             `json:"checked"`
	Failed         int             `json:"failed"`
	ContractHash   string          `json:"contract_hash"`
	Files          []string        `json:"files"`
	DesignIntent   string          `json:"design_intent"`
	Checkpoints    []ContractCheck `json:"checkpoints"`
	Verification   []string        `json:"verification"`
	Privacy        string          `json:"privacy"`
	Recommendation string          `json:"recommendation,omitempty"`
}

type ContractCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
	Evidence string `json:"evidence"`
	Action   string `json:"action,omitempty"`
}

type authoredFile struct {
	rel  string
	body string
}

func Check(root string) (ContractReport, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	root = filepath.Clean(root)
	now := time.Now().UTC()
	report := ContractReport{
		Product:      "Agent Ledger",
		Slug:         "agent-ledger",
		Contract:     ContractName,
		Version:      Version,
		GeneratedAt:  now.Format(time.RFC3339Nano),
		DesignIntent: "embedded, no-framework, black/white/gray, data-dense Agent FinOps dashboard",
		Verification: []string{
			"agent-ledger ui check",
			"node --check internal/server/static/app.js",
			"browser smoke at 375/768/1024/1440px after layout changes",
		},
		Privacy: "UI contract reads only repository static files and reports file names, tokens, selectors, and counts; it does not inspect SQLite usage rows, local paths, sessions, prompts, responses, machine names, authors, secrets, or screenshots.",
	}

	files := []authoredFile{}
	for _, rel := range []string{
		filepath.Join(staticDir, "index.html"),
		filepath.Join(staticDir, "styles.css"),
		filepath.Join(staticDir, "app.js"),
	} {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			report.add("static.files.readable", false, "critical", rel, "restore the embedded dashboard static file")
			continue
		}
		report.Files = append(report.Files, filepath.ToSlash(rel))
		files = append(files, authoredFile{rel: filepath.ToSlash(rel), body: string(body)})
	}
	sort.Strings(report.Files)

	index := fileBody(files, "index.html")
	styles := fileBody(files, "styles.css")
	app := fileBody(files, "app.js")
	all := strings.Join([]string{index, styles, app}, "\n")

	report.add("static.files.readable", len(files) == 3, "critical", fmt.Sprintf("%d/3 authored static files loaded", len(files)), "restore missing embedded dashboard static files")
	report.add("html.viewport", hasViewport(index), "critical", "viewport meta keeps mobile scaling enabled", "add width=device-width, initial-scale=1.0 without disabling zoom")
	report.add("html.skip_link", strings.Contains(index, `class="skip-link"`) && strings.Contains(index, `id="main-content"`), "critical", "skip link targets #main-content", "restore keyboard skip link and main content target")
	report.add("assets.local_only", !hasRemoteAssetReference(index), "high", "index.html does not load remote scripts or styles", "keep dashboard assets embedded/local-first")
	report.add("buttons.labelled", allButtonsLabelled(index), "critical", "static buttons have visible text or aria-label", "add visible text or aria-label to every static button")
	report.add("charts.accessible", chartsAccessible(index), "critical", "chart containers expose role and aria-label", "add role and aria-label to every .chart container")
	report.add("privacy.entrypoints", strings.Contains(index, `id="btn-privacy"`) && strings.Contains(app, "privacyLabel") && strings.Contains(app, "state.privacy"), "critical", "privacy toggle and renderer redaction path are present", "restore privacy toggle and privacyLabel redaction path")
	report.add("integration_readiness.panel", strings.Contains(index, `id="integration-list"`) && strings.Contains(index, `id="s-integration"`) && strings.Contains(app, `integrations/readiness`) && strings.Contains(app, "renderIntegrationReadiness"), "high", "integration readiness panel is wired to the local readiness contract", "restore the readiness KPI, panel, API request, and renderer")
	report.add("palette.monochrome", authoredHexIsMonochrome(files), "high", "authored CSS/JS hex colors are grayscale", "replace decorative hue colors with black, white, gray, or semantic text labels")
	report.add("effects.no_decorative_gradients", decorativeGradientsOK(styles), "high", "only skeleton loading may use linear-gradient; no radial/orb/bokeh decoration", "remove decorative gradients, radial orbs, and bokeh effects")
	report.add("legacy.no_colored_kpi_accents", !containsAny(all, "accent-cyan", "accent-green", "accent-blue", "accent-amber"), "medium", "KPI cards do not depend on color-coded accent classes", "remove legacy colored KPI accent classes")
	report.add("responsive.breakpoints", responsiveBreakpoints(styles), "critical", "CSS includes mobile/tablet/desktop/wide breakpoints for 375/768/1024/1440px smoke", "add systematic breakpoint coverage for 375/768/1024/1440px")
	report.add("touch.targets", strings.Contains(styles, "min-height: 44px"), "critical", "mobile controls define 44px touch targets", "ensure primary controls are at least 44px tall on mobile")
	report.add("loading.skeleton_reduced_motion", strings.Contains(styles, "ledger-skeleton") && strings.Contains(styles, "prefers-reduced-motion"), "high", "loading skeleton respects reduced motion", "add skeleton loading and prefers-reduced-motion fallback")
	report.add("tables.contained_overflow", strings.Contains(styles, ".table-wrap") && strings.Contains(styles, "overflow-x: auto") && bodyOverflowHidden(styles), "high", "wide ledgers scroll inside their table wrapper", "contain table overflow instead of allowing page-level horizontal scroll")
	report.add("no_emoji_icons", !containsEmoji(all), "medium", "authored UI files do not use emoji as structural icons", "replace emoji icons with SVG or text labels")

	report.Checked = len(report.Checkpoints)
	for _, check := range report.Checkpoints {
		if check.Status != "pass" {
			report.Failed++
		}
	}
	report.OK = report.Failed == 0
	if !report.OK {
		report.Recommendation = "fix failed UI contract checkpoints before updating dashboard screenshots or claiming UI goal coverage"
	}
	report.ContractHash = Fingerprint(report)
	return report, nil
}

func Fingerprint(report ContractReport) string {
	report.GeneratedAt = ""
	report.ContractHash = ""
	raw, err := json.Marshal(report)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func FormatMarkdown(report ContractReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Agent Ledger UI Contract\n\n")
	fmt.Fprintf(&b, "- ok: `%t`\n", report.OK)
	fmt.Fprintf(&b, "- checked: `%d`\n", report.Checked)
	fmt.Fprintf(&b, "- failed: `%d`\n", report.Failed)
	fmt.Fprintf(&b, "- hash: `%s`\n", report.ContractHash)
	fmt.Fprintf(&b, "- intent: %s\n\n", report.DesignIntent)
	fmt.Fprintln(&b, "| check | status | severity | evidence |")
	fmt.Fprintln(&b, "|---|---|---|---|")
	for _, check := range report.Checkpoints {
		evidence := strings.ReplaceAll(check.Evidence, "|", "/")
		fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | %s |\n", check.Name, check.Status, check.Severity, evidence)
	}
	if report.Recommendation != "" {
		fmt.Fprintf(&b, "\nRecommendation: %s\n", report.Recommendation)
	}
	return b.String()
}

func (r *ContractReport) add(name string, pass bool, severity, evidence, action string) {
	status := "pass"
	if !pass {
		status = "fail"
	}
	check := ContractCheck{Name: name, Status: status, Severity: severity, Evidence: evidence}
	if !pass {
		check.Action = action
	}
	r.Checkpoints = append(r.Checkpoints, check)
}

func fileBody(files []authoredFile, suffix string) string {
	for _, file := range files {
		if strings.HasSuffix(filepath.ToSlash(file.rel), suffix) {
			return file.body
		}
	}
	return ""
}

func hasViewport(index string) bool {
	re := regexp.MustCompile(`(?is)<meta\s+[^>]*name=["']viewport["'][^>]*>`)
	match := re.FindString(index)
	return strings.Contains(match, "width=device-width") && strings.Contains(match, "initial-scale=1.0") && !strings.Contains(match, "user-scalable=no") && !strings.Contains(match, "maximum-scale=1")
}

func allButtonsLabelled(index string) bool {
	buttonRe := regexp.MustCompile(`(?is)<button\b([^>]*)>(.*?)</button>`)
	tagRe := regexp.MustCompile(`(?is)<[^>]+>`)
	for _, match := range buttonRe.FindAllStringSubmatch(index, -1) {
		attrs := match[1]
		body := strings.TrimSpace(tagRe.ReplaceAllString(match[2], ""))
		if strings.Contains(attrs, "aria-label=") || body != "" {
			continue
		}
		return false
	}
	return true
}

func hasRemoteAssetReference(index string) bool {
	re := regexp.MustCompile(`(?is)\b(?:src|href)=["']https?://`)
	return re.MatchString(index)
}

func chartsAccessible(index string) bool {
	divRe := regexp.MustCompile(`(?is)<div\b([^>]*)>`)
	matches := divRe.FindAllStringSubmatch(index, -1)
	found := 0
	for _, match := range matches {
		attrs := match[1]
		if !hasClass(attrs, "chart") {
			continue
		}
		found++
		if !(strings.Contains(attrs, `role="img"`) || strings.Contains(attrs, `role="region"`)) {
			return false
		}
		if !strings.Contains(attrs, "aria-label=") && !strings.Contains(attrs, "aria-labelledby=") {
			return false
		}
	}
	return found > 0
}

func hasClass(attrs, className string) bool {
	classRe := regexp.MustCompile(`(?is)\bclass=["']([^"']*)["']`)
	match := classRe.FindStringSubmatch(attrs)
	if len(match) < 2 {
		return false
	}
	for _, class := range strings.Fields(match[1]) {
		if class == className {
			return true
		}
	}
	return false
}

func authoredHexIsMonochrome(files []authoredFile) bool {
	hexRe := regexp.MustCompile(`#([0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})\b`)
	for _, file := range files {
		for _, match := range hexRe.FindAllStringSubmatch(file.body, -1) {
			if !isGrayHex(match[1]) {
				return false
			}
		}
	}
	return true
}

func isGrayHex(raw string) bool {
	if len(raw) == 3 {
		return raw[0] == raw[1] && raw[1] == raw[2]
	}
	if len(raw) == 8 {
		raw = raw[:6]
	}
	if len(raw) != 6 {
		return false
	}
	r, _ := strconv.ParseInt(raw[0:2], 16, 64)
	g, _ := strconv.ParseInt(raw[2:4], 16, 64)
	b, _ := strconv.ParseInt(raw[4:6], 16, 64)
	return r == g && g == b
}

func decorativeGradientsOK(styles string) bool {
	if containsAny(strings.ToLower(styles), "radial-gradient", "gradient-orb", "bokeh", " orb ") {
		return false
	}
	lower := strings.ToLower(styles)
	needle := "linear-gradient"
	start := 0
	for {
		idx := strings.Index(lower[start:], needle)
		if idx < 0 {
			return true
		}
		pos := start + idx
		contextStart := pos - 260
		if contextStart < 0 {
			contextStart = 0
		}
		contextEnd := pos + 260
		if contextEnd > len(lower) {
			contextEnd = len(lower)
		}
		context := lower[contextStart:contextEnd]
		if !strings.Contains(context, "loading-state") && !strings.Contains(context, "ledger-skeleton") {
			return false
		}
		start = pos + len(needle)
	}
}

func responsiveBreakpoints(styles string) bool {
	re := regexp.MustCompile(`@media\s*\(\s*(min|max)-width\s*:\s*([0-9]+)px\s*\)`)
	matches := re.FindAllStringSubmatch(styles, -1)
	mobile := false
	tablet := false
	desktop := false
	wide := false
	for _, match := range matches {
		kind := match[1]
		width, _ := strconv.Atoi(match[2])
		if kind == "max" && width <= 520 {
			mobile = true
		}
		if kind == "max" && width >= 760 && width <= 960 {
			tablet = true
		}
		if kind == "max" && width >= 1180 && width <= 1320 {
			desktop = true
		}
		if (kind == "min" && width >= 1440) || (kind == "max" && width >= 1360 && width <= 1440) {
			wide = true
		}
	}
	return mobile && tablet && desktop && wide
}

func bodyOverflowHidden(styles string) bool {
	re := regexp.MustCompile(`(?is)body\s*\{[^}]*overflow-x\s*:\s*hidden`)
	return re.MatchString(styles)
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func containsEmoji(s string) bool {
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 0 {
			return false
		}
		if (r >= 0x1F000 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) {
			return true
		}
		s = s[size:]
	}
	return false
}
