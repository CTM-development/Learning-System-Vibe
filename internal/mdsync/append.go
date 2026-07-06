package mdsync

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// QABlock is one Q/A card to write into a note.
type QABlock struct {
	Front string
	Back  string
}

// GeneratedHeading is where accepted LLM cards land in a note (matches the
// essay template section).
const GeneratedHeading = "## Test cards generated from this essay"

var blankRunRe = regexp.MustCompile(`\n{2,}`)

// AppendQABlocks appends cards as Q/A blocks under `heading` (created at
// the end of the file when missing). It only ever appends — existing prose
// is untouched. Sync afterwards to assign anchors.
func (s *Syncer) AppendQABlocks(rel, heading string, cards []QABlock) error {
	if len(cards) == 0 {
		return nil
	}
	abs := filepath.Join(s.NotesDir, filepath.FromSlash(rel))
	raw, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	content := string(raw)

	var b strings.Builder
	b.WriteString(strings.TrimRight(content, "\n"))
	b.WriteString("\n")
	if !containsHeading(content, heading) {
		b.WriteString("\n" + heading + "\n")
	}
	for _, c := range cards {
		front := sanitizeBlockText(c.Front)
		back := sanitizeBlockText(c.Back)
		if front == "" || back == "" {
			return fmt.Errorf("card with empty front or back")
		}
		b.WriteString("\nQ: " + front + "\nA: " + back + "\n")
	}

	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(b.String()), info.Mode())
}

// sanitizeBlockText makes text safe inside a Q/A block: no blank lines
// (they terminate the block) and no lines that would start a new Q:/A:.
func sanitizeBlockText(s string) string {
	s = strings.TrimSpace(s)
	s = blankRunRe.ReplaceAllString(s, "\n")
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		if qLineRe.MatchString(lines[i]) || aLineRe.MatchString(lines[i]) {
			lines[i] = " " + lines[i] // indent so the parser reads it as continuation
		}
	}
	return strings.Join(lines, "\n")
}

func containsHeading(content, heading string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == heading {
			return true
		}
	}
	return false
}
