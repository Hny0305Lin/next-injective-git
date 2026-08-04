package secrets

import "testing"

func TestMatchHighSignalCredentials(t *testing.T) {
	for _, line := range []string{
		"-----BEGIN OPENSSH PRIVATE KEY-----",
		"AWS_ACCESS_KEY_ID=AKIA1234567890ABCDEF",
		"github_pat_123456789012345678901234567890",
	} {
		if !Match(line) {
			t.Fatalf("Match(%q) = false", line)
		}
	}
	if Match("ordinary source code") {
		t.Fatal("ordinary source code should not match")
	}
}
