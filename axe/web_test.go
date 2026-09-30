package axe

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const resultsHTML = `
    <a rel="nofollow" class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Frust-lang.org%2F&amp;rut=abc">The <b>Rust</b> Programming Language</a>
    <a class="result__snippet" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Frust-lang.org%2F&amp;rut=abc"><b>Rust</b> is fast &amp; reliable.</a>
    <a rel="nofollow" class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fa%3Fb%3D1&amp;rut=def">Example</a>
    <a class="result__snippet" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2F">Snippet   with
    newlines.</a>
    `

const articleHTML = `<html><head><title>Axe</title></head><body>
            <nav>menu menu menu</nav>
            <article><h1>Real heading</h1><p>This is the body of the article, long enough to be
            chosen as the main content of the document by the readability scoring pass, which
            needs a reasonable amount of prose to work with.</p>
            <p>Second paragraph with a <a href="https://example.com/x">link</a>.</p></article>
            <footer>footer footer</footer></body></html>`

func optionURL(base, href string) string {
	url, ok := absolutize(base, href)
	if !ok {
		return "None"
	}
	return fmt.Sprintf("Some(%q)", url)
}

func optionAttr(tag, name string) string {
	value, ok := attr(tag, name)
	if !ok {
		return "None"
	}
	return fmt.Sprintf("Some(%q)", value)
}

func optionCharset(contentType string) string {
	value := charset(contentType)
	if value == "" {
		return "None"
	}
	return fmt.Sprintf("Some(%q)", value)
}

func parseDump(text string) string {
	items := parseResults(text)
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, item.url+"|"+item.title+"|"+item.snippet)
	}
	return strings.Join(parts, "\n---\n")
}

func TestParidadWebConRust(t *testing.T) {
	expected := readTestdata(t, "testdata/paridad-web.txt")
	base := "https://example.com/a/b/index.html"
	linksHTML := "<html><body><nav>\n            <a href=\"/docs/a\">A</a><a href=\"/docs/a\">A otra vez</a>\n            <a href=\"#top\">Arriba</a><a href=\"https://x.com/y\">Y</a>\n            </nav><article><p>cuerpo</p></article></body></html>"

	cases := map[string]func() string{
		"parse_results":        func() string { return escapeEdit(parseDump(resultsHTML)) },
		"format_results":       func() string { return escapeEdit(formatResults(parseResults(resultsHTML), defaultResults)) },
		"format_results_one":   func() string { return escapeEdit(formatResults(parseResults(resultsHTML), 1)) },
		"format_results_empty": func() string { return escapeEdit(formatResults(nil, defaultResults)) },
		"page_links": func() string {
			return escapeEdit(strings.Join(pageLinks(linksHTML, "https://example.com/guia/index.html"), ","))
		},
		"page_links_empty": func() string { return escapeEdit(strings.Join(pageLinks("<p>sin links</p>", "https://e.com/"), ",")) },
		"entities": func() string {
			return escapeEdit(decodeEntities("a &amp; b &#39;c&#x27; &lt;d&gt; &quot;e&quot; &nbsp;f"))
		},
		"entities_bare": func() string { return escapeEdit(decodeEntities("a & b &unknown; c")) },
		"entities_long": func() string { return escapeEdit(decodeEntities("&aaaaaaaaaaaaaaaaaaaaa;")) },
		"entities_hex":  func() string { return escapeEdit(decodeEntities("&#x1F600; &amp")) },
		"is_html": func() string {
			return fmt.Sprintf("%t,%t,%t,%t", isHTML(""), isHTML("text/html"), isHTML("application/xhtml+xml"), isHTML("text/plain"))
		},
		"truncate_short":    func() string { return escapeEdit(truncateText("corto")) },
		"compose_basic":     func() string { return escapeEdit(composePage("T", "body", nil)) },
		"compose_no_title":  func() string { return escapeEdit(composePage("", "body", nil)) },
		"compose_links":     func() string { return escapeEdit(composePage("T", "body", []string{"https://a", "https://b"})) },
		"compose_truncated": func() string { return escapeEdit(composePage("T", strings.Repeat("x", maxMarkdown+10), nil)) },
		"compose_empty":     func() string { return escapeEdit(composePage("", "   ", nil)) },
		"real_url": func() string {
			return escapeEdit(realURL("//duckduckgo.com/l/?uddg=https%3A%2F%2Frust-lang.org%2F&rut=abc"))
		},
		"real_url_plain":      func() string { return escapeEdit(realURL("https://x.com/y")) },
		"percent_decode":      func() string { return escapeEdit(percentDecode("a%20b%2Fc%ZZd")) },
		"attr":                func() string { return optionAttr(`a class="result__a" href='x' data-y=z`, "href") },
		"attr_class":          func() string { return optionAttr(`a class="result__a other" href="x"`, "class") },
		"tag_name":            func() string { return tagName("/a class=x") },
		"tag_name2":           func() string { return tagName("a href=x") },
		"tag_name3":           func() string { return tagName("br/") },
		"absolutize port":     func() string { return optionURL("http://127.0.0.1:8080", "x") },
		"absolutize rel base": func() string { return optionURL("https://example.com/guia/index.html", "otra.html") },
	}
	for _, href := range []string{"https://x.com/y", "//cdn.com/z", "/docs/x", "sub.html", "#frag", "?q=1", "mailto:a@b.com", "../up", "javascript:void(0)", "", "x:y", "a/b"} {
		href := href
		cases["absolutize "+href] = func() string { return optionURL(base, href) }
	}
	for _, contentType := range []string{
		"text/html; charset=ISO-8859-1",
		`text/html; charset="utf-8"`,
		"text/html",
		"text/html; charset=UTF-8; boundary=x",
		"text/plain; charset=Windows-1252",
	} {
		contentType := contentType
		cases["charset "+contentType] = func() string { return optionCharset(contentType) }
	}
	for _, input := range []string{"hola mundo", "a&b", "acentuá", "a b+c/d?e=f&g", ""} {
		input := input
		cases["encode "+input] = func() string { return encodeQuery(input) }
	}

	for name := range expected {
		if _, covered := cases[name]; !covered {
			t.Errorf("el dump trae el caso %q y nadie lo cubre", name)
		}
	}
	for name, want := range expected {
		check, covered := cases[name]
		if !covered {
			continue
		}
		if got := check(); got != want {
			t.Errorf("%s:\n got:  %s\n want: %s", name, got, want)
		}
	}
}

func TestSearchFormatsResults(t *testing.T) {
	items := parseResults(resultsHTML)
	if len(items) != 2 {
		t.Fatalf("items: %+v", items)
	}
	if items[0].url != "https://rust-lang.org/" || items[0].title != "The Rust Programming Language" {
		t.Fatalf("item 0: %+v", items[0])
	}
	if strings.TrimSpace(items[0].snippet) != "Rust is fast & reliable." {
		t.Fatalf("snippet: %q", items[0].snippet)
	}
	if items[1].url != "https://example.com/a?b=1" {
		t.Fatalf("item 1: %+v", items[1])
	}
	text := formatResults(items, defaultResults)
	if !strings.HasPrefix(text, "1. The Rust Programming Language\n   https://rust-lang.org/") {
		t.Fatalf("got: %q", text)
	}
	if lines := len(strings.Split(formatResults(items, 1), "\n")); lines != 3 {
		t.Fatalf("lineas: %d", lines)
	}
}

func TestFetchExtractsTheArticleAndDropsTheChrome(t *testing.T) {
	title, markdown, err := extractArticle(articleHTML, "https://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	if title != "Axe" {
		t.Fatalf("titulo: %q", title)
	}
	if !strings.Contains(markdown, "Real heading") {
		t.Fatalf("falta el heading:\n%s", markdown)
	}
	if !strings.Contains(markdown, "[link](https://example.com/x)") {
		t.Fatalf("falta el link:\n%s", markdown)
	}
	for _, unwanted := range []string{"menu menu", "footer"} {
		if strings.Contains(markdown, unwanted) {
			t.Fatalf("quedo %q:\n%s", unwanted, markdown)
		}
	}
}

func TestFetchToolWalksTheWholeWay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != fetchUA {
			t.Errorf("user agent: %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, articleHTML)
	}))
	defer server.Close()

	out := FetchTool().Run(fmt.Sprintf(`{"url":"%s"}`, server.URL), nil).Text
	for _, want := range []string{"# Axe", "Real heading", "## Links"} {
		if !strings.Contains(out, want) {
			t.Fatalf("falta %q en:\n%s", want, out)
		}
	}
}

func TestFetchToolRejectsBadURLs(t *testing.T) {
	out := FetchTool().Run(`{"url":"ftp://x"}`, nil).Text
	if !strings.HasPrefix(out, "error: url must start with http:// or https://") {
		t.Fatalf("got: %q", out)
	}
}

func TestFetchToolReportsHTTPErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	out := FetchTool().Run(fmt.Sprintf(`{"url":"%s"}`, server.URL), nil).Text
	if !strings.Contains(out, "error: HTTP 404 from") {
		t.Fatalf("got: %q", out)
	}
}

func TestFetchToolReturnsPlainText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "  hola mundo  ")
	}))
	defer server.Close()

	out := FetchTool().Run(fmt.Sprintf(`{"url":"%s"}`, server.URL), nil).Text
	if out != "hola mundo" {
		t.Fatalf("got: %q", out)
	}
}

func TestFetchToolRejectsUnsupportedContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		fmt.Fprint(w, "%PDF")
	}))
	defer server.Close()

	out := FetchTool().Run(fmt.Sprintf(`{"url":"%s"}`, server.URL), nil).Text
	if !strings.Contains(out, "unsupported content type application/pdf") {
		t.Fatalf("got: %q", out)
	}
}

func TestFetchCapsTheBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		chunk := strings.Repeat("x", 64*1024)
		for range (maxPage / len(chunk)) + 2 {
			fmt.Fprint(w, chunk)
		}
	}))
	defer server.Close()

	out := FetchTool().Run(fmt.Sprintf(`{"url":"%s"}`, server.URL), nil).Text
	if !strings.Contains(out, "error: response exceeds") {
		t.Fatalf("got: %q", out[:120])
	}
}

func TestSearchToolRejectsEmptyQuery(t *testing.T) {
	if out := SearchTool().Run(`{"query":"   "}`, nil).Text; out != "error: empty query" {
		t.Fatalf("got: %q", out)
	}
}

func TestDecodeBodyHandlesLatin1(t *testing.T) {
	if got := decodeBody([]byte("caf\xe9"), "text/html; charset=ISO-8859-1"); got != "café" {
		t.Fatalf("latin1: %q", got)
	}
	if got := decodeBody([]byte("hola"), ""); got != "hola" {
		t.Fatalf("utf8: %q", got)
	}
	if got := decodeBody([]byte{0xff, 0xfe}, ""); got != "\uFFFD\uFFFD" {
		t.Fatalf("invalido: %q", got)
	}
}

func TestMarkdownConversion(t *testing.T) {
	html := `<article><h2>T</h2><p>a <strong>b</strong> c</p><ul><li>uno</li><li>dos</li></ul><pre>code</pre></article>`
	_, markdown, err := extractArticle(html, "https://e.com/")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## T", "**b**", "- uno", "- dos", "```\ncode\n```"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("falta %q en:\n%s", want, markdown)
		}
	}
}
