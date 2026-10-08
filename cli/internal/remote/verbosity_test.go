package remote

import (
	"bytes"
	"strings"
	"testing"
)

// runOptionPush drives one push conversation with the given option lines and
// returns the protocol output and the stderr progress log. Messages are
// bilingual, so the tests count "igit: " lines and match on locale-neutral
// arguments (ref names, CIDs) instead of English text.
func runOptionPush(t *testing.T, options string) (string, string) {
	t.Helper()
	c, i, r := &fakeChain{}, &fakeIPFS{}, &fakeAuthorizer{}
	var out, logBuf bytes.Buffer
	input := "capabilities\n" + options + "list for-push\npush refs/heads/main:refs/heads/main\n\n"
	h := NewHelper(RepoURL{Owner: "inj1owner", Repo: "repo"}, c, i, r,
		[]string{"/dns4/us.example/tcp/4001/p2p/peer"}, fakeGit{},
		strings.NewReader(input), &out, &logBuf)
	if err := h.Run(); err != nil {
		t.Fatalf("push conversation: %v", err)
	}
	if !strings.Contains(out.String(), "ok refs/heads/main") {
		t.Fatalf("protocol output = %q", out.String())
	}
	return out.String(), logBuf.String()
}

func progressLines(log string) int { return strings.Count(log, "igit: ") }

func TestOptionVerbosityQuietSilencesPushProgress(t *testing.T) {
	_, log := runOptionPush(t, "option progress false\noption verbosity 0\n")
	if got := progressLines(log); got != 0 {
		t.Fatalf("quiet push printed %d progress lines: %q", got, log)
	}
}

func TestOptionVerbosityDefaultPrintsMilestonesOnly(t *testing.T) {
	_, log := runOptionPush(t, "option progress true\noption verbosity 1\n")
	if got := progressLines(log); got != 4 {
		t.Fatalf("default push printed %d progress lines, want 4 (pack/upload/pin/broadcast): %q", got, log)
	}
	if !strings.Contains(log, "refs/heads/main") || !strings.Contains(log, "bafy-test") {
		t.Fatalf("milestone lines lost ref or cid: %q", log)
	}
}

func TestOptionVerbosityVerbosePrintsDetails(t *testing.T) {
	_, log := runOptionPush(t, "option verbosity 2\n")
	if got := progressLines(log); got != 8 {
		t.Fatalf("verbose push printed %d progress lines, want 8: %q", got, log)
	}
}

func TestUnknownOptionIsReportedUnsupported(t *testing.T) {
	out, _ := runOptionPush(t, "option serve\n")
	if !strings.Contains(out, "unsupported\n") {
		t.Fatalf("unknown option was not reported unsupported: %q", out)
	}
}

func TestEnvQuietBeatsGitVerboseOption(t *testing.T) {
	t.Setenv("IGIT_QUIET", "1")
	_, log := runOptionPush(t, "option verbosity 2\n")
	if got := progressLines(log); got != 0 {
		t.Fatalf("IGIT_QUIET push printed %d progress lines: %q", got, log)
	}
}

func TestEnvVerboseBeatsGitQuietOption(t *testing.T) {
	t.Setenv("IGIT_VERBOSE", "1")
	_, log := runOptionPush(t, "option verbosity 0\n")
	if got := progressLines(log); got != 8 {
		t.Fatalf("IGIT_VERBOSE push printed %d progress lines, want 8: %q", got, log)
	}
}
