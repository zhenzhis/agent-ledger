package storage

import (
	"path/filepath"
	"strings"
)

// SetProjectOptions configures project display aliases and exclusion patterns.
func (d *DB) SetProjectOptions(aliases map[string]string, exclude []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.projectAliases = map[string]string{}
	for k, v := range aliases {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		d.projectAliases[normalizePathKey(k)] = strings.TrimSpace(v)
	}
	d.projectExclude = append([]string(nil), exclude...)
}

func (d *DB) normalizeProject(project, cwd string) string {
	project = strings.TrimSpace(project)
	cwd = strings.TrimSpace(cwd)
	if project == "." || project == string(filepath.Separator) {
		project = ""
	}
	for match, alias := range d.projectAliases {
		if pathMatches(match, cwd) || pathMatches(match, project) {
			return alias
		}
	}
	if project != "" {
		return project
	}
	if cwd == "" {
		return ""
	}
	base := filepath.Base(cwd)
	if base == "." || base == string(filepath.Separator) {
		return cwd
	}
	return base
}

func (d *DB) isExcluded(project, cwd string) bool {
	target := normalizePathKey(project + " " + cwd)
	for _, pattern := range d.projectExclude {
		pattern = normalizePathKey(pattern)
		if pattern != "" && strings.Contains(target, pattern) {
			return true
		}
	}
	return false
}

func normalizeBranch(branch string) string {
	branch = strings.TrimSpace(branch)
	switch strings.ToLower(branch) {
	case "", "none", "null":
		return "unknown"
	case "head", "detached":
		return "detached"
	default:
		return branch
	}
}

func normalizePathKey(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	return strings.ToLower(value)
}

func pathMatches(pattern, value string) bool {
	if pattern == "" || value == "" {
		return false
	}
	value = normalizePathKey(value)
	return value == pattern || strings.HasPrefix(value, pattern+"/") || strings.Contains(value, pattern)
}
