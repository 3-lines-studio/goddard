package axe

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	htmlpkg "golang.org/x/net/html"
)

type searchItem struct {
	url     string
	title   string
	snippet string
}

const (
	modeIdle = iota
	modeTitle
	modeSnippet
)

func parseResults(html string) []searchItem {
	items := []searchItem{}
	mode := modeIdle
	title := ""
	href := ""
	rest := html
	for {
		open := strings.IndexByte(rest, '<')
		if open < 0 {
			break
		}
		text := decodeEntities(rest[:open])
		switch mode {
		case modeTitle:
			title += text
		case modeSnippet:
			if len(items) > 0 {
				items[len(items)-1].snippet += text
			}
		}
		rest = rest[open:]
		closing := strings.IndexByte(rest, '>')
		if closing < 0 {
			break
		}
		classifyTag(rest[1:closing], &items, &mode, &title, &href)
		rest = rest[closing+1:]
	}
	return items
}

func classifyTag(tag string, items *[]searchItem, mode *int, title *string, href *string) {
	if !strings.EqualFold(tagName(tag), "a") {
		return
	}
	if strings.HasPrefix(tag, "/") {
		switch *mode {
		case modeTitle:
			if *href != "" {
				*items = append(*items, searchItem{
					url:   realURL(*href),
					title: strings.TrimSpace(*title),
				})
			}
			*mode = modeIdle
		case modeSnippet:
			*mode = modeIdle
		}
		return
	}
	class, _ := attr(tag, "class")
	link, hasLink := attr(tag, "href")
	if containsToken(class, "result__a") {
		if hasLink {
			*href = link
		} else {
			*href = ""
		}
		*title = ""
		*mode = modeTitle
		return
	}
	if containsToken(class, "result__snippet") {
		*mode = modeSnippet
	}
}

func containsToken(class, token string) bool {
	for _, field := range strings.Fields(class) {
		if field == token {
			return true
		}
	}
	return false
}

func realURL(href string) string {
	href = strings.TrimPrefix(href, "//")
	if strings.HasPrefix(href, "//") {
		href = "https://" + href[2:]
	}
	if index := strings.Index(href, "uddg="); index >= 0 {
		tail := href[index+5:]
		if end := strings.IndexByte(tail, '&'); end >= 0 {
			tail = tail[:end]
		}
		return percentDecode(tail)
	}
	return href
}

func percentDecode(text string) string {
	out := make([]byte, 0, len(text))
	for index := 0; index < len(text); {
		if text[index] == '%' && index+2 < len(text) {
			if value, err := strconv.ParseUint(text[index+1:index+3], 16, 8); err == nil {
				out = append(out, byte(value))
				index += 3
				continue
			}
		}
		out = append(out, text[index])
		index++
	}
	return strings.ToValidUTF8(string(out), "\uFFFD")
}

func tagName(tag string) string {
	tag = strings.TrimPrefix(tag, "/")
	for index, char := range tag {
		if char == ' ' || char == '\t' || char == '\n' || char == '\r' || char == '/' {
			return tag[:index]
		}
	}
	return tag
}

func attr(tag, name string) (string, bool) {
	rest := tag
	for {
		at := findAttr(rest, name)
		if at < 0 {
			return "", false
		}
		rest = rest[at+len(name):]
		trimmed := strings.TrimLeft(rest, " \t\n\r")
		if !strings.HasPrefix(trimmed, "=") {
			continue
		}
		value := strings.TrimLeft(trimmed[1:], " \t\n\r")
		if value == "" {
			return "", true
		}
		quote := value[0]
		if quote == '"' || quote == '\'' {
			if end := strings.IndexByte(value[1:], quote); end >= 0 {
				return value[1 : 1+end], true
			}
			return "", true
		}
		if end := strings.IndexAny(value, " \t\n\r"); end >= 0 {
			return value[:end], true
		}
		return value, true
	}
}

func findAttr(rest, name string) int {
	start := 0
	for {
		offset := strings.Index(rest[start:], name)
		if offset < 0 {
			return -1
		}
		at := start + offset
		before := at == 0 || isHTMLSpace(rest[at-1])
		after := false
		if at+len(name) < len(rest) {
			next := rest[at+len(name)]
			after = isHTMLSpace(next) || next == '='
		}
		if before && after {
			return at
		}
		start = at + len(name)
	}
}

func isHTMLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

func decodeEntities(text string) string {
	var out strings.Builder
	rest := text
	for {
		amp := strings.IndexByte(rest, '&')
		if amp < 0 {
			break
		}
		out.WriteString(rest[:amp])
		tail := rest[amp:]
		end := strings.IndexByte(tail, ';')
		if end > 0 && end <= 12 {
			if char, ok := entityChar(tail[1:end]); ok {
				out.WriteRune(char)
				rest = tail[end+1:]
				continue
			}
		}
		out.WriteByte('&')
		rest = tail[1:]
	}
	out.WriteString(rest)
	return out.String()
}

func entityChar(entity string) (rune, bool) {
	switch entity {
	case "amp":
		return '&', true
	case "lt":
		return '<', true
	case "gt":
		return '>', true
	case "quot":
		return '"', true
	case "apos":
		return '\'', true
	case "nbsp":
		return ' ', true
	}
	if hex, found := strings.CutPrefix(strings.ToLower(entity), "#x"); found {
		if value, err := strconv.ParseUint(hex, 16, 32); err == nil {
			return rune(value), true
		}
		return 0, false
	}
	decimal, found := strings.CutPrefix(entity, "#")
	if !found {
		return 0, false
	}
	if value, err := strconv.ParseUint(decimal, 10, 32); err == nil {
		return rune(value), true
	}
	return 0, false
}

func absolutize(base, href string) (string, bool) {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href, true
	}
	scheme := strings.Index(base, "://")
	if scheme < 0 {
		return "", false
	}
	schemeEnd := scheme + 3
	originEnd := len(base)
	if at := strings.IndexByte(base[schemeEnd:], '/'); at >= 0 {
		originEnd = schemeEnd + at
	}
	origin := base[:originEnd]
	path := base[originEnd:]
	if rest, found := strings.CutPrefix(href, "//"); found {
		return base[:schemeEnd-3] + "://" + rest, true
	}
	if strings.HasPrefix(href, "/") {
		return origin + href, true
	}
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "?") {
		return "", false
	}
	if strings.Contains(href, "..") || strings.Contains(href, ":") {
		return "", false
	}
	dir := "/"
	if at := strings.LastIndexByte(path, '/'); at >= 0 {
		dir = path[:at+1]
	}
	return origin + dir + href, true
}

func pageLinks(html, base string) []string {
	document, err := htmlpkg.Parse(strings.NewReader(html))
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	links := []string{}
	var walk func(*htmlpkg.Node)
	walk = func(node *htmlpkg.Node) {
		if len(links) >= maxLinks {
			return
		}
		if node.Type == htmlpkg.ElementNode && node.Data == "a" {
			for _, attribute := range node.Attr {
				if attribute.Key != "href" {
					continue
				}
				if url, ok := absolutize(base, strings.TrimSpace(attribute.Val)); ok && !seen[url] {
					seen[url] = true
					links = append(links, url)
				}
				break
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	return links
}

var strippedElements = map[string]bool{
	"script": true, "style": true, "noscript": true, "iframe": true,
	"nav": true, "header": true, "footer": true, "aside": true,
	"form": true, "svg": true, "template": true,
}

var strippedClasses = []string{"nav", "menu", "sidebar", "footer", "header", "comment", "cookie", "banner", "advert"}

func extractArticle(html, url string) (string, string, error) {
	document, err := htmlpkg.Parse(strings.NewReader(html))
	if err != nil {
		return "", "", err
	}
	title := documentTitle(document)
	stripChrome(document)
	article := pickArticle(document)
	if article == nil {
		return title, "", nil
	}
	var out strings.Builder
	writeMarkdown(article, url, &out)
	return title, strings.TrimSpace(out.String()), nil
}

func documentTitle(document *htmlpkg.Node) string {
	if title := findElement(document, "title"); title != nil {
		if text := strings.TrimSpace(rawText(title)); text != "" {
			return text
		}
	}
	if heading := findElement(document, "h1"); heading != nil {
		return strings.TrimSpace(rawText(heading))
	}
	return ""
}

func findElement(node *htmlpkg.Node, name string) *htmlpkg.Node {
	if node.Type == htmlpkg.ElementNode && node.Data == name {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findElement(child, name); found != nil {
			return found
		}
	}
	return nil
}

func stripChrome(node *htmlpkg.Node) {
	for child := node.FirstChild; child != nil; {
		next := child.NextSibling
		if child.Type == htmlpkg.ElementNode && shouldStrip(child) {
			node.RemoveChild(child)
			child = next
			continue
		}
		if child.Type == htmlpkg.ElementNode {
			stripChrome(child)
		}
		child = next
	}
}

func shouldStrip(node *htmlpkg.Node) bool {
	if strippedElements[node.Data] {
		return true
	}
	for _, attribute := range node.Attr {
		if attribute.Key != "class" && attribute.Key != "id" {
			continue
		}
		lower := strings.ToLower(attribute.Val)
		for _, token := range strippedClasses {
			if strings.Contains(lower, token) {
				return true
			}
		}
	}
	return false
}

func pickArticle(document *htmlpkg.Node) *htmlpkg.Node {
	best := (*htmlpkg.Node)(nil)
	bestScore := 0
	var walk func(*htmlpkg.Node)
	walk = func(node *htmlpkg.Node) {
		if node.Type == htmlpkg.ElementNode {
			score := 0
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == htmlpkg.ElementNode && child.Data == "p" {
					score += utf8.RuneCountInString(rawText(child))
				}
			}
			if node.Data == "article" || node.Data == "main" {
				score += score / 2
			}
			if score > bestScore {
				best = node
				bestScore = score
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	if best == nil {
		return findElement(document, "body")
	}
	return best
}

func rawText(node *htmlpkg.Node) string {
	var out strings.Builder
	var walk func(*htmlpkg.Node)
	walk = func(n *htmlpkg.Node) {
		if n.Type == htmlpkg.TextNode {
			out.WriteString(n.Data)
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return out.String()
}

func writeMarkdown(node *htmlpkg.Node, url string, out *strings.Builder) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		renderNode(child, url, out)
	}
}

func renderNode(node *htmlpkg.Node, url string, out *strings.Builder) {
	if node.Type == htmlpkg.TextNode {
		out.WriteString(collapseWhitespace(node.Data))
		return
	}
	if node.Type != htmlpkg.ElementNode {
		return
	}
	switch node.Data {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(node.Data[1] - '0')
		blank(out)
		out.WriteString(strings.Repeat("#", level) + " " + strings.TrimSpace(rawText(node)) + "\n\n")
	case "p":
		blank(out)
		writeInline(node, url, out)
		out.WriteString("\n\n")
	case "br":
		out.WriteString("\n")
	case "hr":
		blank(out)
		out.WriteString("---\n\n")
	case "pre":
		blank(out)
		out.WriteString("```\n" + strings.Trim(rawText(node), "\n") + "\n```\n\n")
	case "blockquote":
		blank(out)
		var inner strings.Builder
		writeMarkdown(node, url, &inner)
		for _, line := range strings.Split(strings.TrimSpace(inner.String()), "\n") {
			out.WriteString("> " + line + "\n")
		}
		out.WriteString("\n")
	case "ul", "ol":
		blank(out)
		index := 1
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type != htmlpkg.ElementNode || child.Data != "li" {
				continue
			}
			marker := "- "
			if node.Data == "ol" {
				marker = fmt.Sprintf("%d. ", index)
				index++
			}
			out.WriteString(marker)
			var inner strings.Builder
			writeInline(child, url, &inner)
			out.WriteString(strings.TrimSpace(inner.String()) + "\n")
		}
		out.WriteString("\n")
	case "table":
		blank(out)
		writeTable(node, url, out)
		out.WriteString("\n")
	default:
		writeInline(node, url, out)
	}
}

func writeInline(node *htmlpkg.Node, url string, out *strings.Builder) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == htmlpkg.TextNode {
			out.WriteString(collapseWhitespace(child.Data))
			continue
		}
		if child.Type != htmlpkg.ElementNode {
			continue
		}
		switch child.Data {
		case "a":
			href, _ := attrValue(child, "href")
			target, ok := absolutize(url, strings.TrimSpace(href))
			if !ok {
				target = href
			}
			var inner strings.Builder
			writeInline(child, url, &inner)
			fmt.Fprintf(out, "[%s](%s)", strings.TrimSpace(inner.String()), target)
		case "strong", "b":
			out.WriteString("**" + strings.TrimSpace(rawText(child)) + "**")
		case "em", "i":
			out.WriteString("*" + strings.TrimSpace(rawText(child)) + "*")
		case "code":
			out.WriteString("`" + strings.TrimSpace(rawText(child)) + "`")
		case "img":
			alt, _ := attrValue(child, "alt")
			src, _ := attrValue(child, "src")
			fmt.Fprintf(out, "![%s](%s)", strings.TrimSpace(alt), strings.TrimSpace(src))
		case "br":
			out.WriteString("\n")
		default:
			writeInline(child, url, out)
		}
	}
}

func writeTable(node *htmlpkg.Node, url string, out *strings.Builder) {
	rows := 0
	var walk func(*htmlpkg.Node)
	walk = func(n *htmlpkg.Node) {
		if n.Type == htmlpkg.ElementNode && n.Data == "tr" {
			cells := []string{}
			for cell := n.FirstChild; cell != nil; cell = cell.NextSibling {
				if cell.Type == htmlpkg.ElementNode && (cell.Data == "td" || cell.Data == "th") {
					cells = append(cells, strings.TrimSpace(rawText(cell)))
				}
			}
			if len(cells) > 0 {
				out.WriteString("| " + strings.Join(cells, " | ") + " |\n")
				if rows == 0 {
					separators := make([]string, len(cells))
					for index := range separators {
						separators[index] = "---"
					}
					out.WriteString("| " + strings.Join(separators, " | ") + " |\n")
				}
				rows++
			}
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
}

func attrValue(node *htmlpkg.Node, name string) (string, bool) {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val, true
		}
	}
	return "", false
}

func collapseWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func blank(out *strings.Builder) {
	if out.Len() == 0 {
		return
	}
	current := out.String()
	if !strings.HasSuffix(current, "\n\n") {
		out.WriteString("\n")
	}
}
