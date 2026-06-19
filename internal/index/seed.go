package index

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SeedQuery builds a retrieval seed from cheap, always-available session
// signals: the working directory's name, the git remote (if any), and the
// project's README/dir title. The result is a space-joined term list handed to
// FTS — the OR-of-terms retrieval (search.go) turns it into a recall query.
//
// extraTopics lets a caller (e.g. a future SessionStart hook) fold in last
// session's topics; it is optional and empty by default.
func SeedQuery(cwd string, extraTopics ...string) string {
	terms := make([]string, 0, 8)

	if base := filepath.Base(cwd); base != "" && base != "." && base != string(os.PathSeparator) {
		terms = append(terms, splitIdentifier(base)...)
	}
	if remote := gitRemoteName(cwd); remote != "" {
		terms = append(terms, splitIdentifier(remote)...)
	}
	if title := readmeTitle(cwd); title != "" {
		terms = append(terms, strings.Fields(title)...)
	}
	for _, t := range extraTopics {
		if t != "" {
			terms = append(terms, strings.Fields(t)...)
		}
	}
	return strings.Join(dedupeLower(terms), " ")
}

// gitRemoteName returns the last path segment of the origin remote URL (the repo
// name), or "" when cwd is not a git repo / git is unavailable.
func gitRemoteName(cwd string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", cwd, "config", "--get", "remote.origin.url")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(string(out))
	url = strings.TrimSuffix(url, ".git")
	if i := strings.LastIndexAny(url, "/:"); i >= 0 {
		url = url[i+1:]
	}
	return url
}

// readmeTitle returns the first markdown heading text from cwd/README.md.
func readmeTitle(cwd string) string {
	content, err := os.ReadFile(filepath.Join(cwd, "README.md"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(content), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") {
			return strings.TrimSpace(strings.TrimLeft(t, "# "))
		}
	}
	return ""
}

// splitIdentifier breaks a slug like "deer-print_v2" into ["deer","print","v2"].
func splitIdentifier(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == ' '
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) > 1 {
			out = append(out, f)
		}
	}
	return out
}

// dedupeLower lowercases terms and removes duplicates, preserving first order.
func dedupeLower(terms []string) []string {
	seen := make(map[string]bool, len(terms))
	out := make([]string, 0, len(terms))
	for _, t := range terms {
		lt := strings.ToLower(t)
		if lt != "" && !seen[lt] {
			seen[lt] = true
			out = append(out, lt)
		}
	}
	return out
}
