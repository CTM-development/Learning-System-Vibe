package mdsync

import (
	"strings"
	"testing"
)

func TestSetStage(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string // substring the updated file must start with
	}{
		{
			"replaces existing stage line",
			"---\ntitle: X\nstage: skim\ntags: [a]\n---\n\nBody Q: not touched\n",
			"---\ntitle: X\nstage: deep\ntags: [a]\n---\n\nBody Q: not touched\n",
		},
		{
			"adds stage to existing frontmatter",
			"---\ntitle: X\n---\n\nBody\n",
			"---\nstage: deep\ntitle: X\n---\n\nBody\n",
		},
		{
			"creates frontmatter when absent",
			"# Just a heading\n\nProse.\n",
			"---\nstage: deep\n---\n\n# Just a heading\n\nProse.\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, dir := newTestSyncer(t)
			writeNote(t, dir, "n.md", tc.content)
			if err := s.SetStage("n.md", "deep"); err != nil {
				t.Fatal(err)
			}
			got := readNote(t, dir, "n.md")
			if got != tc.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

func TestSetStageRejectsInvalid(t *testing.T) {
	s, dir := newTestSyncer(t)
	writeNote(t, dir, "n.md", "body\n")
	if err := s.SetStage("n.md", "bogus"); err == nil || !strings.Contains(err.Error(), "invalid stage") {
		t.Errorf("err = %v, want invalid stage", err)
	}
}
