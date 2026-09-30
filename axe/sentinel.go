package axe

import (
	"os"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

const (
	redacted       = "[REDACTED]"
	minKnownLen    = 4
	minTokenLen    = 20
	minJWTLength   = 40
	pemBegin       = "-----BEGIN"
	pemEnd         = "-----END"
	pemDashes      = "-----"
	pemBeginLength = 10
	pemEndLength   = 8
)

type Sentinel struct {
	values []string
}

func NewSentinel() *Sentinel {
	return &Sentinel{}
}

func (s *Sentinel) AddValue(value string) {
	if len(value) < minKnownLen {
		return
	}
	for _, existing := range s.values {
		if existing == value {
			return
		}
	}
	s.values = append(s.values, value)
	sort.SliceStable(s.values, func(i, j int) bool { return len(s.values[i]) > len(s.values[j]) })
}

func (s *Sentinel) FromEnv() *Sentinel {
	for _, entry := range os.Environ() {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		if isSensitiveKey(key) && len(value) >= 8 && !strings.Contains(value, "/") && !strings.Contains(value, `\`) {
			s.AddValue(value)
		}
	}
	return s
}

func (s *Sentinel) Redact(text string) string {
	if text == "" {
		return ""
	}
	for _, value := range s.values {
		if strings.Contains(text, value) {
			text = strings.ReplaceAll(text, value, redacted)
		}
	}
	text = redactPem(text)
	text = redactUrls(text)
	text = redactTokens(text)
	return redactAssignments(text)
}

func (s *Sentinel) RedactMessage(message Message) Message {
	if message.Role != "user" && message.Role != "tool" {
		return message
	}
	message.Content = s.Redact(message.Content)
	return message
}

var (
	globalSentinel Sentinel
	globalOnce     sync.Once
	globalMu       sync.Mutex
)

func SeedSentinel(apiKey string) {
	globalOnce.Do(func() {
		globalSentinel = *NewSentinel().FromEnv()
		globalSentinel.AddValue(apiKey)
	})
}

func GlobalSentinel() *Sentinel {
	globalOnce.Do(func() {
		globalSentinel = *NewSentinel().FromEnv()
	})
	globalMu.Lock()
	defer globalMu.Unlock()
	return &globalSentinel
}

func Redact(text string) string {
	return GlobalSentinel().Redact(text)
}

func RedactMessage(message Message) Message {
	return GlobalSentinel().RedactMessage(message)
}

func redactPem(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	i := 0
	for {
		rel := strings.Index(s[i:], pemBegin)
		if rel < 0 {
			break
		}
		start := i + rel
		b1 := strings.Index(s[start+pemBeginLength:], pemDashes)
		if b1 < 0 {
			break
		}
		beginEnd := start + pemBeginLength + b1
		b2 := strings.Index(s[beginEnd+len(pemDashes):], pemEnd)
		if b2 < 0 {
			break
		}
		endStart := beginEnd + len(pemDashes) + b2
		b3 := strings.Index(s[endStart+pemEndLength:], pemDashes)
		if b3 < 0 {
			break
		}
		out.WriteString(s[i:start])
		out.WriteString(redacted)
		i = endStart + pemEndLength + b3 + len(pemDashes)
	}
	out.WriteString(s[i:])
	return out.String()
}

func redactUrls(s string) string {
	b := []byte(s)
	var out strings.Builder
	out.Grow(len(s))
	i := 0
	for i < len(s) {
		rel := strings.Index(s[i:], "://")
		if rel < 0 {
			out.WriteString(s[i:])
			break
		}
		schemeEnd := i + rel + 3
		out.WriteString(s[i:schemeEnd])
		j := schemeEnd
		for j < len(b) && !isUrlBreak(b[j]) {
			j++
		}
		authority := s[schemeEnd:j]
		at := strings.Index(authority, "@")
		if at >= 0 && strings.Contains(authority[:at], ":") {
			out.WriteString(redacted)
			out.WriteString(authority[at:])
		} else {
			out.WriteString(authority)
		}
		i = j
	}
	return out.String()
}

func isUrlBreak(c byte) bool {
	if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f' {
		return true
	}
	switch c {
	case '"', '\'', '<', '>', ')', ']', '}', ',', '/':
		return true
	}
	return false
}

func redactTokens(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	i := 0
	for i < len(s) {
		if length, ok := secretTokenLen(s, i); ok {
			out.WriteString(redacted)
			i += length
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		out.WriteRune(r)
		i += size
	}
	return out.String()
}

var tokenPrefixes = []string{
	"sk-",
	"sk_live_",
	"sk_test_",
	"rk_live_",
	"rk_test_",
	"pk_live_",
	"pk_test_",
	"ghp_",
	"gho_",
	"ghu_",
	"ghs_",
	"ghr_",
	"github_pat_",
	"glpat-",
	"gldt-",
	"glft-",
	"glsoat-",
	"xoxb-",
	"xoxp-",
	"xoxa-",
	"xoxr-",
	"xapp-",
	"whsec_",
	"npm_",
	"pypi-",
	"hf_",
	"AIza",
	"AKIA",
	"ASIA",
}

func secretTokenLen(s string, i int) (int, bool) {
	b := []byte(s)
	if i > 0 {
		prev := b[i-1]
		if isASCIIAlnum(prev) || prev == '_' {
			return 0, false
		}
	}
	rest := s[i:]
	length := 0
	for length < len(rest) && isTokenByte(rest[length]) {
		length++
	}
	if length == 0 {
		return 0, false
	}
	head := rest[:length]
	if strings.HasPrefix(head, "eyJ") {
		return length, length >= minJWTLength && strings.Count(head, ".") >= 2
	}
	matched := false
	for _, prefix := range tokenPrefixes {
		if len(head) >= len(prefix) && strings.EqualFold(head[:len(prefix)], prefix) {
			matched = true
			break
		}
	}
	return length, matched && length >= minTokenLen
}

func isTokenByte(c byte) bool {
	if isASCIIAlnum(c) {
		return true
	}
	switch c {
	case '_', '-', '.', '+', '=':
		return true
	}
	return false
}

func redactAssignments(s string) string {
	b := []byte(s)
	var out strings.Builder
	out.Grow(len(s))
	i := 0
	for i < len(s) {
		if isKeyByte(b[i]) && (i == 0 || !isKeyByte(b[i-1])) {
			valueStart, valueEnd, replacement, ok := matchAssignment(s, i)
			if ok {
				out.WriteString(s[i:valueStart])
				out.WriteString(replacement)
				i = valueEnd
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		out.WriteRune(r)
		i += size
	}
	return out.String()
}

func matchAssignment(s string, i int) (int, int, string, bool) {
	b := []byte(s)
	k := i
	for k < len(b) && isKeyByte(b[k]) {
		k++
	}
	key := s[i:k]
	if !isSensitiveKey(key) {
		return 0, 0, "", false
	}
	j := k
	if j < len(b) && (b[j] == '"' || b[j] == '\'') {
		j++
	}
	gap := j
	for gap < len(b) && (b[gap] == ' ' || b[gap] == '\t') {
		gap++
	}
	valueStart := 0
	separator := byte(0)
	if gap < len(b) && (b[gap] == '=' || b[gap] == ':') {
		separator = b[gap]
		v := gap + 1
		for v < len(b) && (b[v] == ' ' || b[v] == '\t') {
			v++
		}
		valueStart = v
	} else if gap > j {
		valueStart = gap
	} else {
		return 0, 0, "", false
	}
	if valueStart >= len(b) || strings.HasPrefix(s[valueStart:], redacted) {
		return 0, 0, "", false
	}
	if b[valueStart] != '"' && b[valueStart] != '\'' {
		token := bareToken(s, valueStart)
		allowed := strings.HasPrefix(key, "--") ||
			(separator == '=' && envStyleKey(key)) ||
			strings.EqualFold(token, "bearer") ||
			strings.EqualFold(token, "basic") ||
			looksLikeValue(token)
		if !allowed {
			return 0, 0, "", false
		}
	}
	valueEnd, replacement := redactValue(s, valueStart)
	if valueEnd == valueStart {
		return 0, 0, "", false
	}
	return valueStart, valueEnd, replacement, true
}

func bareToken(s string, start int) string {
	b := []byte(s)
	k := start
	for k < len(b) && !isValueBreak(b[k]) {
		k++
	}
	return s[start:k]
}

func looksLikeValue(token string) bool {
	hasDigit := false
	hasAlpha := false
	for _, c := range token {
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			hasAlpha = true
		}
	}
	return (len(token) >= 6 && hasDigit && hasAlpha) || startsWithSecretPrefix(token)
}

func startsWithSecretPrefix(token string) bool {
	for _, prefix := range tokenPrefixes {
		if len(token) >= len(prefix) && strings.EqualFold(token[:len(prefix)], prefix) {
			return true
		}
	}
	return false
}

func envStyleKey(key string) bool {
	if !strings.ContainsFunc(key, isASCIIAlnumRune) {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') && c != '_' {
			return false
		}
	}
	return true
}

func keyWords(raw string) []string {
	words := []string{}
	var current strings.Builder
	prevLower := false
	for _, c := range raw {
		if !isASCIIAlnumRune(c) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			prevLower = false
			continue
		}
		if c >= 'A' && c <= 'Z' && prevLower && current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
		prevLower = c >= 'a' && c <= 'z'
		current.WriteRune(unicode.ToLower(c))
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

func redactValue(s string, start int) (int, string) {
	b := []byte(s)
	quote := b[start]
	if quote == '"' || quote == '\'' {
		k := start + 1
		for k < len(b) && b[k] != quote {
			if b[k] == '\\' {
				k++
			}
			k++
		}
		end := k + 1
		if end > len(b) {
			end = len(b)
		}
		return end, string(quote) + redacted + string(quote)
	}
	k := start
	for k < len(b) && !isValueBreak(b[k]) {
		k++
	}
	segment := s[start:k]
	if strings.EqualFold(segment, "bearer") || strings.EqualFold(segment, "basic") {
		m := k
		for m < len(b) && isASCIIWhitespace(b[m]) {
			m++
		}
		if strings.HasPrefix(s[m:], redacted) {
			return m + len(redacted), redacted
		}
		n := m
		for n < len(b) && !isValueBreak(b[n]) {
			n++
		}
		if n > m {
			return n, redacted
		}
	}
	return k, redacted
}

func isValueBreak(c byte) bool {
	if isASCIIWhitespace(c) {
		return true
	}
	switch c {
	case ',', '}', ']', ')', ';', '"', '\'', '`':
		return true
	}
	return false
}

func isKeyByte(c byte) bool {
	if isASCIIAlnum(c) {
		return true
	}
	return c == '_' || c == '-'
}

var sensitiveWords = map[string]bool{
	"password":      true,
	"passwd":        true,
	"pwd":           true,
	"passphrase":    true,
	"secret":        true,
	"token":         true,
	"credential":    true,
	"creds":         true,
	"auth":          true,
	"authorization": true,
	"cookie":        true,
	"apikey":        true,
	"privatekey":    true,
	"accesskey":     true,
	"bearer":        true,
}

var sensitiveCompounds = []string{
	"apikey",
	"privatekey",
	"accesskey",
	"secretkey",
	"signingkey",
	"encryptionkey",
	"clientsecret",
}

func isSensitiveKey(raw string) bool {
	for _, word := range keyWords(raw) {
		if sensitiveWords[word] {
			return true
		}
	}
	joined := strings.Map(func(r rune) rune {
		if isASCIIAlnumRune(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, raw)
	for _, compound := range sensitiveCompounds {
		if strings.Contains(joined, compound) {
			return true
		}
	}
	return false
}

func isASCIIAlnum(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isASCIIAlnumRune(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isASCIIWhitespace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return false
}
