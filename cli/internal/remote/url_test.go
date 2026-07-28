package remote

import "testing"

func TestParseURL(t *testing.T) {
	owner := "inj1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"
	cases := []struct {
		in      string
		owner   string
		repo    string
		wantErr bool
	}{
		{"inj://" + owner + "/hello", owner, "hello", false},
		{"inj://" + owner + "/hello.git", owner, "hello", false},
		{"inj://" + owner + "/hello/", owner, "hello", false},
		{"inj::" + owner + "/hello", owner, "hello", false},
		{owner + "/hello", owner, "hello", false},
		{"inj://onlyowner", "", "", true},
		{"inj://a/b/c", "", "", true},
		{"inj://notbech32/hello", "", "", true},
		{"", "", "", true},
	}
	for _, tc := range cases {
		got, err := ParseURL(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseURL(%q): expected error, got %+v", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseURL(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if got.Owner != tc.owner || got.Repo != tc.repo {
			t.Errorf("ParseURL(%q) = %+v, want owner=%s repo=%s", tc.in, got, tc.owner, tc.repo)
		}
	}
}

func TestParsePushSpec(t *testing.T) {
	cases := []struct {
		in    string
		src   string
		dst   string
		force bool
	}{
		{"push refs/heads/main:refs/heads/main", "refs/heads/main", "refs/heads/main", false},
		{"push +refs/heads/dev:refs/heads/dev", "refs/heads/dev", "refs/heads/dev", true},
		{"push :refs/heads/gone", "", "refs/heads/gone", false},
	}
	for _, tc := range cases {
		got := parsePushSpec(tc.in)
		if got.src != tc.src || got.dst != tc.dst || got.force != tc.force {
			t.Errorf("parsePushSpec(%q) = %+v, want src=%q dst=%q force=%v",
				tc.in, got, tc.src, tc.dst, tc.force)
		}
	}
}
