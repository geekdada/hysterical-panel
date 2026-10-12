package oidc

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestDiscoveryAdvertisesTheNarrowProfile(t *testing.T) {
	const issuer = "https://api.example.com/api/oidc"
	meta := Discovery(issuer)
	if meta.Issuer != issuer || meta.AuthorizationEndpoint != issuer+"/authorize" || meta.TokenEndpoint != issuer+"/token" ||
		meta.UserInfoEndpoint != issuer+"/userinfo" || meta.JWKSURI != issuer+"/jwks" || meta.RevocationEndpoint != issuer+"/revoke" {
		t.Fatalf("endpoints = %+v", meta)
	}
	if !slices.Equal(meta.ResponseTypesSupported, []string{"code"}) ||
		!slices.Equal(meta.CodeChallengeMethodsSupported, []string{"S256"}) ||
		!slices.Equal(meta.IDTokenSigningAlgValuesSupported, []string{"RS256"}) ||
		!slices.Equal(meta.GrantTypesSupported, []string{"authorization_code", "refresh_token"}) ||
		!meta.AuthorizationResponseISSParameterSupported {
		t.Fatalf("profile = %+v", meta)
	}
	// request_uri_parameter_supported defaults to true when absent, so the
	// false value must be written out.
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if value, ok := fields["request_uri_parameter_supported"]; !ok || value != false {
		t.Fatalf("request_uri_parameter_supported = %v (present %v)", value, ok)
	}
}
