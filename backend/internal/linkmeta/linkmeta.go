// Package linkmeta fetches the title and favicon of a page for resource links.
package linkmeta

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

const (
	// MaxTitleRunes bounds the extracted title length.
	MaxTitleRunes = 200

	maxPageBytes = 512 << 10
	maxIconBytes = 100 << 10
	userAgent    = "Mozilla/5.0 (compatible; HystericalPanel; +resource-links)"
)

// Metadata describes a page's link preview. Missing fields are zero values.
type Metadata struct {
	Title    string
	Icon     []byte
	IconType string
}

var client = &http.Client{
	CheckRedirect: func(_ *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return errors.New("too many redirects")
		}
		return nil
	},
}

// Fetch reads page metadata. Failures yield zero fields; the caller owns the
// deadline through ctx.
func Fetch(ctx context.Context, pageURL string) Metadata {
	resp, err := get(ctx, pageURL)
	if err != nil {
		return Metadata{}
	}
	defer resp.Body.Close()

	var meta Metadata
	var iconHref string
	// An error page's title names the error, not the site, so only a 2xx
	// page is parsed. /favicon.ico is still tried either way.
	if contentType := resp.Header.Get("Content-Type"); isSuccess(resp.StatusCode) && isHTML(contentType) {
		meta.Title, iconHref = parseHead(resp.Body, contentType)
	}
	meta.Icon, meta.IconType = fetchIcon(ctx, iconCandidates(resp.Request.URL, iconHref))
	return meta
}

func get(ctx context.Context, target string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	return client.Do(req)
}

func isSuccess(status int) bool {
	return status >= 200 && status <= 299
}

func isHTML(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}

func parseHead(body io.Reader, contentType string) (string, string) {
	reader, err := charset.NewReader(io.LimitReader(body, maxPageBytes), contentType)
	if err != nil {
		return "", ""
	}
	z := html.NewTokenizer(reader)
	var facts headFacts
	for {
		tt := z.Next()
		if tt == html.ErrorToken || !facts.consume(z, tt) {
			break
		}
	}
	return chooseTitle(facts.ogTitle, facts.title), facts.iconHref
}

type headFacts struct {
	title    string
	ogTitle  string
	iconHref string
}

// consume reads one tag token and reports whether tokenizing should continue.
func (f *headFacts) consume(z *html.Tokenizer, tt html.TokenType) bool {
	switch tt {
	case html.StartTagToken, html.SelfClosingTagToken:
	case html.EndTagToken:
		return z.Token().Data != "head"
	default:
		return true
	}
	tok := z.Token()
	switch tok.Data {
	case "body":
		return false
	case "title":
		if tt == html.StartTagToken {
			f.title = readTitle(z)
		}
	case "meta":
		if f.ogTitle == "" {
			f.ogTitle = ogTitleFrom(tok)
		}
	case "link":
		if f.iconHref == "" && isIconLink(tok) {
			f.iconHref = attrValue(tok, "href")
		}
	}
	return true
}

func readTitle(z *html.Tokenizer) string {
	var b strings.Builder
	for {
		switch z.Next() {
		case html.ErrorToken:
			return b.String()
		case html.TextToken:
			b.Write(z.Text())
		case html.EndTagToken:
			if tok := z.Token(); tok.Data == "title" {
				return b.String()
			}
		}
	}
}

func chooseTitle(ogTitle, title string) string {
	if normalized := normalizeTitle(ogTitle); normalized != "" {
		return normalized
	}
	return normalizeTitle(title)
}

func normalizeTitle(title string) string {
	title = strings.Join(strings.Fields(title), " ")
	if utf8.RuneCountInString(title) <= MaxTitleRunes {
		return title
	}
	return string([]rune(title)[:MaxTitleRunes])
}

func ogTitleFrom(tok html.Token) string {
	if strings.EqualFold(attrValue(tok, "property"), "og:title") {
		return attrValue(tok, "content")
	}
	return ""
}

func isIconLink(tok html.Token) bool {
	for _, rel := range strings.Fields(attrValue(tok, "rel")) {
		if strings.EqualFold(rel, "icon") {
			return true
		}
	}
	return false
}

func attrValue(tok html.Token, key string) string {
	for _, attr := range tok.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func iconCandidates(base *url.URL, href string) []string {
	candidates := make([]string, 0, 2)
	if href != "" {
		if ref, err := url.Parse(href); err == nil {
			resolved := base.ResolveReference(ref)
			if resolved.Scheme == "http" || resolved.Scheme == "https" {
				candidates = append(candidates, resolved.String())
			}
		}
	}
	return append(candidates, base.Scheme+"://"+base.Host+"/favicon.ico")
}

func fetchIcon(ctx context.Context, candidates []string) ([]byte, string) {
	for _, candidate := range candidates {
		data, iconType, ok := fetchIconAt(ctx, candidate)
		if ok {
			return data, iconType
		}
	}
	return nil, ""
}

func fetchIconAt(ctx context.Context, target string) ([]byte, string, bool) {
	resp, err := get(ctx, target)
	if err != nil {
		return nil, "", false
	}
	defer resp.Body.Close()
	if !isSuccess(resp.StatusCode) {
		return nil, "", false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxIconBytes+1))
	if err != nil || len(data) > maxIconBytes {
		return nil, "", false
	}
	iconType, ok := imageType(resp.Header.Get("Content-Type"), data)
	return data, iconType, ok
}

func imageType(contentType string, data []byte) (string, bool) {
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil && strings.HasPrefix(mediaType, "image/") {
		return mediaType, true
	}
	sniffed := http.DetectContentType(data)
	if strings.HasPrefix(sniffed, "image/") {
		return sniffed, true
	}
	return "", false
}
