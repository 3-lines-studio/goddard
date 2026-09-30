package axe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	searchUA        = "Mozilla/5.0 (X11; Linux x86_64)"
	fetchUA         = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36"
	searchEndpoint  = "https://html.duckduckgo.com/html/"
	defaultResults  = 8
	transferTimeout = 30 * time.Second
	maxSearch       = 2 * 1024 * 1024
	maxPage         = 8 * 1024 * 1024
	maxMarkdown     = 32 * 1024
	maxLinks        = 40
	minText         = 200
	renderBudgetMs  = 4000
	renderTimeout   = 20 * time.Second
)

var browsers = []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"}

const (
	searchSchema = `{"type":"object","properties":{"query":{"type":"string","description":"Search query"},"count":{"type":"integer","description":"How many results to return (default 8)"}},"required":["query"]}`
	fetchSchema  = `{"type":"object","properties":{"url":{"type":"string","description":"HTTP or HTTPS URL"}},"required":["url"]}`
)

type searchArgs struct {
	Query string `json:"query"`
	Count *int   `json:"count"`
}

type fetchArgs struct {
	URL string `json:"url"`
}

func SearchTool() Tool {
	tool := NewTool("search",
		"Search the web. Returns a numbered list of results with title, URL and snippet; read one with fetch.",
		searchSchema,
		func(args searchArgs) string {
			query := strings.TrimSpace(args.Query)
			if query == "" {
				return "error: empty query"
			}
			count := defaultResults
			if args.Count != nil {
				count = *args.Count
			}
			if count < 1 {
				count = 1
			}
			if count > defaultResults*4 {
				count = defaultResults * 4
			}
			items, err := runSearch(query)
			if err != nil {
				return "error: " + err.Error()
			}
			return formatResults(items, count)
		})
	tool.Snippet = "Search the web (DuckDuckGo)"
	return tool
}

func FetchTool() Tool {
	tool := NewTool("fetch",
		"Fetch a URL and return it as Markdown. HTML goes through readability extraction, so you get the content and not the chrome. Pages that need JavaScript are rendered when a browser is available. The links found on the page are listed at the end.",
		fetchSchema,
		func(args fetchArgs) string {
			url := strings.TrimSpace(args.URL)
			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
				return fmt.Sprintf("error: url must start with http:// or https://, got %s", url)
			}
			text, err := runFetch(url)
			if err != nil {
				return "error: " + err.Error()
			}
			return text
		})
	tool.Snippet = "Fetch a URL as Markdown, rendering JavaScript pages when a browser is installed"
	return tool
}

func runSearch(query string) ([]searchItem, error) {
	url := searchEndpoint + "?q=" + encodeQuery(query)
	page, err := webGet(url, maxSearch, searchUA)
	if err != nil {
		return nil, err
	}
	if err := checkStatus(page); err != nil {
		return nil, err
	}
	return parseResults(decodeBody(page.body, page.contentType)), nil
}

func formatResults(items []searchItem, count int) string {
	if len(items) == 0 {
		return "no results"
	}
	var out strings.Builder
	for index, item := range items {
		if index >= count {
			break
		}
		snippet := strings.Join(strings.Fields(item.snippet), " ")
		fmt.Fprintf(&out, "%d. %s\n   %s\n", index+1, item.title, item.url)
		if snippet != "" {
			fmt.Fprintf(&out, "   %s\n", snippet)
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

func runFetch(url string) (string, error) {
	page, err := webGet(url, maxPage, fetchUA)
	if err != nil {
		return "", err
	}
	if err := checkStatus(page); err != nil {
		return "", err
	}
	if !isHTML(page.contentType) {
		return plainOrError(page)
	}

	html := decodeBody(page.body, page.contentType)
	title, markdown, err := extractArticle(html, page.url)
	if err != nil {
		return "", err
	}
	if utf8.RuneCountInString(markdown) < minText {
		if rendered, ok := renderPage(url); ok {
			if renderedTitle, renderedMarkdown, err := extractArticle(rendered, page.url); err == nil && strings.TrimSpace(renderedMarkdown) != "" {
				title = renderedTitle
				markdown = renderedMarkdown
				html = rendered
			}
		}
	}
	return composePage(title, markdown, pageLinks(html, page.url)), nil
}

func composePage(title, markdown string, links []string) string {
	var out strings.Builder
	if strings.TrimSpace(title) != "" {
		fmt.Fprintf(&out, "# %s\n\n", strings.TrimSpace(title))
	}
	out.WriteString(strings.TrimSpace(markdown))
	if utf8.RuneCountInString(out.String()) > maxMarkdown {
		runes := []rune(out.String())
		out.Reset()
		out.WriteString(string(runes[:maxMarkdown]))
		out.WriteString("\n\n[truncado]")
	}
	if len(links) > 0 {
		out.WriteString("\n\n## Links")
		for _, link := range links {
			out.WriteString("\n- <" + link + ">")
		}
	}
	if strings.TrimSpace(out.String()) == "" {
		return "the page has no readable content"
	}
	return out.String()
}

func plainOrError(page webPage) (string, error) {
	if strings.HasPrefix(page.contentType, "text/") {
		return truncateText(strings.TrimSpace(decodeBody(page.body, page.contentType))), nil
	}
	contentType := page.contentType
	if contentType == "" {
		contentType = "unknown"
	}
	return "", fmt.Errorf("unsupported content type %s", contentType)
}

func truncateText(text string) string {
	if utf8.RuneCountInString(text) <= maxMarkdown {
		return text
	}
	return string([]rune(text)[:maxMarkdown]) + "\n\n[truncado]"
}

func isHTML(contentType string) bool {
	return contentType == "" ||
		strings.HasPrefix(contentType, "text/html") ||
		strings.HasPrefix(contentType, "application/xhtml")
}

func checkStatus(page webPage) error {
	if page.status >= 200 && page.status < 300 {
		return nil
	}
	target := page.url
	if target == "" {
		target = "the server"
	}
	return fmt.Errorf("HTTP %d from %s", page.status, target)
}

type webPage struct {
	status      int
	contentType string
	url         string
	body        []byte
}

func webGet(url string, limit int, userAgent string) (webPage, error) {
	client := &http.Client{
		Timeout: transferTimeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("stopped after 5 redirects")
			}
			return nil
		},
	}
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return webPage{}, err
	}
	request.Header.Set("User-Agent", userAgent)
	response, err := client.Do(request)
	if err != nil {
		return webPage{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if err != nil {
		return webPage{}, err
	}
	if len(body) > limit {
		return webPage{}, fmt.Errorf("response exceeds %d KiB", limit/1024)
	}
	effective := url
	if response.Request != nil && response.Request.URL != nil {
		effective = response.Request.URL.String()
	}
	return webPage{
		status:      response.StatusCode,
		contentType: response.Header.Get("Content-Type"),
		url:         effective,
		body:        body,
	}, nil
}

func decodeBody(body []byte, contentType string) string {
	switch strings.ToLower(charset(contentType)) {
	case "iso-8859-1", "latin1", "latin-1", "cp1252", "windows-1252":
		runes := make([]rune, 0, len(body))
		for _, b := range body {
			if b < 0x80 {
				runes = append(runes, rune(b))
				continue
			}
			runes = append(runes, windows1252[b-0x80])
		}
		return string(runes)
	}
	return decodeUTF8(body)
}

func decodeUTF8(body []byte) string {
	var out strings.Builder
	out.Grow(len(body))
	for len(body) > 0 {
		r, size := utf8.DecodeRune(body)
		if r == utf8.RuneError && size == 1 {
			out.WriteRune('\uFFFD')
			body = body[1:]
			continue
		}
		out.WriteRune(r)
		body = body[size:]
	}
	return out.String()
}

var windows1252 = [128]rune{
	'€', '\u0081', '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', '\u008d', 'Ž', '\u008f',
	'\u0090', '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', '\u009d', 'ž', 'Ÿ',
	'\u00a0', '¡', '¢', '£', '¤', '¥', '¦', '§', '¨', '©', 'ª', '«', '¬', '\u00ad', '®', '¯',
	'°', '±', '²', '³', '´', 'µ', '¶', '·', '¸', '¹', 'º', '»', '¼', '½', '¾', '¿',
	'À', 'Á', 'Â', 'Ã', 'Ä', 'Å', 'Æ', 'Ç', 'È', 'É', 'Ê', 'Ë', 'Ì', 'Í', 'Î', 'Ï',
	'Ð', 'Ñ', 'Ò', 'Ó', 'Ô', 'Õ', 'Ö', '×', 'Ø', 'Ù', 'Ú', 'Û', 'Ü', 'Ý', 'Þ', 'ß',
	'à', 'á', 'â', 'ã', 'ä', 'å', 'æ', 'ç', 'è', 'é', 'ê', 'ë', 'ì', 'í', 'î', 'ï',
	'ð', 'ñ', 'ò', 'ó', 'ô', 'õ', 'ö', '÷', 'ø', 'ù', 'ú', 'û', 'ü', 'ý', 'þ', 'ÿ',
}

func charset(contentType string) string {
	parts := strings.Split(contentType, ";")
	for _, part := range parts[1:] {
		key, value, found := strings.Cut(part, "=")
		if !found {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "charset") {
			return strings.Trim(strings.TrimSpace(value), `"`)
		}
	}
	return ""
}

func encodeQuery(input string) string {
	var out strings.Builder
	for index := 0; index < len(input); index++ {
		b := input[index]
		switch {
		case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z', b >= '0' && b <= '9', b == '-', b == '.', b == '_', b == '~':
			out.WriteByte(b)
		case b == ' ':
			out.WriteByte('+')
		default:
			fmt.Fprintf(&out, "%%%02X", b)
		}
	}
	return out.String()
}

func renderPage(url string) (string, bool) {
	browser, ok := whichBrowser()
	if !ok {
		return "", false
	}
	args := []string{
		"--headless",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		fmt.Sprintf("--virtual-time-budget=%d", renderBudgetMs),
		"--dump-dom",
	}
	if os.Geteuid() == 0 {
		args = append(args, "--no-sandbox")
	}
	args = append(args, url)

	ctx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, browser, args...)
	command.Stdin = nil
	stdout := &limitedBuffer{limit: maxPage}
	command.Stdout = stdout
	command.Stderr = nil
	if err := command.Run(); err != nil {
		return "", false
	}
	if stdout.overflow || stdout.buffer.Len() == 0 {
		return "", false
	}
	return stdout.buffer.String(), true
}

type limitedBuffer struct {
	buffer   strings.Builder
	limit    int
	overflow bool
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if l.buffer.Len()+len(p) > l.limit {
		l.overflow = true
		return len(p), nil
	}
	l.buffer.Write(p)
	return len(p), nil
}

func whichBrowser() (string, bool) {
	path := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(path) {
		for _, name := range browsers {
			candidate := filepath.Join(dir, name)
			if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
				return candidate, true
			}
		}
	}
	return "", false
}
