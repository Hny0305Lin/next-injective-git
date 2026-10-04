// Command igit-storage-canary is a minimal, reviewable BYOS canary entry for
// authorized small-object R2/S3 validation. It reuses the production adapters
// (storageconfig, s3store, packstore, packmanifest, gitio) and adds no new
// protocol logic. plan is local-only: no network, no credential resolution.
// execute requires -approve bound to the exact plan.json SHA-256. read is the
// independent-reader cold path. No delete, abort, policy or chain subcommand
// exists in this tool.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/gitio"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packstore/s3store"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/storageconfig"
	"io"
)

// placeholderBootstrap is a syntactically valid, never-fetched locator domain.
// Public bootstrap/anonymous read remains NOT PROVEN when this is used.
const placeholderBootstrap = "https://bootstrap-placeholder.example.com/"

const defaultRef = "refs/heads/main"

type planDoc struct {
	GeneratedAt   string `json:"generatedAt"`
	Provider      string `json:"provider"`
	Bucket        string `json:"bucket"`
	Region        string `json:"region"`
	AccountID     string `json:"accountId,omitempty"`
	Endpoint      string `json:"endpoint"`
	Prefix        string `json:"prefix"`
	WriterProfile string `json:"writerProfile"`
	ReaderProfile string `json:"readerProfile"`
	ReaderLabel   string `json:"readerLabel"`
	RepoGitDir    string `json:"repoGitDir"`
	Ref           string `json:"ref"`
	Tip           string `json:"tip"`
	TipTree       string `json:"tipTree"`
	SourceLog     []string
	ConfigSHA256  string `json:"configSha256"`
	PublicBase    string `json:"publicBase,omitempty"`
	ManifestCtx   packmanifest.Context
	PackObject    packstore.Object                `json:"packObject"`
	PackKey       string                          `json:"packKey"`
	PackRelPath   string                          `json:"packRelPath"`
	ManifestObj   packstore.Object                `json:"manifestObject"`
	ManifestKey   string                          `json:"manifestKey"`
	ManifestPath  string                          `json:"manifestRelPath"`
	Commitment    packmanifest.ManifestCommitment `json:"commitment"`
	EnvNames      struct {
		Writer []string `json:"writer"`
		Reader []string `json:"reader"`
	} `json:"envNames"`
	Budget struct {
		MaxUploadBytes int64 `json:"maxUploadBytes"`
		MaxReadBytes   int64 `json:"maxReadBytes"`
		MaxRequests    int   `json:"maxRequests"`
	} `json:"budget"`
	Notes []string `json:"notes"`
}

type commitmentDoc struct {
	PlanSHA256  string                          `json:"planSha256"`
	Provider    string                          `json:"provider"`
	Context     packmanifest.Context            `json:"context"`
	Commitment  packmanifest.ManifestCommitment `json:"commitment"`
	PackObject  packstore.Object                `json:"packObject"`
	Manifest    packstore.Object                `json:"manifestObject"`
	ReaderLabel string                          `json:"readerLabel"`
}

type readResult struct {
	At             string   `json:"at"`
	Phase          string   `json:"phase"`
	Manifest       string   `json:"manifest"`
	Pack           string   `json:"pack"`
	GitIndex       string   `json:"gitIndex"`
	TreeMatch      bool     `json:"treeMatch"`
	LogMatch       bool     `json:"logMatch"`
	PublicManifest string   `json:"publicManifest,omitempty"`
	PublicPack     string   `json:"publicPack,omitempty"`
	Notes          []string `json:"notes"`
}

// readersByLabel keeps the canary on the production Readers label dispatch
// instead of bypassing it with a direct store reference.
type readersByLabel struct{ r packstore.Readers }

func (a readersByLabel) Open(ctx context.Context, l packmanifest.PackLocation, o packstore.Object) (io.ReadCloser, error) {
	return a.r.Open(ctx, l, o)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "plan":
		err = cmdPlan(os.Args[2:])
	case "execute":
		err = cmdExecute(os.Args[2:])
	case "read":
		err = cmdRead(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "canary:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: igit-storage-canary <plan|execute|read> -config <storage.json> -rundir <dir> [-repo <git>] [-ref refs/heads/main] [-approve <plan-sha256>] [-timeout 10m]")
	os.Exit(2)
}

func mustTimeout(name string, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 || d > 30*time.Minute {
		d = 10 * time.Minute
	}
	return context.WithTimeout(context.Background(), d)
}

func gitRun(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func fileSHA256(path string) (string, int64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), int64(len(b)), nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// redact removes known secret values from any serialized output.
func redact(b []byte, secrets []string) []byte {
	for _, s := range secrets {
		if len(s) >= 8 {
			b = bytes.ReplaceAll(b, []byte(s), []byte("<redacted>"))
		}
	}
	return b
}

func secretValues(names []string) []string {
	var out []string
	for _, n := range names {
		if v, ok := os.LookupEnv(n); ok && v != "" {
			out = append(out, v)
		}
	}
	return out
}

func envNames(p storageconfig.Profile) []string {
	if p.CredentialRef == nil {
		return nil
	}
	names := []string{p.CredentialRef.AccessKeyEnv, p.CredentialRef.SecretKeyEnv}
	if p.CredentialRef.SessionTokenEnv != "" {
		names = append(names, p.CredentialRef.SessionTokenEnv)
	}
	return names
}

// loadBinding returns the single binding and both profiles; canary runs bind
// exactly one repository context per run directory.
func loadBinding(cfgPath string) (storageconfig.Config, storageconfig.Binding, storageconfig.Profile, storageconfig.Profile, error) {
	cfg, err := storageconfig.Load(cfgPath)
	if err != nil {
		return cfg, storageconfig.Binding{}, storageconfig.Profile{}, storageconfig.Profile{}, err
	}
	if len(cfg.Repositories) != 1 {
		return cfg, storageconfig.Binding{}, storageconfig.Profile{}, storageconfig.Profile{}, errors.New("canary requires exactly one repository binding")
	}
	b := cfg.Repositories[0]
	w, okw := cfg.Profiles[b.Writer]
	r, okr := cfg.Profiles[b.Reader]
	if !okw || !okr {
		return cfg, b, w, r, storageconfig.ErrConfig
	}
	return cfg, b, w, r, nil
}

func cmdPlan(args []string) error {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	cfgPath := fs.String("config", "", "storage config json")
	repo := fs.String("repo", "", "local git repository path")
	rundir := fs.String("rundir", "", "exclusive run directory")
	ref := fs.String("ref", defaultRef, "full ref name")
	timeout := fs.Duration("timeout", 5*time.Minute, "pack generation timeout")
	if err := fs.Parse(args); err != nil || *cfgPath == "" || *repo == "" || *rundir == "" {
		return errors.New("plan requires -config, -repo and -rundir")
	}
	ctx, cancel := mustTimeout("plan", *timeout)
	defer cancel()

	_, b, wp, rp, err := loadBinding(*cfgPath)
	if err != nil {
		return err
	}
	endpoint, err := wp.Endpoint()
	if err != nil {
		return err
	}
	cfgSHA, _, err := fileSHA256(*cfgPath)
	if err != nil {
		return err
	}

	abs := func(p string) string { a, _ := filepath.Abs(p); return a }
	gitdir, err := gitRun(abs(*repo), nil, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return err
	}
	tip, err := gitRun(*repo, nil, "rev-parse", "--verify", *ref+"^{commit}")
	if err != nil {
		return fmt.Errorf("ref %s not found: %w", *ref, err)
	}
	if len(tip) != 40 {
		return fmt.Errorf("unexpected tip: %q", tip)
	}

	for _, sub := range []string{"packs", "manifests", "journal", "tmp", "cold"} {
		if err := os.MkdirAll(filepath.Join(*rundir, sub), 0o755); err != nil {
			return err
		}
	}
	grepo := &gitio.Repo{GitDir: gitdir}
	pack, err := grepo.PackFullHistory(ctx, tip, filepath.Join(*rundir, "tmp"))
	if err != nil {
		return err
	}
	src := pack.Source()
	if wp.Provider == "cloudflare-r2" && src.Size > s3store.SinglePutLimit {
		pack.Close()
		return fmt.Errorf("R2 single PUT limit: pack %d > %d bytes", src.Size, s3store.SinglePutLimit)
	}
	packRel := filepath.Join("packs", "pack-"+src.SHA256+".pack")
	final := filepath.Join(*rundir, packRel)
	if err := os.Rename(src.Path, final); err != nil {
		pack.Close()
		return err
	}

	// Locations describe the same pack: anonymous public URL first when a
	// publicReadBase is configured, then the authenticated reader mapping.
	pubBase := strings.TrimRight(rp.PublicReadBase, "/")
	locations := []packmanifest.PackLocation{}
	packKey, err := packmanifest.Key(wp.Prefix, "packs", src.SHA256)
	if err != nil {
		return err
	}
	if pubBase != "" {
		locations = append(locations, packmanifest.PackLocation{Provider: wp.Provider, URL: pubBase + "/" + packKey})
	}
	label := "reader-" + rp.Provider
	if rp.CredentialRef != nil {
		locations = append(locations, packmanifest.PackLocation{Provider: wp.Provider, Reader: label})
	}
	if len(locations) == 0 {
		return errors.New("reader profile needs publicReadBase or a credential reference")
	}
	mctx := packmanifest.Context{ChainID: b.ChainID, SuiteDirectory: b.SuiteDirectory, RepoID: b.RepoID, RefName: *ref, Commit: packmanifest.Commit{Algorithm: "sha1", OID: tip}}
	manifest := packmanifest.PackManifest{
		Schema: "igit.pack-manifest", SchemaVersion: 1, Context: mctx,
		Packs: []packmanifest.PackEntry{{
			Sequence: 0, SHA256: src.SHA256, Size: strconv.FormatInt(src.Size, 10),
			Format: "git-pack", PackVersion: 2, Thin: false, DependsOn: []string{},
			Locations: locations,
		}},
	}
	mbytes, err := packmanifest.Encode(manifest)
	if err != nil {
		return err
	}
	mSHA := packmanifest.Digest(mbytes)
	manRel := filepath.Join("manifests", "manifest-"+mSHA+".json")
	if err := os.WriteFile(filepath.Join(*rundir, manRel), mbytes, 0o644); err != nil {
		return err
	}
	manKey, err := packmanifest.Key(wp.Prefix, "manifests", mSHA)
	if err != nil {
		return err
	}
	tree, err := gitRun(*repo, nil, "rev-parse", tip+"^{tree}")
	if err != nil {
		return err
	}
	log, err := gitRun(*repo, nil, "log", "--oneline", tip)
	if err != nil {
		return err
	}

	var doc planDoc
	doc.GeneratedAt = time.Now().Format(time.RFC3339)
	doc.Provider, doc.Bucket, doc.Region, doc.AccountID = wp.Provider, wp.Bucket, wp.Region, wp.AccountID
	doc.Endpoint, doc.Prefix = endpoint, wp.Prefix
	doc.WriterProfile, doc.ReaderProfile, doc.ReaderLabel = b.Writer, b.Reader, label
	doc.RepoGitDir, doc.Ref, doc.Tip, doc.TipTree = gitdir, *ref, tip, tree
	doc.SourceLog = strings.Split(log, "\n")
	doc.ConfigSHA256 = cfgSHA
	doc.ManifestCtx = mctx
	doc.PackObject = packstore.Object{Kind: "packs", SHA256: src.SHA256, Size: src.Size}
	doc.PackKey, doc.PackRelPath = packKey, packRel
	doc.ManifestObj = packstore.Object{Kind: "manifests", SHA256: mSHA, Size: int64(len(mbytes))}
	doc.ManifestKey, doc.ManifestPath = manKey, manRel
	doc.Commitment = packmanifest.ManifestCommitment{SHA256: mSHA, Size: strconv.Itoa(len(mbytes)), BootstrapLocator: placeholderBootstrap + manKey}
	if pubBase != "" {
		doc.Commitment.BootstrapLocator = pubBase + "/" + manKey
	}
	doc.PublicBase = pubBase
	doc.Notes = append(doc.Notes,
		"plan is local-only: no network and no credential resolution happened",
		"identity isolation NOT PROVEN unless reader credentials are a distinct cloud principal",
	)
	if pubBase != "" {
		doc.Notes = append(doc.Notes, "bootstrapLocator is the configured publicReadBase; anonymous public GET is expected to work")
	} else {
		doc.Notes = append(doc.Notes, "bootstrapLocator uses an example.com placeholder: public bootstrap/anonymous read NOT PROVEN")
	}
	doc.EnvNames.Writer, doc.EnvNames.Reader = envNames(wp), envNames(rp)
	doc.Budget.MaxUploadBytes = src.Size*2 + int64(len(mbytes))
	doc.Budget.MaxReadBytes = doc.Budget.MaxUploadBytes
	doc.Budget.MaxRequests = 20
	if err := writeJSON(filepath.Join(*rundir, "plan.json"), doc); err != nil {
		return err
	}
	planSHA, _, err := fileSHA256(filepath.Join(*rundir, "plan.json"))
	if err != nil {
		return err
	}
	fmt.Printf("plan-sha256: %s\npack: %s (%d bytes)\nmanifest: %s (%d bytes)\nprovider: %s bucket: %s prefix: %s\n",
		planSHA, src.SHA256, src.Size, mSHA, len(mbytes), wp.Provider, wp.Bucket, wp.Prefix)
	return nil
}

// verifyPlanBinding fails when the config or local evidence changed after plan.
func verifyPlanBinding(rundir, cfgPath, approve string) (planDoc, storageconfig.Profile, storageconfig.Profile, error) {
	var doc planDoc
	noProfile := storageconfig.Profile{}
	if approve == "" {
		return doc, noProfile, noProfile, errors.New("execute requires -approve <plan-sha256>; no implicit authorization")
	}
	planPath := filepath.Join(rundir, "plan.json")
	b, err := os.ReadFile(planPath)
	if err != nil {
		return doc, noProfile, noProfile, err
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != strings.ToLower(approve) {
		return doc, noProfile, noProfile, errors.New("approve hash does not match plan.json; regenerate the plan and re-confirm")
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return doc, noProfile, noProfile, err
	}
	cfgSHA, _, err := fileSHA256(cfgPath)
	if err != nil || cfgSHA != doc.ConfigSHA256 {
		return doc, noProfile, noProfile, errors.New("storage config changed since plan; regenerate the plan")
	}
	_, _, wp, rp, err := loadBinding(cfgPath)
	if err != nil {
		return doc, wp, rp, err
	}
	endpoint, _ := wp.Endpoint()
	if wp.Provider != doc.Provider || wp.Bucket != doc.Bucket || wp.Prefix != doc.Prefix || endpoint != doc.Endpoint {
		return doc, wp, rp, errors.New("writer profile no longer matches the approved plan")
	}
	return doc, wp, rp, nil
}

type uploadOutcome struct {
	Key      string `json:"key"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Reused   bool   `json:"reused"`
	Verified bool   `json:"verified"`
	Error    string `json:"error,omitempty"`
}

func cmdExecute(args []string) error {
	fs := flag.NewFlagSet("execute", flag.ContinueOnError)
	cfgPath := fs.String("config", "", "storage config json")
	rundir := fs.String("rundir", "", "exclusive run directory")
	approve := fs.String("approve", "", "approved plan.json sha256")
	journalDir := fs.String("journal", "", "OS-protectable journal directory (see DACL probe notes)")
	timeout := fs.Duration("timeout", 10*time.Minute, "overall execution timeout")
	if err := fs.Parse(args); err != nil || *cfgPath == "" || *rundir == "" {
		return errors.New("execute requires -config, -rundir and -approve")
	}
	if *journalDir == "" {
		*journalDir = filepath.Join(*rundir, "journal")
	}
	doc, wp, _, err := verifyPlanBinding(*rundir, *cfgPath, *approve)
	if err != nil {
		return err
	}
	for _, name := range doc.EnvNames.Writer {
		if v, ok := os.LookupEnv(name); !ok || v == "" {
			return fmt.Errorf("writer credential environment variable %s is not set in this process", name)
		}
	}
	ctx, cancel := mustTimeout("execute", *timeout)
	defer cancel()

	packSrcPath := filepath.Join(*rundir, doc.PackRelPath)
	manSrcPath := filepath.Join(*rundir, doc.ManifestPath)
	packF, err := packstore.CheckSource(ctx, packstore.Source{Path: packSrcPath, SHA256: doc.PackObject.SHA256, Size: doc.PackObject.Size}, doc.PackObject)
	if err != nil {
		return fmt.Errorf("local pack no longer matches the approved plan: %w", err)
	}
	packF.Close()
	manF, err := packstore.CheckSource(ctx, packstore.Source{Path: manSrcPath, SHA256: doc.ManifestObj.SHA256, Size: doc.ManifestObj.Size}, doc.ManifestObj)
	if err != nil {
		return fmt.Errorf("local manifest no longer matches the approved plan: %w", err)
	}
	manF.Close()

	w, err := s3store.NewWriter(wp)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*journalDir, 0o755); err != nil {
		return err
	}
	// A checkpoint failure aborts before any cloud IO (fail-closed). Non-secret
	// receipt copies are retained in the run directory as reviewable evidence.
	journal := func(r packstore.Receipt) error {
		return w.SaveReceipt(filepath.Join(*journalDir, "receipt-"+r.Object.Kind+".json"), r)
	}

	result := struct {
		At        string        `json:"at"`
		Phase     string        `json:"phase"`
		PlanSHA   string        `json:"planSha256"`
		Pack      uploadOutcome `json:"pack"`
		PackReuse uploadOutcome `json:"packIdempotentReuse"`
		Manifest  uploadOutcome `json:"manifest"`
		Notes     []string      `json:"notes"`
	}{At: time.Now().Format(time.RFC3339), Phase: "execute", PlanSHA: *approve}

	packSrc := packstore.Source{Path: packSrcPath, SHA256: doc.PackObject.SHA256, Size: doc.PackObject.Size}
	r, err := w.PutRecoverable(ctx, doc.PackObject, packSrc, journal)
	result.Pack = outcome(r, err)
	if err == nil {
		r2, err2 := w.PutIfAbsent(ctx, doc.PackObject, packSrc)
		result.PackReuse = outcome(r2, err2)
		if err2 == nil && !r2.Reused {
			result.PackReuse.Error = "expected idempotent reuse (412) but object was (re)written"
		}
	}
	manSrc := packstore.Source{Path: manSrcPath, SHA256: doc.ManifestObj.SHA256, Size: doc.ManifestObj.Size}
	rm, errm := w.PutRecoverable(ctx, doc.ManifestObj, manSrc, journal)
	result.Manifest = outcome(rm, errm)
	result.Notes = append(result.Notes,
		"single verified location per object; no delete/abort/policy operation exists in this tool",
		"identity isolation NOT PROVEN unless reader credentials are a distinct cloud principal",
		"OS-protected journal: "+*journalDir,
	)
	for _, kind := range []string{"packs", "manifests"} {
		src := filepath.Join(*journalDir, "receipt-"+kind+".json")
		if b, e := os.ReadFile(src); e == nil {
			_ = os.MkdirAll(filepath.Join(*rundir, "journal-evidence"), 0o755)
			_ = os.WriteFile(filepath.Join(*rundir, "journal-evidence", "receipt-"+kind+".json"), b, 0o644)
		}
	}

	cdoc := commitmentDoc{PlanSHA256: *approve, Provider: doc.Provider, Context: doc.ManifestCtx, Commitment: doc.Commitment, PackObject: doc.PackObject, Manifest: doc.ManifestObj, ReaderLabel: doc.ReaderLabel}
	if err := writeJSON(filepath.Join(*rundir, "commitment.json"), cdoc); err != nil {
		return err
	}
	out, _ := json.MarshalIndent(result, "", "  ")
	os.Stdout.Write(redact(out, secretValues(doc.EnvNames.Writer)))
	fmt.Println()
	if err != nil || errm != nil {
		return errors.New("one or more uploads failed; see results above and journal/ receipts")
	}
	return nil
}

func outcome(r packstore.Receipt, err error) uploadOutcome {
	o := uploadOutcome{Key: r.Key, SHA256: r.Object.SHA256, Size: r.Object.Size, Reused: r.Reused, Verified: r.Verified}
	if err != nil {
		o.Error = err.Error()
		var typed *packstore.Error
		if errors.As(err, &typed) && typed.Status != 0 {
			o.Error = fmt.Sprintf("%s (HTTP %d)", o.Error, typed.Status)
		}
	}
	return o
}

func cmdRead(args []string) error {
	fs := flag.NewFlagSet("read", flag.ContinueOnError)
	cfgPath := fs.String("config", "", "storage config json")
	rundir := fs.String("rundir", "", "exclusive run directory")
	origin := fs.String("origin", "https://www.igit.xyz", "https origin used for CORS diagnostics")
	timeout := fs.Duration("timeout", 10*time.Minute, "overall read timeout")
	if err := fs.Parse(args); err != nil || *cfgPath == "" || *rundir == "" {
		return errors.New("read requires -config and -rundir")
	}
	var cdoc commitmentDoc
	cb, err := os.ReadFile(filepath.Join(*rundir, "commitment.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(cb, &cdoc); err != nil {
		return err
	}
	planBytes, err := os.ReadFile(filepath.Join(*rundir, "plan.json"))
	if err != nil {
		return err
	}
	ph := sha256.Sum256(planBytes)
	if hex.EncodeToString(ph[:]) != cdoc.PlanSHA256 {
		return errors.New("plan.json no longer matches the executed commitment")
	}
	var doc planDoc
	if err := json.Unmarshal(planBytes, &doc); err != nil {
		return err
	}
	cfgSHA, _, err := fileSHA256(*cfgPath)
	if err != nil || cfgSHA != doc.ConfigSHA256 {
		return errors.New("storage config changed since plan")
	}
	_, _, _, rp, err := loadBinding(*cfgPath)
	if err != nil {
		return err
	}
	for _, name := range doc.EnvNames.Reader {
		if v, ok := os.LookupEnv(name); !ok || v == "" {
			return fmt.Errorf("reader credential environment variable %s is not set in this process", name)
		}
	}
	ctx, cancel := mustTimeout("read", *timeout)
	defer cancel()

	rd, err := s3store.NewReader(rp)
	if err != nil {
		return err
	}
	readers := packstore.Readers{Authenticated: map[string]packstore.Reader{doc.ReaderLabel: rd}}
	opener := readersByLabel{readers}

	result := readResult{At: time.Now().Format(time.RFC3339), Phase: "read"}

	mf, err := packstore.ReadVerified(ctx, opener, []packmanifest.PackLocation{{Provider: cdoc.Provider, Reader: cdoc.ReaderLabel}}, cdoc.Manifest, filepath.Join(*rundir, "tmp"))
	if err != nil {
		result.Manifest = "FAIL: " + err.Error()
		return finishRead(*rundir, &result, doc)
	}
	mb, err := os.ReadFile(mf.Source().Path)
	mf.Close()
	if err != nil {
		result.Manifest = "FAIL: local spool"
		return finishRead(*rundir, &result, doc)
	}
	if _, err := packmanifest.Parse(mb, cdoc.Context, cdoc.Commitment); err != nil {
		result.Manifest = "FAIL: parse/commitment mismatch"
		return finishRead(*rundir, &result, doc)
	}
	result.Manifest = "PASS"

	pf, err := packstore.ReadVerified(ctx, opener, []packmanifest.PackLocation{{Provider: cdoc.Provider, Reader: cdoc.ReaderLabel}}, cdoc.PackObject, filepath.Join(*rundir, "tmp"))
	if err != nil {
		result.Pack = "FAIL: " + err.Error()
		return finishRead(*rundir, &result, doc)
	}
	result.Pack = "PASS"
	// Absolute path: gitRun sets cmd.Dir, which would otherwise re-anchor a
	// relative bare path and split init/index-pack between two locations.
	bare := filepath.Join(*rundir, "cold", "bare.git")
	if abs, e := filepath.Abs(bare); e == nil {
		bare = abs
	}
	if _, err := os.Stat(bare); err == nil {
		return errors.New("cold bare repo already exists; use a fresh run directory")
	}
	if _, err := gitRun(*rundir, nil, "init", "--bare", "--object-format=sha1", bare); err != nil {
		return err
	}
	if err := (&gitio.Repo{GitDir: bare}).IndexVerified(ctx, pf, cdoc.PackObject, cdoc.Context.Commit.OID); err != nil {
		result.GitIndex = "FAIL: " + err.Error()
		pf.Close()
		return finishRead(*rundir, &result, doc)
	}
	pf.Close()
	result.GitIndex = "PASS"

	env := []string{"GIT_DIR=" + bare}
	tree, _ := gitRun(*rundir, env, "rev-parse", cdoc.Context.Commit.OID+"^{tree}")
	log, _ := gitRun(*rundir, env, "log", "--oneline", cdoc.Context.Commit.OID)
	result.TreeMatch = tree == doc.TipTree
	result.LogMatch = strings.Join(doc.SourceLog, "\n") == log
	result.Notes = append(result.Notes,
		"cold read used only the reader credential references; writer variables were not required",
		"identity isolation NOT PROVEN unless reader credentials are a distinct cloud principal",
	)
	// Anonymous public path: full-byte verification plus a separate CORS fact.
	if pub := strings.TrimRight(rp.PublicReadBase, "/"); pub != "" {
		diag := func(key string, o packstore.Object) string {
			d, err := rd.DiagnosePublic(ctx, pub+"/"+key, *origin, o)
			switch {
			case err != nil:
				return "FAIL: " + err.Error()
			case !d.CORS:
				return "PASS bytes verified; CORS header absent for origin " + *origin
			default:
				return "PASS bytes verified; CORS allowed for origin " + *origin
			}
		}
		result.PublicManifest = diag(doc.ManifestKey, cdoc.Manifest)
		result.PublicPack = diag(doc.PackKey, cdoc.PackObject)
		result.Notes = append(result.Notes, "browser cross-origin reads additionally require a bucket CORS rule; its absence is reported, not fixed, by this tool")
	}
	return finishRead(*rundir, &result, doc)
}

func finishRead(rundir string, result *readResult, doc planDoc) error {
	out, _ := json.MarshalIndent(result, "", "  ")
	os.Stdout.Write(redact(out, secretValues(doc.EnvNames.Reader)))
	fmt.Println()
	if err := writeJSON(filepath.Join(rundir, "results-read.json"), result); err != nil {
		return err
	}
	if result.Manifest != "PASS" || result.Pack != "PASS" || result.GitIndex != "PASS" || !result.TreeMatch || !result.LogMatch ||
		strings.HasPrefix(result.PublicManifest, "FAIL") || strings.HasPrefix(result.PublicPack, "FAIL") {
		return errors.New("cold read verification incomplete; see results-read.json")
	}
	return nil
}
