// Package storageconfig is separate from network/Suite configuration. It loads
// only explicitly selected files, and stores credential references, never values.
package storageconfig

import (
	"context"
	"errors"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/packmanifest"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/safehttp"
	"github.com/Hny0305Lin/next-injective-git/cli/internal/strictjson"
	"github.com/aws/aws-sdk-go-v2/aws"
	"io"
	"os"
	"regexp"
	"strings"
)

var ErrConfig = errors.New("invalid storage configuration or missing independent credential reference")
var name = regexp.MustCompile("^[A-Za-z][A-Za-z0-9_-]{0,63}$")
var envName = regexp.MustCompile("^[A-Z][A-Z0-9_]{0,127}$")
var bucket = regexp.MustCompile("^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$")
var account = regexp.MustCompile("^[0-9a-f]{32}$")

// Explicit environment variable names are the first supported secure source.
// No LoadDefaultConfig, shared-file, container or instance-metadata fallback.
type CredentialRef struct {
	Kind            string `json:"kind"`
	AccessKeyEnv    string `json:"accessKeyEnv"`
	SecretKeyEnv    string `json:"secretKeyEnv"`
	SessionTokenEnv string `json:"sessionTokenEnv,omitempty"`
}

func (c CredentialRef) Validate() error {
	if c.Kind != "env" || !envName.MatchString(c.AccessKeyEnv) || !envName.MatchString(c.SecretKeyEnv) || c.AccessKeyEnv == c.SecretKeyEnv || c.SessionTokenEnv != "" && (!envName.MatchString(c.SessionTokenEnv) || c.SessionTokenEnv == c.AccessKeyEnv || c.SessionTokenEnv == c.SecretKeyEnv) {
		return ErrConfig
	}
	return nil
}
func (c CredentialRef) Resolve(lookup func(string) (string, bool)) aws.CredentialsProvider {
	return aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		if ctx.Err() != nil || c.Validate() != nil {
			return aws.Credentials{}, ErrConfig
		}
		a, ok := lookup(c.AccessKeyEnv)
		s, ok2 := lookup(c.SecretKeyEnv)
		var token string
		if c.SessionTokenEnv != "" {
			var found bool
			token, found = lookup(c.SessionTokenEnv)
			if !found || token == "" {
				return aws.Credentials{}, ErrConfig
			}
		}
		if !ok || !ok2 || a == "" || s == "" {
			return aws.Credentials{}, ErrConfig
		}
		return aws.Credentials{AccessKeyID: a, SecretAccessKey: s, SessionToken: token, Source: "igit-explicit-env"}, nil
	})
}

type Profile struct {
	Provider       string         `json:"provider"`
	Bucket         string         `json:"bucket"`
	Region         string         `json:"region"`
	AccountID      string         `json:"accountId,omitempty"`
	Prefix         string         `json:"prefix"`
	PublicReadBase string         `json:"publicReadBase,omitempty"`
	CredentialRef  *CredentialRef `json:"credentialRef,omitempty"`
}

// These are the commercial AWS regions accepted by this first profile. Adding
// partitions or regions requires a reviewed endpoint/capability update.
var regions = strings.Fields("af-south-1 ap-east-1 ap-east-2 ap-northeast-1 ap-northeast-2 ap-northeast-3 ap-south-1 ap-south-2 ap-southeast-1 ap-southeast-2 ap-southeast-3 ap-southeast-4 ap-southeast-5 ap-southeast-6 ap-southeast-7 ca-central-1 ca-west-1 eu-central-1 eu-central-2 eu-north-1 eu-south-1 eu-south-2 eu-west-1 eu-west-2 eu-west-3 il-central-1 me-central-1 me-south-1 mx-central-1 sa-east-1 us-east-1 us-east-2 us-west-1 us-west-2")

func (p Profile) Validate() error {
	if !bucket.MatchString(p.Bucket) || strings.HasPrefix(p.Bucket, "xn--") || strings.HasSuffix(p.Bucket, "-s3alias") || strings.HasSuffix(p.Bucket, "--x-s3") || !packmanifest.ValidPrefix(p.Prefix) {
		return ErrConfig
	}
	switch p.Provider {
	case "aws-s3":
		found := false
		for _, r := range regions {
			if r == p.Region {
				found = true
			}
		}
		if !found || p.AccountID != "" {
			return ErrConfig
		}
	case "cloudflare-r2":
		if p.Region != "auto" || !account.MatchString(p.AccountID) {
			return ErrConfig
		}
	default:
		return ErrConfig
	}
	if p.PublicReadBase != "" {
		if _, err := safehttp.ValidateURL(p.PublicReadBase); err != nil {
			return ErrConfig
		}
	}
	if p.CredentialRef != nil {
		return p.CredentialRef.Validate()
	}
	return nil
}
func (p Profile) Endpoint() (string, error) {
	if p.Validate() != nil {
		return "", ErrConfig
	}
	if p.Provider == "cloudflare-r2" {
		return "https://" + p.AccountID + ".r2.cloudflarestorage.com", nil
	}
	return "https://s3." + p.Region + ".amazonaws.com", nil
}

type Binding struct {
	ChainID        string `json:"chainId"`
	SuiteDirectory string `json:"suiteDirectory"`
	RepoID         string `json:"repoId"`
	Writer         string `json:"writer"`
	Reader         string `json:"reader"`
}
type Config struct {
	Version      int                `json:"version"`
	Profiles     map[string]Profile `json:"profiles"`
	Repositories []Binding          `json:"repositories"`
}

func (c Config) Validate() error {
	if c.Version != 1 || len(c.Profiles) < 1 || len(c.Profiles) > 32 || len(c.Repositories) > 128 {
		return ErrConfig
	}
	for n, p := range c.Profiles {
		if !name.MatchString(n) || p.Validate() != nil {
			return ErrConfig
		}
	}
	seen := map[string]bool{}
	for _, b := range c.Repositories {
		ctx := packmanifest.Context{ChainID: b.ChainID, SuiteDirectory: b.SuiteDirectory, RepoID: b.RepoID, RefName: "refs/heads/config", Commit: packmanifest.Commit{Algorithm: "sha1", OID: strings.Repeat("1", 40)}}
		key := b.ChainID + ":" + b.SuiteDirectory + ":" + b.RepoID
		if ctx.Validate() != nil || seen[key] {
			return ErrConfig
		}
		seen[key] = true
		w, wok := c.Profiles[b.Writer]
		r, rok := c.Profiles[b.Reader]
		if !wok || !rok || b.Writer == b.Reader || w.CredentialRef == nil || (r.CredentialRef == nil && r.PublicReadBase == "") {
			return ErrConfig
		}
		if r.CredentialRef != nil && (w.CredentialRef.AccessKeyEnv == r.CredentialRef.AccessKeyEnv || w.CredentialRef.SecretKeyEnv == r.CredentialRef.SecretKeyEnv) {
			return ErrConfig
		}
	}
	return nil
}
func Load(path string) (Config, error) {
	var c Config
	f, err := os.Open(path)
	if err != nil {
		return c, ErrConfig
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil || strictjson.Decode(b, 65536, &c) != nil || c.Validate() != nil {
		return Config{}, ErrConfig
	}
	return c, nil
}
