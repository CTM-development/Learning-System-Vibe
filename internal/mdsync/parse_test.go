package mdsync

import (
	"os"
	"path/filepath"
	"testing"
)

// repoTestdata resolves ../../testdata/notes relative to this package.
func repoTestdata(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "notes", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestParseFullNote(t *testing.T) {
	n, err := Parse("ml/variational-inference.md", repoTestdata(t, "ml/variational-inference.md"))
	if err != nil {
		t.Fatal(err)
	}

	if n.Title != "Variational Inference" {
		t.Errorf("Title = %q", n.Title)
	}
	if n.Stage != "deep" || n.Status != "draft" {
		t.Errorf("stage/status = %q/%q", n.Stage, n.Status)
	}
	if len(n.Tags) != 2 || n.Tags[0] != "ml" {
		t.Errorf("Tags = %v", n.Tags)
	}
	if len(n.Sources) != 1 || n.Sources[0] != "bishop2006prml" {
		t.Errorf("Sources = %v", n.Sources)
	}

	// 2 basic + 2 cloze (c1, c2); the fenced Q/A must NOT be a card.
	if len(n.Cards) != 4 {
		t.Fatalf("got %d cards, want 4: %+v", len(n.Cards), n.Cards)
	}

	c0 := n.Cards[0]
	if c0.Type != "basic" || c0.AnchorID != "0a1b2c3d" {
		t.Errorf("card0 = %+v", c0)
	}
	if c0.Front != "What does the ELBO lower-bound?" {
		t.Errorf("card0.Front = %q", c0.Front)
	}
	if c0.Back != `The marginal log likelihood $\log p(x)$.` {
		t.Errorf("card0.Back = %q (anchor must be stripped)", c0.Back)
	}

	c1 := n.Cards[1]
	if c1.AnchorID != "" {
		t.Errorf("card1 should have no anchor, got %q", c1.AnchorID)
	}
	if c1.Front != "Why is the KL divergence in VI reversed\ncompared to expectation propagation?" {
		t.Errorf("card1.Front = %q (multiline Q)", c1.Front)
	}
	if c1.Back != "VI minimizes $KL(q \\| p)$, which is mode-seeking;\nEP minimizes $KL(p \\| q)$, which is mass-covering." {
		t.Errorf("card1.Back = %q (multiline A)", c1.Back)
	}

	c2, c3 := n.Cards[2], n.Cards[3]
	if c2.Type != "cloze" || c2.ClozeIdx != 1 || c3.ClozeIdx != 2 {
		t.Errorf("cloze cards = %+v / %+v", c2, c3)
	}
	if c2.Front != "The ELBO decomposes into [...] minus the KL from the prior." {
		t.Errorf("c2.Front = %q", c2.Front)
	}
	if c2.Back != "The ELBO decomposes into expected log likelihood minus the KL from the prior." {
		t.Errorf("c2.Back = %q", c2.Back)
	}
	if c3.Front != "The ELBO decomposes into expected log likelihood minus [...]." {
		t.Errorf("c3.Front = %q", c3.Front)
	}

	if len(n.Questions) != 2 || n.Questions[0] != "How does amortization change the variational family?" {
		t.Errorf("Questions = %v", n.Questions)
	}
}

func TestParseDeckFile(t *testing.T) {
	n, err := Parse("decks/linear-algebra.md", repoTestdata(t, "decks/linear-algebra.md"))
	if err != nil {
		t.Fatal(err)
	}
	if n.Title != "linear-algebra" {
		t.Errorf("Title = %q (filename fallback)", n.Title)
	}
	if len(n.Cards) != 3 {
		t.Fatalf("got %d cards, want 3", len(n.Cards))
	}
	if n.Cards[1].Back != "When $x^T A x > 0$ for all nonzero $x$." {
		t.Errorf("card1.Back = %q", n.Cards[1].Back)
	}
	// Cloze hint renders as [scalar].
	if n.Cards[2].Front != "The determinant of a matrix equals [scalar]." {
		t.Errorf("cloze front = %q", n.Cards[2].Front)
	}
}

func TestParseTable(t *testing.T) {
	cases := []struct {
		name      string
		md        string
		cards     int
		questions int
	}{
		{"empty", "", 0, 0},
		{"prose only", "# Title\n\nJust some text.\n", 0, 0},
		{"q without a", "Q: dangling question\n\nmore prose\n", 0, 0},
		{"qa at eof no trailing newline", "Q: q?\nA: a", 1, 0},
		{"two adjacent qa blocks", "Q: one?\nA: 1\nQ: two?\nA: 2\n", 2, 0},
		{"cloze same index twice is one card", "Both {{c1::x}} and {{c1::y}} hide together.\n", 1, 0},
		{"open questions empty section", "## Open questions\n\nNo list here.\n", 0, 0},
		{"open questions star items", "## Open Questions\n* first?\n* second?\n", 0, 2},
		{"tags scalar", "---\ntags: solo\n---\nbody\n", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, err := Parse("t.md", tc.md)
			if err != nil {
				t.Fatal(err)
			}
			if len(n.Cards) != tc.cards {
				t.Errorf("cards = %d, want %d (%+v)", len(n.Cards), tc.cards, n.Cards)
			}
			if len(n.Questions) != tc.questions {
				t.Errorf("questions = %d, want %d", len(n.Questions), tc.questions)
			}
		})
	}

	// scalar tags land in the slice
	n, _ := Parse("t.md", "---\ntags: solo\n---\nbody\n")
	if len(n.Tags) != 1 || n.Tags[0] != "solo" {
		t.Errorf("scalar tags = %v", n.Tags)
	}
}

func TestCardID(t *testing.T) {
	basic := ParsedCard{AnchorID: "abc123", Type: "basic"}
	if basic.CardID() != "abc123" {
		t.Errorf("basic id = %q", basic.CardID())
	}
	cloze := ParsedCard{AnchorID: "abc123", Type: "cloze", ClozeIdx: 2}
	if cloze.CardID() != "abc123#2" {
		t.Errorf("cloze id = %q", cloze.CardID())
	}
}
