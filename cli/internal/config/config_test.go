package config

import "testing"

func TestEffectiveGatewaysUsesProfileNameForLegacyDefault(t *testing.T) {
	gateways := Defaults().EffectiveGateways()
	if len(gateways) != 2 || gateways[0].Name != "hk" || gateways[1].Name != "us" {
		t.Fatalf("gateways = %#v, want hk then us", gateways)
	}
}

func TestEffectiveGatewaysPrependsCustomLegacyGateway(t *testing.T) {
	cfg := Defaults()
	cfg.IPFSGateway = "https://custom.example/"
	gateways := cfg.EffectiveGateways()
	if len(gateways) != 3 || gateways[0].Name != "custom" || gateways[0].URL != "https://custom.example" {
		t.Fatalf("gateways = %#v, want custom first", gateways)
	}
}

func TestEffectiveGatewaysKeepsPublicLegacyGatewayAsFallback(t *testing.T) {
	cfg := Defaults()
	cfg.IPFSGateway = "https://ipfs.io/"
	gateways := cfg.EffectiveGateways()
	if len(gateways) != 2 || gateways[0].Name != "hk" || gateways[1].Name != "us" {
		t.Fatalf("gateways = %#v, want project gateways before public fallback", gateways)
	}
	fallbacks := cfg.EffectiveReadFallbacks()
	if len(fallbacks) != 1 || fallbacks[0] != "https://ipfs.io" {
		t.Fatalf("fallbacks = %#v, want ipfs.io", fallbacks)
	}
}

func TestEffectiveUploadPeersUseBuiltInDefaultsForLegacyEmptyValues(t *testing.T) {
	cfg := Defaults()
	cfg.Upload.USPeer = ""
	cfg.Upload.HKPeer = ""
	peers := cfg.EffectiveUploadPeers()
	if len(peers) != 2 || peers[0] != DefaultUSUploadPeer || peers[1] != DefaultHKUploadPeer {
		t.Fatalf("peers = %#v, want built-in US then HK", peers)
	}
}
