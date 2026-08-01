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
