package ipfs

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

// GatewayHealth is the outcome of a lightweight public-gateway probe.
type GatewayHealth struct {
	Gateway config.Gateway
	Latency time.Duration
	Err     error
}

// ProbeGateways checks every configured health endpoint concurrently. A
// gateway is healthy only when /healthz returns a 2xx status, avoiding a
// content fetch during control-plane checks.
func ProbeGateways(ctx context.Context, gateways []config.Gateway) []GatewayHealth {
	results := make([]GatewayHealth, len(gateways))
	// Cross-region TLS handshakes from mainland networks commonly take 2-3s;
	// leave enough margin to avoid treating ordinary jitter as an outage.
	client := &http.Client{Timeout: 5 * time.Second}
	var wg sync.WaitGroup
	for i, gateway := range gateways {
		wg.Add(1)
		go func(i int, gateway config.Gateway) {
			defer wg.Done()
			started := time.Now()
			endpoint := strings.TrimRight(gateway.URL, "/") + "/healthz"
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			if err != nil {
				results[i] = GatewayHealth{Gateway: gateway, Err: err}
				return
			}
			resp, err := client.Do(req)
			latency := time.Since(started)
			if err != nil {
				results[i] = GatewayHealth{Gateway: gateway, Latency: latency, Err: err}
				return
			}
			resp.Body.Close()
			if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
				results[i] = GatewayHealth{Gateway: gateway, Latency: latency, Err: fmt.Errorf("HTTP %d", resp.StatusCode)}
				return
			}
			results[i] = GatewayHealth{Gateway: gateway, Latency: latency}
		}(i, gateway)
	}
	wg.Wait()
	return results
}

// SelectGateways returns healthy endpoints from fastest to slowest. When all
// health probes fail (for example a captive network blocks /healthz), return
// the original endpoint order so actual content requests still get a chance.
func SelectGateways(ctx context.Context, gateways []config.Gateway) ([]config.Gateway, []GatewayHealth) {
	health := ProbeGateways(ctx, gateways)
	var selected []GatewayHealth
	for _, result := range health {
		if result.Err == nil {
			selected = append(selected, result)
		}
	}
	if len(selected) == 0 {
		return gateways, health
	}
	sort.SliceStable(selected, func(i, j int) bool { return selected[i].Latency < selected[j].Latency })
	gateways = make([]config.Gateway, 0, len(selected))
	for _, result := range selected {
		gateways = append(gateways, result.Gateway)
	}
	return gateways, health
}
