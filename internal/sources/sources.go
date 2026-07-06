// Package sources manages citable source documents: PDF upload storage
// under the attachments directory, text extraction, and FTS indexing so
// PDF content is searchable alongside notes.
package sources

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"

	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// MaxUploadBytes caps a single PDF upload (128 MiB).
const MaxUploadBytes = 128 << 20

// Manager stores source files and their index entries.
type Manager struct {
	Store          *store.Store
	AttachmentsDir string
}

// SavePDF stores an uploaded PDF, creates its sources row and indexes its
// extracted text. filename is the client-provided name (used for key and
// title fallbacks). Extraction failure is non-fatal: the source is still
// stored and citable, just not full-text searchable.
func (m *Manager) SavePDF(filename, title, key string, r io.Reader) (store.SourceRow, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxUploadBytes+1))
	if err != nil {
		return store.SourceRow{}, err
	}
	if len(data) > MaxUploadBytes {
		return store.SourceRow{}, fmt.Errorf("file exceeds %d MiB limit", MaxUploadBytes>>20)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return store.SourceRow{}, fmt.Errorf("not a PDF file (missing %%PDF header)")
	}

	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	if title == "" {
		title = base
	}
	if key == "" {
		key = base
	}
	key, err = m.uniqueKey(slugify(key))
	if err != nil {
		return store.SourceRow{}, err
	}

	relPath := filepath.Join("pdfs", key+".pdf")
	absPath := filepath.Join(m.AttachmentsDir, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return store.SourceRow{}, err
	}
	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return store.SourceRow{}, err
	}

	src, err := m.Store.CreateSource("pdf", key, filepath.ToSlash(relPath), title)
	if err != nil {
		os.Remove(absPath)
		return store.SourceRow{}, err
	}

	text, err := ExtractText(absPath)
	if err != nil {
		log.Printf("pdf extraction failed for %s: %v", key, err)
		text = ""
	}
	if err := m.Store.IndexSourceText(src.ID, title, text); err != nil {
		return src, err
	}
	return src, nil
}

// FilePath resolves a source's absolute file path, confined to the
// attachments directory.
func (m *Manager) FilePath(src store.SourceRow) (string, error) {
	abs := filepath.Join(m.AttachmentsDir, filepath.FromSlash(src.Path))
	root, err := filepath.Abs(m.AttachmentsDir)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return "", fmt.Errorf("source path %q escapes attachments dir", src.Path)
	}
	return resolved, nil
}

// uniqueKey appends -2, -3, … until the key is free.
func (m *Manager) uniqueKey(key string) (string, error) {
	if key == "" {
		key = "source"
	}
	candidate := key
	for i := 2; ; i++ {
		taken, err := m.Store.SourceKeyExists(candidate)
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", key, i)
	}
}

// slugify lowercases and reduces a string to [a-z0-9-].
func slugify(s string) string {
	var b strings.Builder
	lastDash := true // suppress leading dash
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// ExtractText pulls plain text from a PDF: pdftotext (poppler) when
// available — much better layout handling — else a pure-Go fallback.
func ExtractText(path string) (string, error) {
	if pdftotext, err := exec.LookPath("pdftotext"); err == nil {
		out, err := exec.Command(pdftotext, "-enc", "UTF-8", path, "-").Output()
		if err == nil {
			return string(out), nil
		}
		log.Printf("pdftotext failed on %s (%v), falling back to pure-Go extractor", path, err)
	}
	return extractTextPureGo(path)
}

func extractTextPureGo(path string) (_ string, err error) {
	// The pdf library panics on some malformed files.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pdf parse panic: %v", r)
		}
	}()
	f, reader, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	r, err := reader.GetPlainText()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return "", err
	}
	return buf.String(), nil
}
