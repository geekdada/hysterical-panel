package oidc

// Endpoint paths relative to the issuer.
const (
	DiscoveryPath  = "/.well-known/openid-configuration"
	AuthorizePath  = "/authorize"
	TokenPath      = "/token"
	UserInfoPath   = "/userinfo"
	JWKSPath       = "/jwks"
	RevocationPath = "/revoke"
)

var clientAuthMethods = []string{"client_secret_basic", "client_secret_post"}

// ProviderMetadata is the discovery document (OIDC Discovery 3 and the
// RFC 8414 fields the panel supports).
type ProviderMetadata struct {
	Issuer                                     string   `json:"issuer"`
	AuthorizationEndpoint                      string   `json:"authorization_endpoint"`
	TokenEndpoint                              string   `json:"token_endpoint"`
	UserInfoEndpoint                           string   `json:"userinfo_endpoint"`
	JWKSURI                                    string   `json:"jwks_uri"`
	RevocationEndpoint                         string   `json:"revocation_endpoint"`
	ResponseTypesSupported                     []string `json:"response_types_supported"`
	ResponseModesSupported                     []string `json:"response_modes_supported"`
	GrantTypesSupported                        []string `json:"grant_types_supported"`
	SubjectTypesSupported                      []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported           []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                            []string `json:"scopes_supported"`
	ClaimsSupported                            []string `json:"claims_supported"`
	TokenEndpointAuthMethodsSupported          []string `json:"token_endpoint_auth_methods_supported"`
	RevocationEndpointAuthMethodsSupported     []string `json:"revocation_endpoint_auth_methods_supported"`
	CodeChallengeMethodsSupported              []string `json:"code_challenge_methods_supported"`
	PromptValuesSupported                      []string `json:"prompt_values_supported"`
	AuthorizationResponseISSParameterSupported bool     `json:"authorization_response_iss_parameter_supported"`
	RequestParameterSupported                  bool     `json:"request_parameter_supported"`
	RequestURIParameterSupported               bool     `json:"request_uri_parameter_supported"`
	ClaimsParameterSupported                   bool     `json:"claims_parameter_supported"`
}

// Discovery describes the provider published at issuer.
func Discovery(issuer string) ProviderMetadata {
	return ProviderMetadata{
		Issuer:                                     issuer,
		AuthorizationEndpoint:                      issuer + AuthorizePath,
		TokenEndpoint:                              issuer + TokenPath,
		UserInfoEndpoint:                           issuer + UserInfoPath,
		JWKSURI:                                    issuer + JWKSPath,
		RevocationEndpoint:                         issuer + RevocationPath,
		ResponseTypesSupported:                     []string{"code"},
		ResponseModesSupported:                     []string{"query"},
		GrantTypesSupported:                        []string{"authorization_code", "refresh_token"},
		SubjectTypesSupported:                      []string{"public"},
		IDTokenSigningAlgValuesSupported:           []string{SigningAlgorithm},
		ScopesSupported:                            []string{ScopeOpenID, ScopeEmail, ScopeOfflineAccess},
		ClaimsSupported:                            []string{"iss", "sub", "aud", "exp", "iat", "auth_time", "nonce", "at_hash", "email", "email_verified"},
		TokenEndpointAuthMethodsSupported:          clientAuthMethods,
		RevocationEndpointAuthMethodsSupported:     clientAuthMethods,
		CodeChallengeMethodsSupported:              []string{"S256"},
		PromptValuesSupported:                      PromptValues,
		AuthorizationResponseISSParameterSupported: true,
	}
}
