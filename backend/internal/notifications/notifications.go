// Package notifications validates and sends the panel's supported Shoutrrr
// notification-channel URLs. It deliberately returns only typed, URL-safe
// errors so callers never expose credentials embedded in service URLs.
package notifications

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/nicholas-fedor/shoutrrr"
	"github.com/nicholas-fedor/shoutrrr/pkg/router"
	"github.com/nicholas-fedor/shoutrrr/pkg/services/chat/matrix"
	"github.com/nicholas-fedor/shoutrrr/pkg/types"
)

// Service is the canonical, non-secret notification service identifier stored
// with a channel. The allowlist intentionally matches Beszel's Shoutrrr guide.
type Service string

const (
	ServiceGeneric    Service = "generic"
	ServiceBark       Service = "bark"
	ServiceDiscord    Service = "discord"
	ServiceGotify     Service = "gotify"
	ServiceGoogleChat Service = "googlechat"
	ServiceIFTTT      Service = "ifttt"
	ServiceJoin       Service = "join"
	ServiceLark       Service = "lark"
	ServiceMattermost Service = "mattermost"
	ServiceMatrix     Service = "matrix"
	ServiceMQTT       Service = "mqtt"
	ServiceNtfy       Service = "ntfy"
	ServiceOpsgenie   Service = "opsgenie"
	ServicePushbullet Service = "pushbullet"
	ServicePushover   Service = "pushover"
	ServiceRocketChat Service = "rocketchat"
	ServiceSignal     Service = "signal"
	ServiceSlack      Service = "slack"
	ServiceTeams      Service = "teams"
	ServiceTelegram   Service = "telegram"
	ServiceTwilio     Service = "twilio"
	ServiceWeCom      Service = "wecom"
	ServiceZulip      Service = "zulip"
)

var (
	// ErrInvalidURL is intentionally generic: Shoutrrr errors can reproduce a
	// full service URL containing credentials.
	ErrInvalidURL = errors.New("invalid notification channel URL")
	// ErrUnsupportedService identifies a syntactically valid URL whose service
	// is not in the panel's Beszel-compatible allowlist.
	ErrUnsupportedService = errors.New("unsupported notification service")

	errServiceTimeout  = router.ErrServiceTimeout
	errDeliveryFailure = errors.New("delivery failed")
)

// ErrorCode is the persisted, safe outcome of an explicit channel test.
type ErrorCode string

const (
	ErrorTimedOut       ErrorCode = "timed_out"
	ErrorDeliveryFailed ErrorCode = "delivery_failed"
)

// Result describes the delivery outcome without carrying the provider error.
type Result struct {
	Succeeded bool
	ErrorCode ErrorCode
}

// Client is the adapter used by the panel API. It is stateless so a fresh
// Shoutrrr sender is created for each validation or single-channel test.
type Client struct{}

// New returns the production notification delivery adapter.
func New() *Client {
	return &Client{}
}

// Validate is the package-level convenience entry point used when only local
// URL validation is needed.
func Validate(rawURL string) (Service, error) {
	return New().Validate(rawURL)
}

// Validate accepts one complete Shoutrrr URL, applies the panel's explicit
// service allowlist, and initializes the service without sending a message.
func (c *Client) Validate(rawURL string) (Service, error) {
	_ = c
	service, err := serviceForURL(rawURL)
	if err != nil {
		return "", err
	}
	// Matrix's Service.Initialize performs a login for non-dummy hosts. Use
	// its URL configuration parser directly so saving a Channel is always a
	// local validation step and never becomes an implicit delivery attempt.
	if service == ServiceMatrix {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return "", ErrInvalidURL
		}
		var config matrix.Config
		if err := config.SetURL(parsed); err != nil {
			return "", ErrInvalidURL
		}
		return service, nil
	}
	if _, err := shoutrrr.CreateSender(rawURL); err != nil {
		return "", ErrInvalidURL
	}
	return service, nil
}

// Send delivers one fixed test message. Callers should validate before saving,
// but Send validates again so stored or corrupted data cannot bypass the
// allowlist. Provider errors are classified rather than returned verbatim.
func (c *Client) Send(rawURL, message string) Result {
	_ = c
	if _, err := serviceForURL(rawURL); err != nil {
		return Result{ErrorCode: ErrorDeliveryFailed}
	}
	sender, err := shoutrrr.CreateSender(rawURL)
	if err != nil {
		return Result{ErrorCode: ErrorDeliveryFailed}
	}
	for _, sendErr := range sender.Send(message, &types.Params{}) {
		if sendErr != nil {
			return Result{ErrorCode: classifySendError(sendErr)}
		}
	}
	return Result{Succeeded: true}
}

func serviceForURL(rawURL string) (Service, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || strings.ContainsAny(rawURL, "\r\n") {
		return "", ErrInvalidURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" {
		return "", ErrInvalidURL
	}
	scheme := strings.ToLower(parsed.Scheme)
	if strings.HasPrefix(scheme, "generic+") {
		scheme = string(ServiceGeneric)
	}
	if scheme == "mqtts" {
		scheme = string(ServiceMQTT)
	}
	service := Service(scheme)
	if !isAllowed(service) {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedService, service)
	}
	return service, nil
}

func isAllowed(service Service) bool {
	switch service {
	case ServiceGeneric, ServiceBark, ServiceDiscord, ServiceGotify, ServiceGoogleChat,
		ServiceIFTTT, ServiceJoin, ServiceLark, ServiceMattermost, ServiceMatrix,
		ServiceMQTT, ServiceNtfy, ServiceOpsgenie, ServicePushbullet, ServicePushover,
		ServiceRocketChat, ServiceSignal, ServiceSlack, ServiceTeams, ServiceTelegram,
		ServiceTwilio, ServiceWeCom, ServiceZulip:
		return true
	default:
		return false
	}
}

func classifySendError(err error) ErrorCode {
	if errors.Is(err, errServiceTimeout) {
		return ErrorTimedOut
	}
	return ErrorDeliveryFailed
}
