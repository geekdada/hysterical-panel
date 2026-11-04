package automaticemails

import (
	"errors"
	"html"
	"math/big"
	"regexp"
	"strings"
	"unicode/utf8"

	"hysterical-panel/internal/bytesize"
	"hysterical-panel/internal/sendouts"
	"hysterical-panel/internal/subscriptions"
)

var (
	ErrUnknownPlaceholder = errors.New("unknown placeholder")
	// A placeholder in HTML must appear in Text too, and unbroken by markup.
	ErrBrokenPlaceholder  = errors.New("placeholders in html and text do not match")
	ErrEncodedPlaceholder = errors.New("placeholder is url-encoded in html")
)

type Template struct{ Subject, HTML, Text string }

const timeLayout = "2006-01-02 15:04 UTC"

// Names match case-insensitively because plain-text conversion uppercases headings.
var placeholderPattern = regexp.MustCompile(`\{\{([A-Za-z_]+)\}\}`)

var knownPlaceholders = map[string]bool{
	"email": true, "remaining": true, "allowance": true, "used": true,
	"remaining_percent": true, "window_starts_at": true, "window_ends_at": true,
	"subscription_ends_at": true, "panel_url": true,
}

func placeholderNames(s string) map[string]bool {
	names := map[string]bool{}
	for _, match := range placeholderPattern.FindAllStringSubmatch(s, -1) {
		names[strings.ToLower(match[1])] = true
	}
	return names
}

func ValidateTemplate(t Template) error {
	htmlNames, textNames := placeholderNames(t.HTML), placeholderNames(t.Text)
	for _, names := range []map[string]bool{placeholderNames(t.Subject), htmlNames, textNames} {
		for name := range names {
			if !knownPlaceholders[name] {
				return ErrUnknownPlaceholder
			}
		}
	}
	if strings.Contains(strings.ToLower(t.HTML), "%7b%7b") {
		return ErrEncodedPlaceholder
	}
	if len(htmlNames) != len(textNames) {
		return ErrBrokenPlaceholder
	}
	for name := range htmlNames {
		if !textNames[name] {
			return ErrBrokenPlaceholder
		}
	}
	return nil
}

func Variables(email string, state *subscriptions.State, panelURL string) map[string]string {
	start := state.Grant.GetDateTime("starts_at").Time()
	end := state.Grant.GetDateTime("ends_at").Time()
	windowStart := start
	if state.Window > 0 {
		windowStart = subscriptions.WindowEnd(start, end, state.Window-1, state.Type.GetInt("reset_days"))
	}
	return map[string]string{
		"email":                email,
		"remaining":            bytesize.Format(state.Remaining),
		"allowance":            bytesize.Format(state.Allowance),
		"used":                 bytesize.Format(state.Used.Total()),
		"remaining_percent":    remainingPercent(state.Remaining, state.Allowance),
		"window_starts_at":     windowStart.UTC().Format(timeLayout),
		"window_ends_at":       state.WindowEndsAt().UTC().Format(timeLayout),
		"subscription_ends_at": end.UTC().Format(timeLayout),
		"panel_url":            panelURL,
	}
}

// remainingPercent uses big integers because remaining*100 can overflow int64.
func remainingPercent(remaining, allowance int64) string {
	if allowance <= 0 || remaining <= 0 {
		return "0%"
	}
	percent := new(big.Int).Mul(big.NewInt(remaining), big.NewInt(100))
	percent.Quo(percent, big.NewInt(allowance))
	return percent.String() + "%"
}

// Render substitutes known placeholders. Unknown ones are left untouched.
func Render(t Template, vars map[string]string) Template {
	return Template{
		Subject: truncateRunes(substitute(t.Subject, vars, false), sendouts.MaxSubjectRunes),
		HTML:    substitute(t.HTML, vars, true),
		Text:    substitute(t.Text, vars, false),
	}
}

// truncateRunes keeps the subject within the sendout limit once values are filled in.
func truncateRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit])
}

func substitute(s string, vars map[string]string, escape bool) string {
	return placeholderPattern.ReplaceAllStringFunc(s, func(match string) string {
		value, ok := vars[strings.ToLower(placeholderPattern.FindStringSubmatch(match)[1])]
		if !ok {
			return match
		}
		if escape {
			return html.EscapeString(value)
		}
		return value
	})
}
