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

func TestOptionDryRunReportsSuccessWithoutSideEffects(t *testing.T) {
	c, i, r := &fakeChain{}, &fakeIPFS{}, &fakeAuthorizer{}
	var out, logBuf bytes.Buffer
	input := "capabilities\noption verbosity 1\noption dry-run true\nlist for-push\npush refs/heads/main:refs/heads/main\n\n"
	h := NewHelper(RepoURL{Owner: "inj1owner", Repo: "repo"}, c, i, r,
		[]string{"/dns4/us.example/tcp/4001/p2p/peer"}, fakeGit{},
		strings.NewReader(input), &out, &logBuf)
	if err := h.Run(); err != nil {
		t.Fatalf("dry-run push conversation: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "ok refs/heads/main") {
		t.Fatalf("dry-run protocol output = %q", got)
	}
	if c.updates != 0 || c.deletes != 0 || i.add != 0 || i.swarm != 0 || i.gc != 0 || r.authorized != 0 {
		t.Fatalf("dry-run had side effects: updates=%d deletes=%d add=%d swarm=%d gc=%d authorized=%d",
			c.updates, c.deletes, i.add, i.swarm, i.gc, r.authorized)
	}
	if got := progressLines(logBuf.String()); got != 0 {
		t.Fatalf("dry-run push printed %d progress lines: %q", got, logBuf.String())
	}
}

func TestOptionCloningIsAccepted(t *testing.T) {
	out, _ := runOptionPush(t, "option cloning true\noption verbosity 0\n")
	if strings.Contains(out, "unsupported") {
		t.Fatalf("option cloning must be accepted: %q", out)
	}
}

func TestDeferredStartupNoticesHonorVerbosity(t *testing.T) {
	run := func(options string) string {
		c, i, r := &fakeChain{}, &fakeIPFS{}, &fakeAuthorizer{}
		var out, logBuf bytes.Buffer
		h := NewHelper(RepoURL{Owner: "inj1owner", Repo: "repo"}, c, i, r,
			[]string{"/dns4/us.example/tcp/4001/p2p/peer"}, fakeGit{},
			strings.NewReader("capabilities\n"+options+"list for-push\npush refs/heads/main:refs/heads/main\n\n"), &out, &logBuf)
		h.DeferStep("alice -> inj1resolved")
		h.DeferDetail("gateway hk selected (12ms)")
		if err := h.Run(); err != nil {
			t.Fatalf("conversation: %v", err)
		}
		return logBuf.String()
	}
	// quiet: both startup notices are silenced
	if log := run("option verbosity 0\n"); strings.Contains(log, "alice") || strings.Contains(log, "gateway") {
		t.Fatalf("quiet startup notices leaked: %q", log)
	}
	// default: only the step-level notice appears
	log := run("option verbosity 1\n")
	if !strings.Contains(log, "alice -> inj1resolved") || strings.Contains(log, "gateway") {
		t.Fatalf("default startup notices = %q", log)
	}
	// verbose: both appear
	log = run("option verbosity 2\n")
	if !strings.Contains(log, "alice -> inj1resolved") || !strings.Contains(log, "gateway hk selected") {
		t.Fatalf("verbose startup notices = %q", log)
	}
}
