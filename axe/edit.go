package axe

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type editArg struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

type editArgs struct {
	Path  string
	Edits []editArg
}

func (a *editArgs) UnmarshalJSON(data []byte) error {
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return errors.New("expected an object")
	}
	path, ok := object["path"].(string)
	if !ok {
		return errors.New("missing path")
	}
	a.Path = path
	if value, present := object["edits"]; present {
		if text, isString := value.(string); isString {
			var parsed any
			if err := json.Unmarshal([]byte(text), &parsed); err != nil {
				return fmt.Errorf("edits is not valid JSON: %v", err)
			}
			value = parsed
		}
		switch typed := value.(type) {
		case nil:
		case []any:
			for _, item := range typed {
				edit, err := decodeEdit(item)
				if err != nil {
					return err
				}
				a.Edits = append(a.Edits, edit)
			}
		case map[string]any:
			edit, err := decodeEdit(typed)
			if err != nil {
				return err
			}
			a.Edits = append(a.Edits, edit)
		default:
			return fmt.Errorf("edits must be an array, got %v", value)
		}
	}
	oldText, hasOld := object["oldText"].(string)
	newText, hasNew := object["newText"].(string)
	if hasOld && hasNew {
		a.Edits = append(a.Edits, editArg{OldText: oldText, NewText: newText})
	}
	return nil
}

func decodeEdit(value any) (editArg, error) {
	var edit editArg
	encoded, err := json.Marshal(value)
	if err != nil {
		return edit, err
	}
	if err := json.Unmarshal(encoded, &edit); err != nil {
		return edit, err
	}
	return edit, nil
}

func EditTool(machine Machine) Tool {
	tool := NewTool("edit",
		"Edit a single file using exact text replacement. Every edits[].oldText must match a unique, non-overlapping region of the original file. If two changes affect the same block or nearby lines, merge them into one edit instead of emitting overlapping edits. Do not include large unchanged regions just to connect distant changes.",
		editSchema,
		func(args editArgs) string {
			if len(args.Edits) == 0 {
				return "error: edits must contain at least one replacement"
			}
			data, err := machine.Read(args.Path)
			if err != nil {
				return "error: " + err.Error()
			}
			if !utf8.Valid(data) {
				return fmt.Sprintf("error: %s is not valid UTF-8", args.Path)
			}
			result, err := applyEdits(args.Path, string(data), args.Edits)
			if err != nil {
				return err.Error()
			}
			if err := machine.Write(args.Path, []byte(result.content)); err != nil {
				return "error: " + err.Error()
			}
			return fmt.Sprintf("Successfully replaced %d block(s) in %s.\n\n%s", len(args.Edits), args.Path, result.patch)
		})
	tool.Sequential = true
	tool.Snippet = "Make precise file edits with exact text replacement, including multiple disjoint edits in one call"
	return tool
}

func normalizeLf(s string) string {
	if !strings.Contains(s, "\r") {
		return s
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

func lfMap(body string) []int {
	bytes := []byte(body)
	offsets := make([]int, 0, len(bytes)+1)
	for i := 0; i < len(bytes); {
		offsets = append(offsets, i)
		if bytes[i] == '\r' {
			if i+1 < len(bytes) && bytes[i+1] == '\n' {
				i += 2
			} else {
				i++
			}
		} else {
			i++
		}
	}
	offsets = append(offsets, len(bytes))
	return offsets
}

func withEnding(s, ending string) string {
	if ending == "\r\n" {
		return strings.Join(strings.Split(s, "\n"), "\r\n")
	}
	return s
}

func regionEnding(body string, bs, be int) string {
	if strings.Contains(body[bs:be], "\r\n") || strings.HasPrefix(body[be:], "\r\n") {
		return "\r\n"
	}
	if strings.HasPrefix(body[be:], "\n") {
		return "\n"
	}
	if strings.HasSuffix(body[:bs], "\r\n") {
		return "\r\n"
	}
	if strings.HasSuffix(body[:bs], "\n") {
		return "\n"
	}
	if strings.Contains(body, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func emptyOldError(path string, i, total int) string {
	if total == 1 {
		return fmt.Sprintf("error: oldText must not be empty in %s.", path)
	}
	return fmt.Sprintf("error: edits[%d].oldText must not be empty in %s.", i, path)
}

func notFoundError(path string, i, total int) string {
	if total == 1 {
		return fmt.Sprintf("error: Could not find the exact text in %s. The old text must match exactly including all whitespace and newlines.", path)
	}
	return fmt.Sprintf("error: Could not find edits[%d] in %s. The oldText must match exactly including all whitespace and newlines.", i, path)
}

func duplicateError(path string, i, total, n int) string {
	if total == 1 {
		return fmt.Sprintf("error: Found %d occurrences of the text in %s. The text must be unique. Please provide more context to make it unique.", n, path)
	}
	return fmt.Sprintf("error: Found %d occurrences of edits[%d] in %s. Each oldText must be unique. Please provide more context to make it unique.", n, i, path)
}

func noChangeError(path string, total int) string {
	if total == 1 {
		return fmt.Sprintf("error: No changes made to %s. The replacement produced identical content.", path)
	}
	return fmt.Sprintf("error: No changes made to %s. The replacements produced identical content.", path)
}

type applied struct {
	content string
	patch   string
}

type matched struct {
	edit    int
	start   int
	length  int
	newText string
}

type group struct {
	start int
	end   int
	edits []int
}

type span struct {
	start int
	end   int
}

func normalizeFuzzy(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRightFunc(lines[i], unicode.IsSpace)
	}
	return strings.Map(func(r rune) rune {
		switch r {
		case '\u2018', '\u2019', '\u201a', '\u201b':
			return '\''
		case '\u201c', '\u201d', '\u201e', '\u201f':
			return '"'
		case '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015', '\u2212':
			return '-'
		case '\u00a0', '\u202f', '\u205f', '\u3000':
			return ' '
		}
		if r >= '\u2002' && r <= '\u200a' {
			return ' '
		}
		if r >= '\uff01' && r <= '\uff5e' {
			return r - 0xfee0
		}
		return r
	}, strings.Join(lines, "\n"))
}

func lineSpans(content string) []span {
	spans := []span{}
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			spans = append(spans, span{start, i + 1})
			start = i + 1
		}
	}
	if start < len(content) {
		spans = append(spans, span{start, len(content)})
	}
	return spans
}

func splitLines(s string) []string {
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func fuzzyFind(content, old string) (int, int, bool) {
	if start := strings.Index(content, old); start >= 0 {
		return start, len(old), true
	}
	fuzzyOld := normalizeFuzzy(old)
	if start := strings.Index(normalizeFuzzy(content), fuzzyOld); start >= 0 {
		return start, len(fuzzyOld), true
	}
	return 0, 0, false
}

func countOccurrences(content, old string) int {
	fuzzyOld := normalizeFuzzy(old)
	if fuzzyOld == "" {
		return 0
	}
	return strings.Count(normalizeFuzzy(content), fuzzyOld)
}

func findExact(path, content string, olds, news []string) ([]matched, error) {
	total := len(olds)
	found := make([]matched, 0, total)
	for i, old := range olds {
		start := strings.Index(content, old)
		if start < 0 {
			return nil, errors.New(notFoundError(path, i, total))
		}
		remaining := content[start+len(old):]
		if strings.Contains(remaining, old) {
			return nil, errors.New(duplicateError(path, i, total, 1+strings.Count(remaining, old)))
		}
		found = append(found, matched{edit: i, start: start, length: len(old), newText: news[i]})
	}
	return found, nil
}

func findFuzzy(path, base string, olds, news []string) ([]matched, error) {
	total := len(olds)
	found := make([]matched, 0, total)
	for i, old := range olds {
		start, length, ok := fuzzyFind(base, old)
		if !ok {
			return nil, errors.New(notFoundError(path, i, total))
		}
		if n := countOccurrences(base, old); n > 1 {
			return nil, errors.New(duplicateError(path, i, total, n))
		}
		found = append(found, matched{edit: i, start: start, length: length, newText: news[i]})
	}
	return found, nil
}

func checkOverlap(path string, found []matched) error {
	sort.SliceStable(found, func(i, j int) bool { return found[i].start < found[j].start })
	for i := 0; i+1 < len(found); i++ {
		if found[i].start+found[i].length > found[i+1].start {
			return fmt.Errorf(
				"error: edits[%d] and edits[%d] overlap in %s. Merge them into one edit or target disjoint regions.",
				found[i].edit, found[i+1].edit, path,
			)
		}
	}
	return nil
}

func lineRange(spans []span, start, end int) (int, int, bool) {
	startLine := -1
	for i, s := range spans {
		if start >= s.start && start < s.end {
			startLine = i
			break
		}
	}
	if startLine < 0 {
		return 0, 0, false
	}
	endLine := startLine
	for endLine < len(spans) && spans[endLine].end < end {
		endLine++
	}
	if endLine >= len(spans) {
		return 0, 0, false
	}
	return startLine, endLine + 1, true
}

func groupRegions(spans []span, found []matched) []group {
	order := make([]int, len(found))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return found[order[i]].start < found[order[j]].start })
	groups := []group{}
	for _, i := range order {
		m := found[i]
		start, end, ok := lineRange(spans, m.start, m.start+m.length)
		if !ok {
			continue
		}
		if len(groups) > 0 {
			current := &groups[len(groups)-1]
			if start < current.end {
				if end > current.end {
					current.end = end
				}
				current.edits = append(current.edits, i)
				continue
			}
		}
		groups = append(groups, group{start: start, end: end, edits: []int{i}})
	}
	return groups
}

func groupBlock(source string, spans []span, g group, found []matched) string {
	start := spans[g.start].start
	end := spans[g.end-1].end
	block := source[start:end]
	for i := len(g.edits) - 1; i >= 0; i-- {
		m := found[g.edits[i]]
		offset := m.start - start
		block = block[:offset] + m.newText + block[offset+m.length:]
	}
	return block
}

func applyExact(body string, found []matched) string {
	var offsets []int
	if strings.Contains(body, "\r") {
		offsets = lfMap(body)
	}
	out := body
	for i := len(found) - 1; i >= 0; i-- {
		m := found[i]
		bs, be := m.start, m.start+m.length
		if offsets != nil {
			bs, be = offsets[m.start], offsets[m.start+m.length]
		}
		out = out[:bs] + withEnding(m.newText, regionEnding(body, bs, be)) + out[be:]
	}
	return out
}

func applyGroups(body string, bodyLines []span, base string, baseLines []span, groups []group, found []matched) string {
	var out strings.Builder
	line := 0
	for _, g := range groups {
		out.WriteString(body[bodyLines[line].start:bodyLines[g.start].start])
		block := groupBlock(base, baseLines, g, found)
		bs := bodyLines[g.start].start
		be := bodyLines[g.end-1].end
		out.WriteString(withEnding(block, regionEnding(body, bs, be)))
		line = g.end
	}
	tail := len(body)
	if line < len(bodyLines) {
		tail = bodyLines[line].start
	}
	out.WriteString(body[tail:])
	return out.String()
}

const maxPatchLines = 80

func truncatePatch(patch string) string {
	lines := splitLines(patch)
	if len(lines) <= maxPatchLines {
		return patch
	}
	head := maxPatchLines / 2
	tail := maxPatchLines - head
	var out strings.Builder
	for _, line := range lines[:head] {
		out.WriteString(line)
		out.WriteByte('\n')
	}
	fmt.Fprintf(&out, "... [%d lines omitted] ...\n", len(lines)-head-tail)
	for _, line := range lines[len(lines)-tail:] {
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}

func unifiedPatch(path, old string, spans []span, groups []group, blocks []string) string {
	const context = 4
	lineText := func(i int) string {
		s, e := spans[i].start, spans[i].end
		if old[e-1] == '\n' {
			e--
		}
		return old[s:e]
	}
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", path, path)
	newOffset := 0
	for index, g := range groups {
		prevEnd := 0
		if index > 0 {
			prevEnd = groups[index-1].end
		}
		nextStart := len(spans)
		if index+1 < len(groups) {
			nextStart = groups[index+1].start
		}
		oldStart := g.start - context
		if oldStart < prevEnd {
			oldStart = prevEnd
		}
		oldEnd := g.end + context
		if oldEnd > nextStart {
			oldEnd = nextStart
		}
		before := g.start - oldStart
		after := oldEnd - g.end
		added := splitLines(blocks[index])
		newStart := oldStart + newOffset + 1
		if newStart < 1 {
			newStart = 1
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", oldStart+1, before+(g.end-g.start)+after, newStart, before+len(added)+after)
		for i := oldStart; i < g.start; i++ {
			fmt.Fprintf(&out, " %s\n", lineText(i))
		}
		for i := g.start; i < g.end; i++ {
			fmt.Fprintf(&out, "-%s\n", lineText(i))
		}
		for _, line := range added {
			fmt.Fprintf(&out, "+%s\n", line)
		}
		for i := g.end; i < oldEnd; i++ {
			fmt.Fprintf(&out, " %s\n", lineText(i))
		}
		newOffset += len(added) - (g.end - g.start)
	}
	return truncatePatch(out.String())
}

func applyEdits(path, content string, edits []editArg) (applied, error) {
	if len(edits) == 0 {
		return applied{}, errors.New("error: edits must contain at least one replacement.")
	}
	bom := ""
	body := content
	if strings.HasPrefix(content, "\uFEFF") {
		bom = "\uFEFF"
		body = content[len("\uFEFF"):]
	}
	normalized := normalizeLf(body)
	olds := make([]string, len(edits))
	news := make([]string, len(edits))
	for i, edit := range edits {
		if edit.OldText == "" {
			return applied{}, errors.New(emptyOldError(path, i, len(edits)))
		}
		olds[i] = normalizeLf(edit.OldText)
		news[i] = normalizeLf(edit.NewText)
	}

	allExact := true
	for _, old := range olds {
		if !strings.Contains(normalized, old) {
			allExact = false
			break
		}
	}
	if allExact {
		found, err := findExact(path, normalized, olds, news)
		if err != nil {
			return applied{}, err
		}
		if err := checkOverlap(path, found); err != nil {
			return applied{}, err
		}
		spans := lineSpans(normalized)
		groups := groupRegions(spans, found)
		blocks := make([]string, len(groups))
		for i, g := range groups {
			blocks[i] = groupBlock(normalized, spans, g, found)
		}
		patch := unifiedPatch(path, normalized, spans, groups, blocks)
		out := applyExact(body, found)
		if out == body {
			return applied{}, errors.New(noChangeError(path, len(edits)))
		}
		return applied{content: bom + out, patch: patch}, nil
	}

	base := normalizeFuzzy(normalized)
	found, err := findFuzzy(path, base, olds, news)
	if err != nil {
		return applied{}, err
	}
	if err := checkOverlap(path, found); err != nil {
		return applied{}, err
	}
	bodyLines := lineSpans(body)
	baseLines := lineSpans(base)
	if len(bodyLines) != len(baseLines) {
		return applied{}, fmt.Errorf(
			"error: cannot match edits in %s: the file mixes line endings in a way that prevents safe reconstruction.", path,
		)
	}
	groups := groupRegions(baseLines, found)
	blocks := make([]string, len(groups))
	for i, g := range groups {
		blocks[i] = groupBlock(base, baseLines, g, found)
	}
	spans := lineSpans(normalized)
	patch := unifiedPatch(path, normalized, spans, groups, blocks)
	out := applyGroups(body, bodyLines, base, baseLines, groups, found)
	if out == body {
		return applied{}, errors.New(noChangeError(path, len(edits)))
	}
	return applied{content: bom + out, patch: patch}, nil
}
