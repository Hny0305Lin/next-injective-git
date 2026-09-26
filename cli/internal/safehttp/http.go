// Package safehttp provides restricted HTTPS reads and signed cloud transports.
package safehttp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var ErrEndpoint = errors.New("unsafe storage endpoint")
var hostname = regexp.MustCompile("^[a-z0-9]+([a-z0-9.-]*[a-z0-9])?$")
var domainSuffix = regexp.MustCompile("^[a-z][a-z0-9-]*[a-z0-9]$")
var path = regexp.MustCompile("^(/[a-zA-Z0-9_-][a-zA-Z0-9_.-]*)*/?$")

// ValidateURL accepts a narrow stable URL grammar: no encoded paths, query,
// userinfo, fragment, IP literal or custom port. DNS is checked at dial time.
func ValidateURL(raw string) (*url.URL, error) {
	if !strings.HasPrefix(raw, "https://") {
		return nil, ErrEndpoint
	}
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Scheme != "https" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") || u.Port() != "" || u.Host != u.Hostname() || !hostname.MatchString(u.Host) || !strings.Contains(u.Host, ".") || strings.Contains(u.Host, "..") || u.RawPath != "" || strings.Contains(raw, "%") || !path.MatchString(u.Path) {
		return nil, ErrEndpoint
	}
	if len(u.Host) > 253 || net.ParseIP(u.Host) != nil {
		return nil, ErrEndpoint
	}
	labels := strings.Split(u.Host, ".")
	if !domainSuffix.MatchString(labels[len(labels)-1]) {
		return nil, ErrEndpoint
	}
	for _, s := range []string{".localhost", ".local", ".internal", ".test", ".invalid"} {
		if strings.HasSuffix(u.Host, s) {
			return nil, ErrEndpoint
		}
	}
	for _, p := range strings.Split(u.Host, ".") {
		if len(p) == 0 || len(p) > 63 || strings.HasPrefix(p, "-") || strings.HasSuffix(p, "-") {
			return nil, ErrEndpoint
		}
	}
	for _, p := range strings.Split(u.Path, "/") {
		if p == "." || p == ".." {
			return nil, ErrEndpoint
		}
	}
	return u, nil
}

var denied = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("224.0.0.0/3"), netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
}

func PublicIP(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Is4In6() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, p := range denied {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

type resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

// Resolve once, reject mixed public/private answers, then dial the checked IP.
func dialPublic(ctx context.Context, network, addr string, r resolver, dial func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port != "443" {
		return nil, ErrEndpoint
	}
	ips, err := r.LookupNetIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, ErrEndpoint
	}
	for _, ip := range ips {
		if !PublicIP(ip) {
			return nil, ErrEndpoint
		}
	}
	return dial(ctx, network, net.JoinHostPort(ips[0].String(), port))
}

type guard struct {
	next       http.RoundTripper
	signedHost string
}

func (g guard) RoundTrip(req *http.Request) (*http.Response, error) {
	if g.signedHost != "" {
		if req.URL.Scheme != "https" || req.URL.Host != g.signedHost || req.URL.User != nil {
			return nil, ErrEndpoint
		}
	} else {
		if _, err := ValidateURL(req.URL.String()); err != nil {
			return nil, err
		}
		if req.Method != http.MethodGet && req.Method != http.MethodOptions {
			return nil, ErrEndpoint
		}
		req = req.Clone(req.Context())
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
		req.Header.Del("X-Amz-Security-Token")
	}
	return g.next.RoundTrip(req)
}

// No ambient proxy, cookies, redirects or default credential discovery.
// signedHost must be derived from a validated provider profile.
func NewClient(signedHost string) *http.Client {
	d := &net.Dialer{Timeout: 15 * time.Second}
	t := &http.Transport{Proxy: nil, DisableCompression: true, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 30 * time.Second, MaxIdleConns: 4, DialContext: func(ctx context.Context, n, a string) (net.Conn, error) {
		return dialPublic(ctx, n, a, net.DefaultResolver, d.DialContext)
	}}
	return &http.Client{Transport: guard{t, signedHost}, Timeout: 10 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrEndpoint }}
}
