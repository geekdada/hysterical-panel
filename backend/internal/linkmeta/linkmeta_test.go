package linkmeta

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

var (
	icoBytes = []byte("\x00\x00\x01\x00\x10\x00\x10\x00\x00\x00\x01\x00\x20\x00")
	pngBytes = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
)

func htmlPage(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, body)
	}
}

func serveHTML(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(htmlPage(body))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchPrefersOGTitle(t *testing.T) {
	srv := serveHTML(t, `<html><head><title>Plain</title><meta property="og:title" content="Rock &amp; Roll"></head></html>`)

	meta := Fetch(context.Background(), srv.URL+"/")

	if meta.Title != "Rock & Roll" {
		t.Fatalf("Title = %q, want %q", meta.Title, "Rock & Roll")
	}
}

func TestFetchFallsBackToTitleTag(t *testing.T) {
	srv := serveHTML(t, "<html><head><title>\n  Hello\n  World </title></head></html>")

	meta := Fetch(context.Background(), srv.URL+"/")

	if meta.Title != "Hello World" {
		t.Fatalf("Title = %q, want %q", meta.Title, "Hello World")
	}
}

func TestFetchTruncatesLongTitle(t *testing.T) {
	srv := serveHTML(t, "<html><head><title>"+strings.Repeat("你", 300)+"</title></head></html>")

	meta := Fetch(context.Background(), srv.URL+"/")

	if got := utf8.RuneCountInString(meta.Title); got != MaxTitleRunes {
		t.Fatalf("rune count = %d, want %d", got, MaxTitleRunes)
	}
}

func TestFetchDecodesDeclaredCharset(t *testing.T) {
	title, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("你好"))
	if err != nil {
		t.Fatalf("encode title: %v", err)
	}
	page := append([]byte(`<html><head><meta charset="gbk"><title>`), title...)
	page = append(page, []byte(`</title></head></html>`)...)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(page)
	}))
	t.Cleanup(srv.Close)

	meta := Fetch(context.Background(), srv.URL+"/")

	if meta.Title != "你好" {
		t.Fatalf("Title = %q, want %q", meta.Title, "你好")
	}
}

func TestFetchUsesLinkRelIcon(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", htmlPage(`<html><head><link rel="shortcut icon" href="/i.png"></head></html>`))
	mux.HandleFunc("/i.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	meta := Fetch(context.Background(), srv.URL+"/")

	if meta.IconType != "image/png" {
		t.Fatalf("IconType = %q, want %q", meta.IconType, "image/png")
	}
	if !bytes.Equal(meta.Icon, pngBytes) {
		t.Fatalf("Icon = %v, want %v", meta.Icon, pngBytes)
	}
}

func TestFetchResolvesIconAgainstFinalURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dir/page", http.StatusFound)
	})
	mux.HandleFunc("/dir/page", htmlPage(`<html><head><link rel="icon" href="icon.svg"></head></html>`))
	mux.HandleFunc("/dir/icon.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	meta := Fetch(context.Background(), srv.URL+"/start")

	if meta.IconType != "image/svg+xml" {
		t.Fatalf("IconType = %q, want %q", meta.IconType, "image/svg+xml")
	}
}

func TestFetchFallsBackToFaviconICO(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", htmlPage(`<html><head><title>No icon link</title></head></html>`))
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/x-icon")
		w.Write(icoBytes)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	meta := Fetch(context.Background(), srv.URL+"/")

	if meta.Icon == nil {
		t.Fatal("Icon = nil, want favicon bytes")
	}
}

func TestFetchSniffsUntypedIcon(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", htmlPage(`<html><head><title>No icon link</title></head></html>`))
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(icoBytes)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	meta := Fetch(context.Background(), srv.URL+"/")

	if meta.IconType != "image/x-icon" {
		t.Fatalf("IconType = %q, want %q", meta.IconType, "image/x-icon")
	}
}

func TestFetchRejectsNonImageIcon(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", htmlPage(`<html><head><title>No icon link</title></head></html>`))
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<html>not an icon</html>")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	meta := Fetch(context.Background(), srv.URL+"/")

	if meta.Icon != nil || meta.IconType != "" {
		t.Fatalf("Icon = %v, IconType = %q, want both empty", meta.Icon, meta.IconType)
	}
}

func TestFetchRejectsOversizedIcon(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", htmlPage(`<html><head><title>No icon link</title></head></html>`))
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(bytes.Repeat([]byte{0}, (100<<10)+1))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	meta := Fetch(context.Background(), srv.URL+"/")

	if meta.Icon != nil {
		t.Fatalf("Icon length = %d, want nil", len(meta.Icon))
	}
}

func TestFetchStopsAfterThreeRedirects(t *testing.T) {
	mux := http.NewServeMux()
	for i := 0; i < 4; i++ {
		next := fmt.Sprintf("/%d", i+1)
		mux.HandleFunc(fmt.Sprintf("/%d", i), func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, next, http.StatusFound)
		})
	}
	mux.HandleFunc("/4", htmlPage(`<html><head><title>Too far</title></head></html>`))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	meta := Fetch(context.Background(), srv.URL+"/0")

	if meta.Title != "" {
		t.Fatalf("Title = %q, want empty", meta.Title)
	}
}

func TestFetchNonHTMLStillFindsFavicon(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		io.WriteString(w, "%PDF-1.4")
	})
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/x-icon")
		w.Write(icoBytes)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	meta := Fetch(context.Background(), srv.URL+"/")

	if meta.Title != "" {
		t.Fatalf("Title = %q, want empty", meta.Title)
	}
	if meta.Icon == nil {
		t.Fatal("Icon = nil, want favicon from /favicon.ico")
	}
}

func TestFetchHonoursContextDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	meta := Fetch(ctx, srv.URL+"/")
	elapsed := time.Since(start)

	if elapsed >= time.Second {
		t.Fatalf("Fetch took %v, want under 1s", elapsed)
	}
	if meta.Title != "" || meta.Icon != nil || meta.IconType != "" {
		t.Fatalf("Metadata = %+v, want zero value", meta)
	}
}
