package api

// ── Node ──────────────────────────────────────────────────────────────────────

// Node is the public representation returned by every node endpoint.
// api_secret is intentionally omitted.
type Node struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	APIURL         string `json:"api_url"`
	PollInterval   int    `json:"poll_interval"`
	Enabled        bool   `json:"enabled"`
	LastPolledAt   string `json:"last_polled_at"`
	LastError      string `json:"last_error"`
	Health         string `json:"health"` // "ok" | "error" | "never"
	CurrentTxSpeed int64  `json:"current_tx_speed"`
	CurrentRxSpeed int64  `json:"current_rx_speed"`
}

// NodeCreateRequest is the body for POST /nodes.
type NodeCreateRequest struct {
	Name         string `json:"name"`
	APIURL       string `json:"api_url"`
	APISecret    string `json:"api_secret"`
	PollInterval *int   `json:"poll_interval,omitempty"`
	Enabled      *bool  `json:"enabled,omitempty"`
}

// NodeUpdateRequest is the body for PATCH /nodes/{id}.
// All fields optional; api_secret omitted means unchanged, empty string is an error.
type NodeUpdateRequest struct {
	Name         *string `json:"name,omitempty"`
	APIURL       *string `json:"api_url,omitempty"`
	APISecret    *string `json:"api_secret,omitempty"`
	PollInterval *int    `json:"poll_interval,omitempty"`
	Enabled      *bool   `json:"enabled,omitempty"`
}

// NodeTestResponse is the body returned by POST /nodes/{id}/test.
type NodeTestResponse struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
}

// ── User ──────────────────────────────────────────────────────────────────────

// PanelUser is the public representation returned by user endpoints.
type PanelUser struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	Role       string `json:"role"` // "admin" | "user"
	AuthString string `json:"auth_string"`
	QuotaBytes int64  `json:"quota_bytes"`
	UsedTx     int64  `json:"used_tx"`
	UsedRx     int64  `json:"used_rx"`
	Status     string `json:"status"` // "active" | "disabled"
}

// UserCreateRequest is the body for POST /users.
type UserCreateRequest struct {
	Email      string  `json:"email"`
	Password   string  `json:"password"`
	AuthString string  `json:"auth_string"`
	Role       *string `json:"role,omitempty"`
	QuotaBytes *int64  `json:"quota_bytes,omitempty"`
	Status     *string `json:"status,omitempty"`
}

// UserUpdateRequest is the body for PATCH /users/{id}.
type UserUpdateRequest struct {
	Email      *string `json:"email,omitempty"`
	Password   *string `json:"password,omitempty"`
	AuthString *string `json:"auth_string,omitempty"`
	Role       *string `json:"role,omitempty"`
	QuotaBytes *int64  `json:"quota_bytes,omitempty"`
	Status     *string `json:"status,omitempty"`
}

// ── Passkeys ─────────────────────────────────────────────────────────────────

// Passkey is the public representation of a registered passkey credential.
type Passkey struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Transports     []string `json:"transports"`
	SignCount      int64    `json:"sign_count"`
	BackupEligible bool     `json:"backup_eligible"`
	BackupState    bool     `json:"backup_state"`
	CloneWarning   bool     `json:"clone_warning"`
	Created        string   `json:"created"`
	Updated        string   `json:"updated"`
	LastUsedAt     string   `json:"last_used_at"`
}

// PasskeyOptionsResponse returns a server-side challenge id and WebAuthn options.
type PasskeyOptionsResponse struct {
	ChallengeID string         `json:"challenge_id"`
	Options     map[string]any `json:"options"`
}

// PasskeyFinishRequest is used by passkey login and registration finish endpoints.
type PasskeyFinishRequest struct {
	ChallengeID string         `json:"challenge_id"`
	Credential  map[string]any `json:"credential"`
	Name        *string        `json:"name,omitempty"`
}

// PanelAuthResponse is the PocketBase-compatible auth response returned by passkey login.
type PanelAuthResponse struct {
	Token  string    `json:"token"`
	Record PanelUser `json:"record"`
}

// ── Traffic ───────────────────────────────────────────────────────────────────

// PanelTrafficResponse is returned by GET /traffic.
type PanelTrafficResponse struct {
	From  string    `json:"from"`
	To    string    `json:"to"`
	Total ByteCount `json:"total"`
}

// PanelNodeTrafficResponse is returned by GET /nodes/traffic/summary.
type PanelNodeTrafficResponse struct {
	From   string        `json:"from"`
	To     string        `json:"to"`
	Total  ByteCount     `json:"total"`
	ByNode []NodeTraffic `json:"by_node"`
}

// TrafficSummaryResponse is returned by GET /users/{id}/traffic/summary (UTC today).
type TrafficSummaryResponse struct {
	Total  ByteCount     `json:"total"`
	ByNode []NodeTraffic `json:"by_node"`
}

// ByteCount represents tx/rx byte totals.
type ByteCount struct {
	Tx int64 `json:"tx"`
	Rx int64 `json:"rx"`
}

// NodeRef is a lightweight node identity used inside aggregation responses.
type NodeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// UserRef is a lightweight user identity used inside aggregation responses.
type UserRef struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// NodeTraffic is a per-node traffic breakdown inside summary responses.
type NodeTraffic struct {
	Node NodeRef `json:"node"`
	Tx   int64   `json:"tx"`
	Rx   int64   `json:"rx"`
}

// UserTraffic is a per-user traffic breakdown inside node summary responses.
type UserTraffic struct {
	User UserRef `json:"user"`
	Tx   int64   `json:"tx"`
	Rx   int64   `json:"rx"`
}

// NodeTrafficSummaryResponse is returned by GET /nodes/{id}/traffic/summary (UTC today).
type NodeTrafficSummaryResponse struct {
	Total  ByteCount     `json:"total"`
	ByUser []UserTraffic `json:"by_user"`
}

// TrafficSeriesResponse is returned by GET /users/{id}/traffic/series.
type TrafficSeriesResponse struct {
	Granularity string        `json:"granularity"` // "hourly" | "daily"
	Points      []BucketPoint `json:"points"`
}

// BucketPoint is one time-bucketed data point in a series.
type BucketPoint struct {
	Bucket string `json:"bucket"`
	Tx     int64  `json:"tx"`
	Rx     int64  `json:"rx"`
}

// ── Database management ──────────────────────────────────────────────────────

// DatabaseStatsResponse is returned by GET /database/stats.
type DatabaseStatsResponse struct {
	Cutoff        string                      `json:"cutoff"`
	Storage       DatabaseStorageStats        `json:"storage"`
	TrafficTables []DatabaseTrafficTableStats `json:"traffic_tables"`
}

// DatabaseStorageStats contains the on-disk database file footprint.
type DatabaseStorageStats struct {
	TotalBytes int64                 `json:"total_bytes"`
	Files      []DatabaseStorageFile `json:"files"`
}

// DatabaseStorageFile is one database-related file in pb_data.
type DatabaseStorageFile struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// DatabaseTrafficTableStats contains row counts for one traffic table.
type DatabaseTrafficTableStats struct {
	Table           string `json:"table"`
	Points          int64  `json:"points"`
	OlderThan30Days int64  `json:"older_than_30_days"`
}

// DatabasePruneResponse is returned by POST /database/prune.
type DatabasePruneResponse struct {
	Cutoff  string                       `json:"cutoff"`
	Deleted []DatabaseTrafficPruneResult `json:"deleted"`
}

// DatabaseTrafficPruneResult contains deleted row counts for one traffic table.
type DatabaseTrafficPruneResult struct {
	Table       string `json:"table"`
	DeletedRows int64  `json:"deleted_rows"`
}

// ── Live ──────────────────────────────────────────────────────────────────────

// LiveResponse is returned by GET /users/{id}/live.
type LiveResponse struct {
	OnlineDevices int              `json:"online_devices"`
	ActiveStreams int              `json:"active_streams"`
	ByNode        []LiveNodeResult `json:"by_node"`
	TopDomains    []TopDomain      `json:"top_domains"`
	ByConnection  []ConnSummary    `json:"by_connection"`
}

// LiveNodeResult is the per-node breakdown inside the live response.
type LiveNodeResult struct {
	Node          NodeRef      `json:"node"`
	OnlineDevices int          `json:"online_devices"`
	Streams       []LiveStream `json:"streams"`
	Error         string       `json:"error,omitempty"`
}

// NodeLiveResponse is returned by GET /nodes/{id}/live. Unlike the user live
// response it covers a single node and every user on it (no auth filter).
type NodeLiveResponse struct {
	OnlineDevices int                  `json:"online_devices"`
	ActiveStreams int                  `json:"active_streams"`
	ByUser        []NodeLiveUserResult `json:"by_user"`
	TopDomains    []TopDomain          `json:"top_domains"`
	ByConnection  []ConnSummary        `json:"by_connection"`
	Error         string               `json:"error,omitempty"`
}

// NodeLiveUserResult groups a node's live streams under one panel user. Streams
// whose auth string matches no panel user are grouped under an "unknown" entry.
type NodeLiveUserResult struct {
	User          UserRef      `json:"user"`
	OnlineDevices int          `json:"online_devices"`
	Streams       []LiveStream `json:"streams"`
}

// LiveStream is one active stream shown in live diagnostics.
type LiveStream struct {
	Connection    int64  `json:"connection"`
	Stream        int64  `json:"stream"`
	State         string `json:"state"`
	ReqAddr       string `json:"req_addr"`
	HookedReqAddr string `json:"hooked_req_addr"`
	Tx            int64  `json:"tx"`
	Rx            int64  `json:"rx"`
	InitialAt     string `json:"initial_at"`
	LastActiveAt  string `json:"last_active_at"`
	LifetimeSec   int64  `json:"lifetime_sec"`
	IdleSec       int64  `json:"idle_sec"`
}

// TopDomain is a domain aggregation entry in the live response.
type TopDomain struct {
	Domain  string  `json:"domain"`
	Streams int     `json:"streams"`
	Tx      int64   `json:"tx"`
	Rx      int64   `json:"rx"`
	IPMeta  *IPMeta `json:"ip_meta,omitempty"`
}

// IPMeta is MMDB-derived metadata for IP literal domain rows.
type IPMeta struct {
	IP          string `json:"ip"`
	ASN         string `json:"asn,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	CountryName string `json:"country_name,omitempty"`
	IPInfoURL   string `json:"ipinfo_url,omitempty"`
}

// ConnSummary is a per-connection (device) summary in the live response.
type ConnSummary struct {
	Connection  int64  `json:"connection"`
	StreamCount int    `json:"stream_count"`
	Tx          int64  `json:"tx"`
	Rx          int64  `json:"rx"`
	TopDomain   string `json:"top_domain"`
}

// ── Config ────────────────────────────────────────────────────────────────────

// PanelConfigResponse is returned by GET /api/panel/config (no auth).
type PanelConfigResponse struct {
	APIURL          string `json:"api_url"`
	FrontendURL     string `json:"frontend_url"`
	PasskeysEnabled bool   `json:"passkeys_enabled"`
	Version         string `json:"version"`
}

// ── Shared ────────────────────────────────────────────────────────────────────

// DeleteResponse is returned by DELETE endpoints.
type DeleteResponse struct {
	Deleted bool `json:"deleted"`
}

// ErrorResponse is a generic error envelope.
type ErrorResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}
