package mdsync

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// Syncer scans the notes directory into the store, appending ID anchors to
// new card blocks. It never rewrites prose — the only file modification is
// appending "<!-- srs:xxxx -->" comments.
type Syncer struct {
	Store    *store.Store
	NotesDir string

	// mu serializes SyncAll and every helper that mutates a note file
	// (anchor write-back, appends, stage edits). The file watcher and the
	// API handlers both trigger these concurrently; without this lock two
	// runs could parse the same anchor-less card, mint different anchors,
	// and race on the file write — corrupting anchors and duplicating cards.
	mu sync.Mutex
}

// Result summarizes one sync run.
type Result struct {
	Notes          int `json:"notes"`
	CardsCreated   int `json:"cards_created"`
	CardsUpdated   int `json:"cards_updated"`
	CardsOrphaned  int `json:"cards_orphaned"`
	AnchorsWritten int `json:"anchors_written"`
	OpenQuestions  int `json:"open_questions"`
}

// SyncAll scans every .md file under NotesDir, reconciles the store and
// logs one "sync" activity event.
func (s *Syncer) SyncAll() (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var res Result

	files, err := listMarkdownFiles(s.NotesDir)
	if err != nil {
		return res, err
	}

	usedAnchors, err := s.Store.ListCardBaseIDs()
	if err != nil {
		return res, err
	}

	// Preloaded once so unchanged files can be skipped without a per-note
	// query: hashes[path] is the content hash we last stored, cardsByNote
	// keeps a skipped note's cards from being orphaned.
	hashes, err := s.Store.NoteContentHashes()
	if err != nil {
		return res, err
	}
	cardsByNote, err := s.Store.ActiveCardIDsByNote()
	if err != nil {
		return res, err
	}

	seenCards := map[string]bool{}
	seenNotes := map[string]bool{}

	for _, rel := range files {
		if err := s.syncFile(rel, usedAnchors, seenCards, hashes, cardsByNote, &res); err != nil {
			return res, fmt.Errorf("sync %s: %w", rel, err)
		}
		seenNotes[rel] = true
		res.Notes++
	}

	// Notes removed from disk.
	stored, err := s.Store.ListNotePaths()
	if err != nil {
		return res, err
	}
	for _, path := range stored {
		if !seenNotes[path] {
			if err := s.Store.DeleteNote(path); err != nil {
				return res, err
			}
		}
	}

	// Cards whose anchors vanished (or whose file did) → orphan.
	active, err := s.Store.ListActiveCardIDs()
	if err != nil {
		return res, err
	}
	var vanished []string
	for _, id := range active {
		if !seenCards[id] {
			vanished = append(vanished, id)
		}
	}
	if err := s.Store.OrphanCards(vanished); err != nil {
		return res, err
	}
	res.CardsOrphaned = len(vanished)

	// Wikilink targets can only be resolved once every note is in place.
	if err := s.Store.ResolveNoteLinks(); err != nil {
		return res, err
	}

	if _, err := s.Store.LogEvent("sync", "", 0, 0, res); err != nil {
		return res, err
	}
	return res, nil
}

// syncFile parses one note, writes anchors for new cards back into the
// file, and upserts note, cards and open questions.
func (s *Syncer) syncFile(rel string, usedAnchors map[string]bool, seenCards map[string]bool, hashes map[string]string, cardsByNote map[string][]string, res *Result) error {
	abs := filepath.Join(s.NotesDir, rel)

	raw, err := os.ReadFile(abs)
	if err != nil {
		return err
	}

	// Skip notes whose bytes match what we last stored: reparse, re-upsert
	// and FTS reindex are the dominant sync cost, and every LLM/capture
	// action triggers a full SyncAll. Hashing the content (rather than mtime)
	// keeps this correct across same-second edits, git checkouts and backup
	// restores. A skipped note's cards must still be marked seen or the
	// orphan sweep below would soft-delete them.
	sum := hashContent(raw)
	if stored, ok := hashes[rel]; ok && stored == sum {
		for _, id := range cardsByNote[rel] {
			seenCards[id] = true
		}
		return nil
	}

	parsed, err := Parse(rel, string(raw))
	if err != nil {
		return err
	}

	// Assign anchors to new card blocks. Cloze cards from one paragraph
	// share a line and therefore one anchor.
	newAnchorByLine := map[int]string{}
	for i := range parsed.Cards {
		card := &parsed.Cards[i]
		if card.AnchorID != "" {
			continue
		}
		id, ok := newAnchorByLine[card.AnchorLine]
		if !ok {
			id = newAnchorID(usedAnchors)
			newAnchorByLine[card.AnchorLine] = id
		}
		card.AnchorID = id
	}

	if len(newAnchorByLine) > 0 {
		info, err := os.Stat(abs)
		if err != nil {
			return err
		}
		lines := strings.Split(string(raw), "\n")
		for lineIdx, id := range newAnchorByLine {
			lines[lineIdx] += fmt.Sprintf(" <!-- srs:%s -->", id)
		}
		updated := strings.Join(lines, "\n")
		if err := os.WriteFile(abs, []byte(updated), info.Mode()); err != nil {
			return fmt.Errorf("write anchors: %w", err)
		}
		parsed.Content = updated
		res.AnchorsWritten += len(newAnchorByLine)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if err := s.Store.UpsertNote(store.NoteRow{
		Path:        rel,
		Title:       parsed.Title,
		Frontmatter: parsed.Frontmatter,
		Stage:       parsed.Stage,
		Status:      parsed.Status,
		Type:        parsed.Type,
		Tags:        parsed.Tags,
		Sources:     parsed.Sources,
		Mtime:       info.ModTime().Unix(),
		Content:     parsed.Content,
		// Hash of the final on-disk bytes (post anchor write-back) so the
		// next sync of an unchanged file matches and skips.
		ContentHash: hashContent([]byte(parsed.Content)),
	}); err != nil {
		return err
	}

	if err := s.Store.SetNoteLinks(rel, parsed.Links); err != nil {
		return err
	}

	deck := filepath.ToSlash(filepath.Dir(rel))
	if deck == "." {
		deck = ""
	}
	for _, card := range parsed.Cards {
		created, err := s.Store.UpsertCard(store.CardRow{
			ID:       card.CardID(),
			NotePath: rel,
			Type:     card.Type,
			Front:    card.Front,
			Back:     card.Back,
			Deck:     deck,
			Tags:     parsed.Tags,
		})
		if err != nil {
			return err
		}
		if created {
			res.CardsCreated++
		} else {
			res.CardsUpdated++
		}
		seenCards[card.CardID()] = true
	}

	res.OpenQuestions += len(parsed.Questions)
	return s.Store.SyncOpenQuestions(rel, parsed.Questions)
}

// listMarkdownFiles returns relative paths of all .md files under root,
// skipping hidden directories, sorted for deterministic runs.
func listMarkdownFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); strings.HasPrefix(name, ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// hashContent returns the hex SHA-256 of a note's bytes, used to detect
// files unchanged since the last sync.
func hashContent(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// newAnchorID generates an 8-hex-char id not present in used, and records
// it there.
func newAnchorID(used map[string]bool) string {
	for {
		b := make([]byte, 4)
		if _, err := rand.Read(b); err != nil {
			panic(err) // crypto/rand failure is unrecoverable
		}
		id := hex.EncodeToString(b)
		if !used[id] {
			used[id] = true
			return id
		}
	}
}
