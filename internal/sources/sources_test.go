package sources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CTM-development/learning-system-vibe/internal/store"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	return &Manager{Store: st, AttachmentsDir: t.TempDir()}
}

func fixturePDF(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "testdata", "pdfs", "fixture.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestExtractText(t *testing.T) {
	text, err := ExtractText(filepath.Join("..", "..", "testdata", "pdfs", "fixture.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "spectral theorem fixture") {
		t.Errorf("extracted text = %q", text)
	}
}

func TestExtractTextPureGo(t *testing.T) {
	text, err := extractTextPureGo(filepath.Join("..", "..", "testdata", "pdfs", "fixture.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "spectral theorem fixture") {
		t.Errorf("pure-Go extracted text = %q", text)
	}
}

func TestSavePDFStoresIndexesAndSearches(t *testing.T) {
	m := newTestManager(t)

	src, err := m.SavePDF("Bishop PRML (2006).pdf", "Pattern Recognition and ML", "", fixturePDF(t))
	if err != nil {
		t.Fatal(err)
	}
	if src.Key != "bishop-prml-2006" {
		t.Errorf("key = %q", src.Key)
	}
	if src.Kind != "pdf" || src.Title != "Pattern Recognition and ML" {
		t.Errorf("row = %+v", src)
	}

	// File landed under attachments/pdfs and is confined there.
	abs, err := m.FilePath(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Errorf("stored file missing: %v", err)
	}

	// Extracted text is searchable.
	hits, err := m.Store.SearchSources("orthonormal", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].SourceID != src.ID {
		t.Fatalf("hits = %+v", hits)
	}
	if !strings.Contains(hits[0].Snippet, "<mark>orthonormal</mark>") {
		t.Errorf("snippet = %q", hits[0].Snippet)
	}

	// Key collision → suffix.
	src2, err := m.SavePDF("bishop_prml 2006.pdf", "", "", fixturePDF(t))
	if err != nil {
		t.Fatal(err)
	}
	if src2.Key != "bishop-prml-2006-2" {
		t.Errorf("second key = %q", src2.Key)
	}

	// Non-PDF rejected.
	if _, err := m.SavePDF("evil.pdf", "", "", strings.NewReader("MZ not a pdf")); err == nil {
		t.Error("want error for non-PDF payload")
	}
}

func TestFilePathConfinement(t *testing.T) {
	m := newTestManager(t)
	_, err := m.FilePath(store.SourceRow{Path: "../../../etc/passwd"})
	if err == nil {
		t.Error("want error for path escaping attachments dir")
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Bishop PRML (2006)":   "bishop-prml-2006",
		"  weird__name!!.pdf ": "weird-name-pdf",
		"ALLCAPS":              "allcaps",
		"---":                  "",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
