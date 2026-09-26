package safehttp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
)

func TestURLsAndSpecialAddresses(t *testing.T) {
	for _, s := range []string{"http://public.example.com/a", "https://user:pass@public.example.com/a", "https://public.example.com:443/a", "https://public.example.com/a?X-Amz-Signature=secret", "https://public.example.com/a?", "https://public.example.com/a#", "https://public.example.com/a/../b", "https://public.example.com/%2e%2e/x", "https://127.0.0.1/a", "https://[::1]/a", "https://metadata.google.internal/a", "https://example.local/a", "https://public.example.com//a", "https://public.example.com./a"} {
		if _, e := ValidateURL(s); e == nil {
			t.Error("accepted", s)
		}
	}
	for _, s := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.100.100.200", "172.16.0.1", "192.168.1.1", "0.0.0.0", "::1", "::ffff:8.8.8.8", "fc00::1", "fe80::1", "64:ff9b::a00:1", "2002:7f00:1::", "192.0.2.1"} {
		if PublicIP(netip.MustParseAddr(s)) {
			t.Error("public", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !PublicIP(netip.MustParseAddr(s)) {
			t.Error("blocked public", s)
		}
	}
}

type fakeDNS struct {
	ips   []netip.Addr
	calls int
}

func (d *fakeDNS) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	d.calls++
	return d.ips, nil
}
func TestDNSPinningAndMixedAnswers(t *testing.T) {
	d := &fakeDNS{ips: []netip.Addr{netip.MustParseAddr("8.8.8.8")}}
	called := ""
	dial := func(_ context.Context, network, addr string) (net.Conn, error) {
		called = addr
		return nil, errors.New("fake dial")
	}
	_, _ = dialPublic(context.Background(), "tcp", "storage.example.com:443", d, dial)
	if called != "8.8.8.8:443" || d.calls != 1 {
		t.Fatal("DNS not pinned", called)
	}
	called = ""
	d.ips = append(d.ips, netip.MustParseAddr("127.0.0.1"))
	_, e := dialPublic(context.Background(), "tcp", "storage.example.com:443", d, dial)
	if !errors.Is(e, ErrEndpoint) || called != "" {
		t.Fatal("mixed answer dialed")
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCredentialBoundaryAndRedirect(t *testing.T) {
	called := false
	rt := roundTrip(func(r *http.Request) (*http.Response, error) {
		called = true
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Amz-Security-Token") != "" {
			t.Error("credential leak")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})
	req, _ := http.NewRequest("GET", "https://public.example.com/a", nil)
	req.Header.Set("Authorization", "secret")
	req.Header.Set("Cookie", "secret")
	req.Header.Set("X-Amz-Security-Token", "secret")
	res, e := (guard{next: rt}).RoundTrip(req)
	if e != nil || !called {
		t.Fatal(e)
	}
	res.Body.Close()
	called = false
	if _, e = (guard{next: rt, signedHost: "s3.us-east-1.amazonaws.com"}).RoundTrip(req); e == nil || called {
		t.Fatal("signed redirect/host allowed")
	}
	if NewClient("").CheckRedirect(req, nil) == nil {
		t.Fatal("redirect allowed")
	}
}
