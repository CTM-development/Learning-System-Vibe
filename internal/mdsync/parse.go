// Package mdsync parses markdown notes into cards, metadata and open
// questions, and syncs them into the store, writing ID anchors back into
// the files. The parser never touches non-card text.
package mdsync

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParsedNote is the result of parsing one markdown file.
type ParsedNote struct {
	Path        string // relative to notes dir
	Title       string
	Frontmatter map[string]any
	Stage       string
	Status      string
	Tags        []string
	Sources     []string
	Content     string
	Cards       []ParsedCard
	Questions   []string
}

// ParsedCard is one card found in a note. Cards from the same cloze
// paragraph share an anchor; ID carries a "#n" suffix per cloze index.
type ParsedCard struct {
	AnchorID   string // "" when the block has no anchor yet
	ClozeIdx   int    // 0 for basic cards
	Type       string // "basic" | "cloze"
	Front      string
	Back       string
	AnchorLine int // 0-based line the anchor sits on / should be appended to
}

// CardID returns the card's storage ID: anchor for basic cards,
// anchor#idx for cloze cards.
func (c ParsedCard) CardID() string {
	if c.Type == "cloze" {
		return fmt.Sprintf("%s#%d", c.AnchorID, c.ClozeIdx)
	}
	return c.AnchorID
}

var (
	anchorRe   = regexp.MustCompile(`\s*<!--\s*srs:([0-9a-f]+)\s*-->\s*$`)
	clozeRe    = regexp.MustCompile(`\{\{c(\d+)::(.*?)(?:::(.*?))?\}\}`)
	qLineRe    = regexp.MustCompile(`^Q:\s?(.*)$`)
	aLineRe    = regexp.MustCompile(`^A:\s?(.*)$`)
	headingRe  = regexp.MustCompile(`^#{1,6}\s+(.*)$`)
	openQHdrRe = regexp.MustCompile(`(?i)^##+\s+open\s+questions\s*$`)
	listItemRe = regexp.MustCompile(`^\s*[-*]\s+(.*)$`)
)

// Parse parses one markdown file. relPath is the note's path relative to
// the notes directory (used for title fallback).
func Parse(relPath, content string) (*ParsedNote, error) {
	n := &ParsedNote{
		Path:        relPath,
		Frontmatter: map[string]any{},
		Tags:        []string{},
		Sources:     []string{},
		Content:     content,
	}

	lines := strings.Split(content, "\n")
	body := 0 // first line index after frontmatter

	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				fmText := strings.Join(lines[1:i], "\n")
				if err := yaml.Unmarshal([]byte(fmText), &n.Frontmatter); err != nil {
					return nil, fmt.Errorf("%s: frontmatter: %w", relPath, err)
				}
				body = i + 1
				break
			}
		}
	}
	n.Stage = fmString(n.Frontmatter, "stage")
	n.Status = fmString(n.Frontmatter, "status")
	n.Title = fmString(n.Frontmatter, "title")
	n.Tags = fmStrings(n.Frontmatter, "tags")
	n.Sources = fmStrings(n.Frontmatter, "sources")

	inFence := false
	inOpenQ := false

	for i := body; i < len(lines); i++ {
		line := lines[i]

		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		if m := headingRe.FindStringSubmatch(line); m != nil {
			inOpenQ = openQHdrRe.MatchString(line)
			if n.Title == "" && strings.HasPrefix(line, "# ") {
				n.Title = strings.TrimSpace(m[1])
			}
			continue
		}
		if inOpenQ {
			if m := listItemRe.FindStringSubmatch(line); m != nil {
				if q := strings.TrimSpace(m[1]); q != "" {
					n.Questions = append(n.Questions, q)
				}
			}
			continue
		}

		if qLineRe.MatchString(line) {
			card, next := parseQABlock(lines, i)
			if card != nil {
				n.Cards = append(n.Cards, *card)
			}
			i = next - 1 // loop increments
			continue
		}

		if clozeRe.MatchString(line) {
			cards, next := parseClozeParagraph(lines, i)
			n.Cards = append(n.Cards, cards...)
			i = next - 1
			continue
		}
	}

	if n.Title == "" {
		base := filepath.Base(relPath)
		n.Title = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return n, nil
}

// parseQABlock parses a card starting at lines[start] (which matches Q:).
// Returns the card (nil if there is no A: part) and the index of the first
// line after the block.
func parseQABlock(lines []string, start int) (*ParsedCard, int) {
	q := []string{qLineRe.FindStringSubmatch(lines[start])[1]}
	i := start + 1
	for ; i < len(lines); i++ {
		if aLineRe.MatchString(lines[i]) || strings.TrimSpace(lines[i]) == "" {
			break
		}
		q = append(q, lines[i])
	}
	if i >= len(lines) || !aLineRe.MatchString(lines[i]) {
		return nil, i // Q without A: not a card
	}

	a := []string{aLineRe.FindStringSubmatch(lines[i])[1]}
	lastLine := i
	i++
	for ; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" || qLineRe.MatchString(lines[i]) {
			break
		}
		a = append(a, lines[i])
		lastLine = i
	}

	card := &ParsedCard{Type: "basic", AnchorLine: lastLine}
	back := strings.Join(a, "\n")
	if m := anchorRe.FindStringSubmatch(back); m != nil {
		card.AnchorID = m[1]
		back = anchorRe.ReplaceAllString(back, "")
	}
	card.Front = strings.TrimSpace(strings.Join(q, "\n"))
	card.Back = strings.TrimSpace(back)
	return card, i
}

// parseClozeParagraph parses the paragraph starting at lines[start] and
// produces one card per distinct cloze index. Returns the cards and the
// index of the first line after the paragraph.
func parseClozeParagraph(lines []string, start int) ([]ParsedCard, int) {
	var para []string
	i := start
	lastLine := start
	for ; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			break
		}
		para = append(para, lines[i])
		lastLine = i
	}

	text := strings.Join(para, "\n")
	anchorID := ""
	if m := anchorRe.FindStringSubmatch(text); m != nil {
		anchorID = m[1]
		text = anchorRe.ReplaceAllString(text, "")
	}
	text = strings.TrimSpace(text)

	// Distinct cloze indexes, in order of first appearance.
	seen := map[int]bool{}
	var indexes []int
	for _, m := range clozeRe.FindAllStringSubmatch(text, -1) {
		idx, _ := strconv.Atoi(m[1])
		if !seen[idx] {
			seen[idx] = true
			indexes = append(indexes, idx)
		}
	}

	var cards []ParsedCard
	for _, idx := range indexes {
		cards = append(cards, ParsedCard{
			AnchorID:   anchorID,
			ClozeIdx:   idx,
			Type:       "cloze",
			Front:      renderCloze(text, idx, true),
			Back:       renderCloze(text, idx, false),
			AnchorLine: lastLine,
		})
	}
	return cards, i
}

// renderCloze rewrites cloze markers: the target index becomes "[...]"
// (or "[hint]") when hidden, its text when revealed; other indexes are
// always revealed.
func renderCloze(text string, target int, hidden bool) string {
	return clozeRe.ReplaceAllStringFunc(text, func(m string) string {
		g := clozeRe.FindStringSubmatch(m)
		idx, _ := strconv.Atoi(g[1])
		if hidden && idx == target {
			if g[3] != "" {
				return "[" + g[3] + "]"
			}
			return "[...]"
		}
		return g[2]
	})
}

func fmString(fm map[string]any, key string) string {
	if v, ok := fm[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// fmStrings reads a frontmatter value that may be a list or a single
// scalar into a string slice.
func fmStrings(fm map[string]any, key string) []string {
	out := []string{}
	switch v := fm[key].(type) {
	case string:
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	case []any:
		for _, item := range v {
			out = append(out, strings.TrimSpace(fmt.Sprint(item)))
		}
	}
	return out
}
