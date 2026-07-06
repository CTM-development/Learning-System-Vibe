package mdsync

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ValidStages are the study-flow stages a note moves through.
var ValidStages = []string{"skim", "deep", "synthesis"}

var stageLineRe = regexp.MustCompile(`(?m)^stage:\s*.*$`)

// SetStage rewrites only the `stage:` frontmatter line of a note (adding
// frontmatter if the file has none) and leaves everything else untouched.
func (s *Syncer) SetStage(rel, stage string) error {
	valid := false
	for _, v := range ValidStages {
		if stage == v {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid stage %q (want one of %s)", stage, strings.Join(ValidStages, "|"))
	}

	abs := filepath.Join(s.NotesDir, filepath.FromSlash(rel))
	raw, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	content := string(raw)

	var updated string
	if fmEnd := frontmatterEnd(content); fmEnd > 0 {
		fm := content[:fmEnd]
		if stageLineRe.MatchString(fm) {
			fm = stageLineRe.ReplaceAllString(fm, "stage: "+stage)
		} else {
			// Insert after the opening --- line.
			nl := strings.IndexByte(fm, '\n')
			fm = fm[:nl+1] + "stage: " + stage + "\n" + fm[nl+1:]
		}
		updated = fm + content[fmEnd:]
	} else {
		updated = "---\nstage: " + stage + "\n---\n\n" + content
	}

	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(updated), info.Mode())
}

// frontmatterEnd returns the byte offset of the start of the closing "---"
// line of the frontmatter block, or -1 when the file has no frontmatter.
func frontmatterEnd(content string) int {
	lines := strings.SplitAfter(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return -1
	}
	offset := len(lines[0])
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			return offset
		}
		offset += len(line)
	}
	return -1
}
