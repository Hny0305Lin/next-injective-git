package secrets

import "regexp"

var patterns = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN (?:RSA|OPENSSH|EC|DSA) PRIVATE KEY-----`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`(?i)github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`(?i)gh[pousr]_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`(?i)xox[baprs]-[A-Za-z0-9-]{10,}`),
}

// Match reports whether a line contains a high-signal credential pattern.
func Match(line string) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(line) {
			return true
		}
	}
	return false
}
