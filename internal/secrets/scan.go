package secrets

import (
	"bufio"
	"bytes"
	"path"
	"regexp"
	"strings"
)

// Finding is a likely secret about to be published. Kind says what it looks
// like, as a code the UI turns into words.
type Finding struct {
	Path string `json:"path"`
	Line int    `json:"line"` // 0 when the whole file is sensitive
	Kind string `json:"kind"`
}

var secretPatterns = []struct {
	re   *regexp.Regexp
	kind string
}{
	{regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`), "private-key"},
	{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b|\bgithub_pat_[A-Za-z0-9_]{50,}\b`), "github-token"},
	{regexp.MustCompile(`\bglpat-[A-Za-z0-9_\-]{20,}\b`), "gitlab-token"},
	{regexp.MustCompile(`\b(AKIA|ASIA)[0-9A-Z]{16}\b`), "aws-key"},
	{regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`), "slack-token"},
	{regexp.MustCompile(`\b[rs]k_live_[0-9A-Za-z]{20,}\b`), "stripe-key"},
	{regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}\b`), "google-key"},
	{regexp.MustCompile(`\bsk-(ant-|proj-)[A-Za-z0-9_\-]{20,}\b`), "ai-key"},
	{regexp.MustCompile(`(?i)\b(password|passwd|secret|api[_-]?key|access[_-]?token)\s*[:=]\s*["']?([^\s"'#]{12,})`), "password"},
}

func sensitiveFile(p string) string {
	base := strings.ToLower(path.Base(p))
	switch {
	case base == ".env" || (strings.HasPrefix(base, ".env.") && !strings.Contains(base, "example") && !strings.Contains(base, "sample")):
		return "env-file"
	case base == ".netrc" || base == "_netrc":
		return "credentials-file"
	case strings.HasPrefix(base, "id_rsa") || strings.HasPrefix(base, "id_ed25519") || strings.HasPrefix(base, "id_ecdsa"):
		return "ssh-key"
	case strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") || strings.HasSuffix(base, ".p12") || strings.HasSuffix(base, ".pfx"):
		return "certificate"
	}
	return ""
}

// ScanFile checks a file that is about to be published for secrets.
// Binary content (images) is skipped.
func ScanFile(p string, content []byte) []Finding {
	if kind := sensitiveFile(p); kind != "" {
		return []Finding{{Path: p, Kind: kind}}
	}
	if bytes.IndexByte(content[:min(len(content), 8000)], 0) >= 0 {
		return nil // binary
	}
	var out []Finding
	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		for _, pat := range secretPatterns {
			if value := secretValue(pat.re, line); value != "" && !looksLikePlaceholder(value) {
				out = append(out, Finding{Path: p, Line: n, Kind: pat.kind})
				break
			}
		}
	}
	return out
}

// secretValue returns what a pattern matched on the line: its capture group
// if it has one, so a keyword like "password" isn't judged as part of the
// value. It is empty when nothing matched.
func secretValue(re *regexp.Regexp, line string) string {
	match := re.FindStringSubmatch(line)
	if match == nil {
		return ""
	}

	return match[len(match)-1]
}

func looksLikePlaceholder(value string) bool {
	l := strings.ToLower(value)
	for _, w := range []string{"example", "your_", "your-", "xxxx", "<", "changeme", "placeholder", "{{"} {
		if strings.Contains(l, w) {
			return true
		}
	}
	return false
}
