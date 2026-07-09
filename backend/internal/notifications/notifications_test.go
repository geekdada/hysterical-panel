package notifications

import (
	"strings"
	"testing"
)

func TestValidateAcceptsBeszelGenericURL(t *testing.T) {
	service, err := Validate("generic://example.test?template=json")
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if service != ServiceGeneric {
		t.Fatalf("Validate() service = %q, want %q", service, ServiceGeneric)
	}
}

func TestValidateAcceptsEveryBeszelService(t *testing.T) {
	tests := []struct {
		service Service
		url     string
	}{
		{ServiceGeneric, "generic://example.test?template=json"},
		{ServiceBark, "bark://:device-key@bark.example.test"},
		{ServiceDiscord, "discord://token@123456"},
		{ServiceGotify, "gotify://gotify.example.test/Aaa.bbb.ccc.ddd"},
		{ServiceGoogleChat, "googlechat://chat.googleapis.com/v1/spaces/space/messages?key=key&token=token"},
		{ServiceIFTTT, "ifttt://key/?events=event"},
		{ServiceJoin, "join://shoutrrr:key@join/?devices=device"},
		{ServiceLark, "lark://open.larksuite.com/token?secret=secret"},
		{ServiceMattermost, "mattermost://mattermost.example.test/token"},
		{ServiceMatrix, "matrix://testuser:testpass@matrix.example.test?room=testroom"},
		{ServiceMQTT, "mqtt://broker.example.test/topic"},
		{ServiceNtfy, "ntfy://ntfy.example.test/topic"},
		{ServiceOpsgenie, "opsgenie://api.opsgenie.com/key"},
		{ServicePushbullet, "pushbullet://tokentokentokentokentokentokentoke"},
		{ServicePushover, "pushover://shoutrrr:token@user/"},
		{ServiceRocketChat, "rocketchat://rocketchat.example.test/token-a/token-b"},
		{ServiceSignal, "signal://signal.example.test/+1234567890/+0987654321"},
		{ServiceSlack, "slack://hook:AAAAAAAAA-BBBBBBBBB-123456789123456789123456@webhook"},
		{ServiceTeams, "teams://?host=https%3A%2F%2Fexample.test%2Fworkflow"},
		{ServiceTelegram, "telegram://12345:mock-token@telegram/?chats=123"},
		{ServiceTwilio, "twilio://sid:token@+1234567890/+1987654321"},
		{ServiceWeCom, "wecom://key"},
		{ServiceZulip, "zulip://bot%40example.test:key@zulip.example.test?stream=alerts"},
	}

	for _, tt := range tests {
		t.Run(string(tt.service), func(t *testing.T) {
			service, err := Validate(tt.url)
			if err != nil {
				t.Fatalf("Validate(%q) error = %v", tt.url, err)
			}
			if service != tt.service {
				t.Fatalf("Validate(%q) = %q, want %q", tt.url, service, tt.service)
			}
		})
	}
}

func TestValidateNormalizesCompatibleSchemes(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want Service
	}{
		{
			name: "generic HTTPS",
			url:  "generic+https://example.test/webhook?template=json",
			want: ServiceGeneric,
		},
		{
			name: "MQTT over TLS",
			url:  "mqtts://user:password@example.test/topic",
			want: ServiceMQTT,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Validate(tt.url)
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Validate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateRejectsUnsupportedAndMalformedURLsWithoutEchoingThem(t *testing.T) {
	tests := []string{
		"smtp://user:super-secret@example.test:587/?toaddresses=recipient@example.test",
		"pagerduty://super-secret@pagerduty.example.test",
		"notifiarr://super-secret@notifiarr.example.test",
		"logger://super-secret",
		"xmpp://super-secret@example.test",
		"not a URL with secret=super-secret",
	}

	for _, rawURL := range tests {
		t.Run(rawURL[:4], func(t *testing.T) {
			_, err := Validate(rawURL)
			if err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
			if strings.Contains(err.Error(), "super-secret") {
				t.Fatalf("Validate() error leaked a URL secret: %q", err)
			}
		})
	}
}

func TestClassifySendError(t *testing.T) {
	if got := classifySendError(errServiceTimeout); got != ErrorTimedOut {
		t.Fatalf("classifySendError(timeout) = %q, want %q", got, ErrorTimedOut)
	}
	if got := classifySendError(errDeliveryFailure); got != ErrorDeliveryFailed {
		t.Fatalf("classifySendError(failure) = %q, want %q", got, ErrorDeliveryFailed)
	}
}
