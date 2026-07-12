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
	OnlineDevices  *int64 `json:"online_devices"`
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

// NodeAPISecretResetResponse is returned by POST /nodes/{id}/reset-api-secret.
// api_secret is revealed once so the admin can update the Hysteria server config.
type NodeAPISecretResetResponse struct {
	APISecret string `json:"api_secret"`
	Node      Node   `json:"node"`
}

// ── Notification channels ───────────────────────────────────────────────────

// NotificationChannel is the public, non-secret representation of an outbound
// destination. Its Shoutrrr URL and encrypted ciphertext are never returned by
// ordinary channel endpoints.
type NotificationChannel struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Service        string  `json:"service"`
	Enabled        bool    `json:"enabled"`
	LastTestStatus string  `json:"last_test_status"` // "never" | "succeeded" | "failed"
	LastTestedAt   *string `json:"last_tested_at,omitempty"`
	LastTestError  string  `json:"last_test_error,omitempty"` // "timed_out" | "delivery_failed"
	Created        string  `json:"created"`
	Updated        string  `json:"updated"`
}

// NotificationChannelCreateRequest creates one destination. The URL is
// encrypted before persistence and is never returned in the response.
type NotificationChannelCreateRequest struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// NotificationChannelUpdateRequest updates Channel metadata or replaces its
// URL. Omitted URL leaves it unchanged; an empty URL is invalid.
type NotificationChannelUpdateRequest struct {
	Name    *string `json:"name,omitempty"`
	URL     *string `json:"url,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}

// NotificationChannelTestResponse reports the persisted result of an explicit
// test delivery. A failed provider delivery is still an HTTP 200 outcome.
type NotificationChannelTestResponse struct {
	Status   string `json:"status"`
	TestedAt string `json:"tested_at"`
	Error    string `json:"error,omitempty"`
}

// NotificationChannelRevealResponse returns the selected Channel URL only
// after a fresh, one-time passkey assertion.
type NotificationChannelRevealResponse struct {
	URL string `json:"url"`
}

// ── Monitoring ──────────────────────────────────────────────────────────────

type Monitor struct {
	ID                      string         `json:"id"`
	Name                    string         `json:"name"`
	Kind                    string         `json:"kind"`
	Enabled                 bool           `json:"enabled"`
	Severity                string         `json:"severity"`
	NotificationLanguage    string         `json:"notification_language"`
	EvaluationWindowSeconds int            `json:"evaluation_window_seconds"`
	NodeScope               string         `json:"node_scope"`
	NodeIDs                 []string       `json:"node_ids"`
	ChannelIDs              []string       `json:"channel_ids"`
	Config                  map[string]any `json:"config"`
	Created                 string         `json:"created"`
	Updated                 string         `json:"updated"`
}

type OfflineMonitorConfig struct{}

type HighTrafficMonitorConfig struct {
	ThresholdBytesPerSecond int64 `json:"threshold_bytes_per_second"`
}

type MonitorCreateRequest struct {
	Name                    string         `json:"name"`
	Kind                    string         `json:"kind"`
	Enabled                 *bool          `json:"enabled,omitempty"`
	Severity                string         `json:"severity"`
	NotificationLanguage    string         `json:"notification_language"`
	EvaluationWindowSeconds int            `json:"evaluation_window_seconds"`
	NodeScope               string         `json:"node_scope"`
	NodeIDs                 []string       `json:"node_ids"`
	ChannelIDs              []string       `json:"channel_ids"`
	Config                  map[string]any `json:"config"`
}

type MonitorUpdateRequest struct {
	Name                    *string         `json:"name,omitempty"`
	Kind                    *string         `json:"kind,omitempty"`
	Enabled                 *bool           `json:"enabled,omitempty"`
	Severity                *string         `json:"severity,omitempty"`
	NotificationLanguage    *string         `json:"notification_language,omitempty"`
	EvaluationWindowSeconds *int            `json:"evaluation_window_seconds,omitempty"`
	NodeScope               *string         `json:"node_scope,omitempty"`
	NodeIDs                 *[]string       `json:"node_ids,omitempty"`
	ChannelIDs              *[]string       `json:"channel_ids,omitempty"`
	Config                  *map[string]any `json:"config,omitempty"`
}

type AlertDeliverySummary struct {
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

type Alert struct {
	ID                      string               `json:"id"`
	MonitorID               string               `json:"monitor_id"`
	Node                    NodeRef              `json:"node"`
	Status                  string               `json:"status"`
	Severity                string               `json:"severity"`
	MonitorName             string               `json:"monitor_name"`
	MonitorKind             string               `json:"monitor_kind"`
	MonitorConfig           map[string]any       `json:"monitor_config"`
	EvaluationWindowSeconds int                  `json:"evaluation_window_seconds"`
	FiringValue             map[string]any       `json:"firing_value,omitempty"`
	RecoveryValue           map[string]any       `json:"recovery_value,omitempty"`
	StartedAt               string               `json:"started_at"`
	EndedAt                 string               `json:"ended_at,omitempty"`
	LastEvaluatedAt         string               `json:"last_evaluated_at"`
	ResolutionReason        string               `json:"resolution_reason,omitempty"`
	DurationSeconds         int64                `json:"duration_seconds"`
	DeliveryChannelCount    int                  `json:"delivery_channel_count"`
	Deliveries              AlertDeliverySummary `json:"deliveries"`
}

type AlertListResponse struct {
	Items   []Alert `json:"items"`
	Total   int64   `json:"total"`
	Page    int     `json:"page"`
	PerPage int     `json:"per_page"`
}

type AlertSummaryResponse struct {
	Total    int64   `json:"total"`
	Warning  int64   `json:"warning"`
	Critical int64   `json:"critical"`
	Items    []Alert `json:"items"`
}

// ── User ──────────────────────────────────────────────────────────────────────

// PanelUser is the public representation returned by user endpoints.
type PanelUser struct {
	ID                string             `json:"id"`
	Email             string             `json:"email"`
	Role              string             `json:"role"` // "admin" | "user"
	AuthString        string             `json:"auth_string"`
	QuotaBytes        int64              `json:"quota_bytes"`
	UsedTx            int64              `json:"used_tx"`
	UsedRx            int64              `json:"used_rx"`
	Status            string             `json:"status"` // "active" | "disabled"
	Created           string             `json:"created"`
	LastConnectedAt   string             `json:"last_connected_at"`
	RecentConnections []RecentConnection `json:"recent_connections"`
}

// UserDetail is returned by GET /users/{id}. OnlineDevices is the sum of the
// latest known counts across the enabled nodes visible to the user.
type UserDetail struct {
	PanelUser
	OnlineDevices *int64 `json:"online_devices"`
}

// RecentConnection is one recent successful Hysteria auth source for a user.
type RecentConnection struct {
	IP         string  `json:"ip"`
	LastSeenAt string  `json:"last_seen_at"`
	IPMeta     *IPMeta `json:"ip_meta,omitempty"`
}

// UserListResponse is the paginated response for GET /users.
type UserListResponse struct {
	Items   []PanelUser `json:"items"`
	Total   int64       `json:"total"`
	Page    int         `json:"page"`
	PerPage int         `json:"per_page"`
}

// UserStatsResponse is the aggregate user count for GET /users/stats.
type UserStatsResponse struct {
	Total  int64 `json:"total"`
	Active int64 `json:"active"`
}

// UserCreateRequest is the body for POST /users. Only email is required; an
// admin can quick-create by email alone. Password and auth_string are
// system-generated when omitted, and the account is always created verified.
type UserCreateRequest struct {
	Email      string  `json:"email"`
	Password   *string `json:"password,omitempty"`
	AuthString *string `json:"auth_string,omitempty"`
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

// ── Registration ───────────────────────────────────────────────────────────────

// RegisterRequest is the body for the public POST /register endpoint.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Code     string `json:"code,omitempty"`
}

// RegisterResponse covers both registration outcomes: a code-backed signup is
// auto-logged-in (token + record), while the open no-code path returns only
// requires_verification=true and sends a verification email.
type RegisterResponse struct {
	Token                string     `json:"token,omitempty"`
	Record               *PanelUser `json:"record,omitempty"`
	RequiresVerification bool       `json:"requires_verification"`
	VerificationSent     bool       `json:"verification_sent,omitempty"`
}

// ── Invitations ────────────────────────────────────────────────────────────────

// Invitation is the admin-facing representation of an invite code.
type Invitation struct {
	ID            string `json:"id"`
	Code          string `json:"code"`
	Email         string `json:"email"`
	MaxUses       int    `json:"max_uses"` // 0 = unlimited
	UsedCount     int    `json:"used_count"`
	ExpiresAt     string `json:"expires_at"`
	Revoked       bool   `json:"revoked"`
	Note          string `json:"note"`
	LastUsedAt    string `json:"last_used_at"`
	Created       string `json:"created"`
	Valid         bool   `json:"valid"`
	InvalidReason string `json:"invalid_reason,omitempty"` // "revoked" | "expired" | "exhausted"
	Link          string `json:"link"`
	EmailSent     *bool  `json:"email_sent,omitempty"` // set only on create when send was attempted
}

// InvitationCreateRequest is the body for POST /invitations.
type InvitationCreateRequest struct {
	Email          *string `json:"email,omitempty"`
	MaxUses        *int    `json:"max_uses,omitempty"`
	ExpiresInHours *int    `json:"expires_in_hours,omitempty"`
	Note           *string `json:"note,omitempty"`
	SendEmail      *bool   `json:"send_email,omitempty"`
}

// IgnoredConnectionIP is a globally ignored client IP for recent-connections tracking.
type IgnoredConnectionIP struct {
	ID      string `json:"id"`
	IP      string `json:"ip"`
	Created string `json:"created"`
}

// IgnoredConnectionIPCreateRequest is the body for POST /ignored-connection-ips.
type IgnoredConnectionIPCreateRequest struct {
	IP string `json:"ip"`
}

// ── App settings ───────────────────────────────────────────────────────────────

// SettingsResponse is returned by GET/PATCH /settings.
type SettingsResponse struct {
	InvitationsEnabled   bool `json:"invitations_enabled"`
	OpenRegistration     bool `json:"open_registration"`
	RequireInviteForOpen bool `json:"require_invite_for_open"`
	// ManagementAPIEnabled controls the /api/mgmt/* surface. The token hash is
	// never returned; ManagementAPITokenSet only signals whether a token exists.
	ManagementAPIEnabled  bool `json:"management_api_enabled"`
	ManagementAPITokenSet bool `json:"management_api_token_set"`
	// ManagementAPIToken carries the plaintext token exactly once, only when a
	// token was just generated or rotated in this response. It is never
	// populated by a plain GET.
	ManagementAPIToken string `json:"management_api_token,omitempty"`
}

// SettingsUpdateRequest is the body for PATCH /settings (all fields optional).
// The Management API token is always server-generated; use the dedicated
// rotate endpoint to replace it.
type SettingsUpdateRequest struct {
	InvitationsEnabled   *bool `json:"invitations_enabled,omitempty"`
	OpenRegistration     *bool `json:"open_registration,omitempty"`
	RequireInviteForOpen *bool `json:"require_invite_for_open,omitempty"`
	ManagementAPIEnabled *bool `json:"management_api_enabled,omitempty"`
}

// ManagementAPITokenResponse is returned by POST /management-api/rotate.
// The plaintext token is shown exactly once.
type ManagementAPITokenResponse struct {
	ManagementAPIToken string `json:"management_api_token"`
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

// TrafficSummaryResponse is returned by GET /users/{id}/traffic/summary.
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
	ID      string `json:"id"`
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
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

// NodeTrafficSummaryResponse is returned by GET /nodes/{id}/traffic/summary.
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
	ActiveStreams int              `json:"active_streams"`
	ByNode        []LiveNodeResult `json:"by_node"`
	TopDomains    []TopDomain      `json:"top_domains"`
	ByConnection  []ConnSummary    `json:"by_connection"`
}

// LiveNodeResult is the per-node breakdown inside the live response.
type LiveNodeResult struct {
	Node    NodeRef      `json:"node"`
	Streams []LiveStream `json:"streams"`
	Error   string       `json:"error,omitempty"`
}

// NodeLiveResponse is returned by GET /nodes/{id}/live. Unlike the user live
// response it covers a single node and every user on it (no auth filter).
type NodeLiveResponse struct {
	ActiveStreams int                  `json:"active_streams"`
	ByUser        []NodeLiveUserResult `json:"by_user"`
	TopDomains    []TopDomain          `json:"top_domains"`
	ByConnection  []ConnSummary        `json:"by_connection"`
	Error         string               `json:"error,omitempty"`
}

// NodeLiveUserResult groups a node's live streams under one panel user. Streams
// whose auth string matches no panel user are grouped under an "unknown" entry.
type NodeLiveUserResult struct {
	User    UserRef      `json:"user"`
	Streams []LiveStream `json:"streams"`
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
	// Registration flags are read live from app_settings (runtime-mutable) so
	// the login/register pages can render the right entry points.
	RegistrationOpen          bool `json:"registration_open"`
	RegistrationRequireInvite bool `json:"registration_require_invite"`
	InvitationsEnabled        bool `json:"invitations_enabled"`
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
