package announcements

import (
	"strings"
	"testing"
)

const storedImage = "/api/files/files/abcdefghij12345/diagram_k3j2h1g0f9.png"

func TestSanitizeHTMLKeepsEditorMarkup(t *testing.T) {
	in := `<h2>Title</h2><p><strong>b</strong> <em>i</em> <s>s</s> <code>c</code></p>` +
		`<pre><code class="language-go">x</code></pre><blockquote><p>q</p></blockquote>` +
		`<ol start="3"><li>a</li></ol><ul><li>b</li></ul><hr>` +
		`<img src="` + storedImage + `" alt="diagram">`
	got := SanitizeHTML(in)
	for _, want := range []string{
		"<h2>", "<strong>", "<em>", "<s>", `<code class="language-go">`, "<blockquote>",
		`<ol start="3">`, "<ul>", "<hr", `src="` + storedImage + `"`, `alt="diagram"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("sanitized output lost %q: %s", want, got)
		}
	}
}

func TestSanitizeHTMLRemovesScripts(t *testing.T) {
	cases := map[string]string{
		"script element":  `<p>ok</p><script>alert(1)</script>`,
		"event handler":   `<p onclick="alert(1)">ok</p>`,
		"javascript href": `<a href="javascript:alert(1)">ok</a>`,
		"data image":      `<img src="data:image/svg+xml;base64,PHN2Zz4=">`,
		"iframe":          `<iframe src="https://example.com"></iframe>`,
		"style attribute": `<p style="background:url(x)">ok</p>`,
	}
	for name, in := range cases {
		got := strings.ToLower(SanitizeHTML(in))
		for _, banned := range []string{"<script", "onclick", "javascript:", "data:", "<iframe", "style="} {
			if strings.Contains(got, banned) {
				t.Errorf("%s: output still has %q: %s", name, banned, got)
			}
		}
	}
}

func TestSanitizeHTMLRestrictsImageSources(t *testing.T) {
	cases := []string{
		"https://example.com/a.png",
		"/api/files/files/abcdefghij12345/../../panel/users",
		"/api/files/users/abcdefghij12345/avatar.png",
		"//example.com/a.png",
	}
	for _, src := range cases {
		got := SanitizeHTML(`<img src="` + src + `">`)
		if strings.Contains(got, "src=") {
			t.Errorf("src %q kept: %s", src, got)
		}
	}
}

func TestSanitizeHTMLLinks(t *testing.T) {
	external := SanitizeHTML(`<a href="https://example.com/x">x</a>`)
	for _, want := range []string{`href="https://example.com/x"`, `target="_blank"`, "noopener", "noreferrer"} {
		if !strings.Contains(external, want) {
			t.Errorf("external link lost %q: %s", want, external)
		}
	}

	attachment := SanitizeHTML(`<a href="` + storedImage + `?download=1" download="Diagram.png">Diagram.png</a>`)
	if !strings.Contains(attachment, `href="`+storedImage+`?download=1"`) || !strings.Contains(attachment, `download="Diagram.png"`) {
		t.Errorf("attachment link changed: %s", attachment)
	}

	encodedExt := SanitizeHTML(`<a href="/api/files/files/abcdefghij12345/notes_k3j2h1g0f9.c%2B%2B">notes</a>`)
	if !strings.Contains(encodedExt, `href="/api/files/files/abcdefghij12345/notes_k3j2h1g0f9.c%2B%2B"`) {
		t.Errorf("encoded extension dropped: %s", encodedExt)
	}

	for _, href := range []string{"/settings", "ftp://example.com", "/api/panel/users"} {
		if got := SanitizeHTML(`<a href="` + href + `">x</a>`); strings.Contains(got, "href=") {
			t.Errorf("href %q kept: %s", href, got)
		}
	}
}

func TestSanitizeHTMLKeepsTextOfUnknownElements(t *testing.T) {
	got := SanitizeHTML(`<h1>Big</h1><table><tr><td>cell</td></tr></table>`)
	if strings.Contains(got, "<h1") || strings.Contains(got, "<table") {
		t.Fatalf("unknown elements kept: %s", got)
	}
	if !strings.Contains(got, "Big") || !strings.Contains(got, "cell") {
		t.Fatalf("text dropped: %s", got)
	}
}

func TestHasVisibleContent(t *testing.T) {
	cases := map[string]bool{
		"<p></p>":                         false,
		"<p> &nbsp; </p><p><br></p>":      false,
		"<hr>":                            false,
		"<img alt=\"x\">":                 false,
		"<p>hi</p>":                       true,
		`<img src="` + storedImage + `">`: true,
	}
	for in, want := range cases {
		if got := hasVisibleContent(SanitizeHTML(in)); got != want {
			t.Errorf("hasVisibleContent(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSanitizeHTMLImageWidth(t *testing.T) {
	kept := SanitizeHTML(`<img src="` + storedImage + `" width="320">`)
	if !strings.Contains(kept, `width="320"`) {
		t.Errorf("pixel width dropped: %s", kept)
	}

	for _, width := range []string{"50%", "0", "-1", "12345", "320px", "auto"} {
		got := SanitizeHTML(`<img src="` + storedImage + `" width="` + width + `">`)
		if strings.Contains(got, "width=") {
			t.Errorf("width %q kept: %s", width, got)
		}
	}
	if got := SanitizeHTML(`<p width="320">x</p>`); strings.Contains(got, "width=") {
		t.Errorf("width kept outside img: %s", got)
	}
}
