package announcements

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

// Record ids are 15 lowercase alphanumerics. PocketBase stores names as a
// snake case stem plus an extension that may hold * - + = #, which arrive
// percent-encoded. The stem never starts with a dot, so ".." cannot match.
const storedFilePattern = `/api/files/files/[a-z0-9]{15}/[A-Za-z0-9_-]+(\.[A-Za-z0-9_*+=%-]+)*`

var (
	imageSourcePattern = regexp.MustCompile(`^` + storedFilePattern + `$`)
	linkTargetPattern  = regexp.MustCompile(`^(?:https?://|mailto:|` + storedFilePattern + `(?:\?download=1)?$)`)
	codeClassPattern   = regexp.MustCompile(`^language-[A-Za-z0-9_+-]+$`)
	// The editor stores a resized image as a whole pixel width.
	imageWidthPattern = regexp.MustCompile(`^[1-9][0-9]{0,3}$`)
)

// bodyPolicy mirrors what the announcement editor can produce. A bluemonday
// policy is safe for concurrent use once built.
var bodyPolicy = newBodyPolicy()

func newBodyPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements(
		"p", "br", "h2", "h3", "strong", "em", "s", "code", "pre",
		"blockquote", "ul", "ol", "li", "hr", "a", "img",
	)
	p.AllowStandardURLs()
	p.AllowAttrs("href").Matching(linkTargetPattern).OnElements("a")
	p.AllowAttrs("download").OnElements("a")
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.AllowAttrs("src").Matching(imageSourcePattern).OnElements("img")
	p.AllowAttrs("alt", "title").OnElements("img")
	p.AllowAttrs("width").Matching(imageWidthPattern).OnElements("img")
	p.AllowAttrs("start").Matching(bluemonday.Integer).OnElements("ol")
	p.AllowAttrs("class").Matching(codeClassPattern).OnElements("code")
	return p
}

// SanitizeHTML keeps only the markup the editor produces. Unknown elements
// lose their tags but keep their text.
func SanitizeHTML(raw string) string {
	return bodyPolicy.Sanitize(raw)
}

// hasVisibleContent reports whether sanitized HTML shows any text or image.
func hasVisibleContent(clean string) bool {
	tokens := html.NewTokenizer(strings.NewReader(clean))
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			return false
		case html.TextToken:
			if strings.TrimFunc(string(tokens.Text()), unicode.IsSpace) != "" {
				return true
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			if isImageWithSource(tokens) {
				return true
			}
		}
	}
}

func isImageWithSource(tokens *html.Tokenizer) bool {
	name, hasAttr := tokens.TagName()
	if string(name) != "img" {
		return false
	}
	for hasAttr {
		var key, value []byte
		key, value, hasAttr = tokens.TagAttr()
		if string(key) == "src" && len(value) > 0 {
			return true
		}
	}
	return false
}
