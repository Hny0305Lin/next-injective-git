package chain

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

func TestClientListReposDrainsV1NamePages(t *testing.T) {
	var starts []any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		parts := strings.SplitN(req.URL.EscapedPath(), "/smart/", 2)
		if len(parts) != 2 {
			t.Errorf("unexpected smart-query path %q", req.URL.EscapedPath())
			return
		}
		encoded, err := url.PathUnescape(parts[1])
		if err != nil {
			t.Errorf("decode smart-query path: %v", err)
			return
		}
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Errorf("decode smart-query base64: %v", err)
			return
		}
		var query struct {
			ListRepos struct {
				Owner      string  `json:"owner"`
				StartAfter *string `json:"start_after"`
				Limit      int     `json:"limit"`
			} `json:"list_repos"`
		}
		if err := json.Unmarshal(raw, &query); err != nil {
			t.Errorf("decode smart query: %v", err)
			return
		}
		if query.ListRepos.Owner != "inj1owner" || query.ListRepos.Limit != 100 {
			t.Errorf("list_repos query = %#v", query.ListRepos)
		}
		if query.ListRepos.StartAfter == nil {
			starts = append(starts, nil)
		} else {
			starts = append(starts, *query.ListRepos.StartAfter)
		}

		repositories := make([]RepoInfo, 0, 100)
		if query.ListRepos.StartAfter == nil {
			for i := 0; i < 100; i++ {
				repositories = append(repositories, RepoInfo{
					Owner: "inj1owner", Name: fmt.Sprintf("repo-%03d", i), DefaultBranch: "main",
				})
			}
		} else if *query.ListRepos.StartAfter == "repo-099" {
			repositories = append(repositories, RepoInfo{
				Owner: "inj1owner", Name: "repo-100", DefaultBranch: "main",
			})
		} else {
			t.Errorf("unexpected start_after %q", *query.ListRepos.StartAfter)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"repos": repositories},
		})
	}))
	defer server.Close()

	cfg := config.Defaults()
	cfg.LCDEndpoint = server.URL
	cfg.ContractAddress = "inj1contract"
	repositories, err := New(cfg).ListRepos("inj1owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(repositories) != 101 || repositories[0].Name != "repo-000" || repositories[100].Name != "repo-100" {
		t.Fatalf("repositories = %d entries, first=%q last=%q", len(repositories), repositories[0].Name, repositories[len(repositories)-1].Name)
	}
	if len(starts) != 2 || starts[0] != nil || starts[1] != "repo-099" {
		t.Fatalf("start_after sequence = %#v", starts)
	}
}
