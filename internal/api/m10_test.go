package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CTM-development/learning-system-vibe/internal/llm"
	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// fakeJPEG/fakePNG are just magic bytes plus junk — SaveScan only sniffs
// the prefix.
var (
	fakeJPEG = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte("jpeg-body")...)
	fakePNG  = append([]byte("\x89PNG\r\n\x1a\n"), []byte("png-body")...)
)

func uploadScan(t *testing.T, url, title string, pages ...[]byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, p := range pages {
		fw, _ := mw.CreateFormFile("pages", "img.jpg")
		fw.Write(p)
	}
	if title != "" {
		mw.WriteField("title", title)
	}
	mw.Close()
	res, err := http.Post(url+"/api/sources", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestScanUploadAndPaging drives scan upload → meta pages → page serving,
// plus the rejection paths.
func TestScanUploadAndPaging(t *testing.T) {
	ts, _, _ := newTestServer(t)

	res := uploadScan(t, ts.URL, "Measure Theory Lecture 3", fakeJPEG, fakePNG)
	if res.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("scan upload: %d %s", res.StatusCode, body)
	}
	src := decode[store.SourceRow](t, res)
	if src.Kind != "scan" || src.Key != "measure-theory-lecture-3" {
		t.Errorf("source = %+v", src)
	}
	var meta struct {
		Pages []string `json:"pages"`
	}
	json.Unmarshal([]byte(src.Meta), &meta)
	if len(meta.Pages) != 2 || meta.Pages[0] != "page-01.jpg" || meta.Pages[1] != "page-02.png" {
		t.Fatalf("meta = %s", src.Meta)
	}

	// Pages served with sniffed extensions, in order.
	page1 := mustGet(t, ts.URL+"/api/sources/1/page/1")
	data, _ := io.ReadAll(page1.Body)
	page1.Body.Close()
	if !bytes.Equal(data, fakeJPEG) {
		t.Error("page 1 differs from upload")
	}
	page2 := mustGet(t, ts.URL+"/api/sources/1/page/2")
	if ct := page2.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("page 2 Content-Type = %q", ct)
	}
	page2.Body.Close()

	// Out of range and garbage page numbers.
	for path, want := range map[string]int{
		"/api/sources/1/page/3":  http.StatusNotFound,
		"/api/sources/1/page/0":  http.StatusBadRequest,
		"/api/sources/1/page/xx": http.StatusBadRequest,
	} {
		r, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != want {
			t.Errorf("GET %s = %d, want %d", path, r.StatusCode, want)
		}
	}

	// A non-image page is rejected.
	bad := uploadScan(t, ts.URL, "junk", []byte("not an image"))
	defer bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Errorf("non-image scan upload: %d", bad.StatusCode)
	}

	// Untitled upload falls back to a dated title/key.
	res2 := uploadScan(t, ts.URL, "", fakeJPEG)
	src2 := decode[store.SourceRow](t, res2)
	today := time.Now().Format("2006-01-02")
	if src2.Key != "scan-"+today || src2.Title != "Scan "+today {
		t.Errorf("default scan naming = %+v", src2)
	}
}

// TestCreateNote drives the workbench save path: thoughts land dated in
// thoughts/, reading notes take a stage, cards go live on save, and the
// editing time is logged.
func TestCreateNote(t *testing.T) {
	ts, srv, _ := newTestServer(t)

	// A scan to cite.
	res := uploadScan(t, ts.URL, "Lecture scan", fakeJPEG)
	scan := decode[store.SourceRow](t, res)

	// Thought with a card in the body.
	res = postJSON(t, ts.URL+"/api/notes", map[string]any{
		"title":      "ELBO tightness idea",
		"type":       "thought",
		"body":       "The gap is exactly the KL.\n\nQ: What is the ELBO gap?\nA: KL(q||p).",
		"tags":       []string{"vi"},
		"elapsed_ms": 45000,
	})
	if res.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("create thought: %d %s", res.StatusCode, body)
	}
	note := decode[store.NoteDetail](t, res)
	today := time.Now().Format("2006-01-02")
	if note.Path != "thoughts/"+today+"-elbo-tightness-idea.md" {
		t.Errorf("thought path = %q", note.Path)
	}
	if note.Type != "thought" || note.Stage != "" || note.CardCount != 1 {
		t.Errorf("thought = %+v", note.NoteSummary)
	}

	// Same title again → uniquified filename, not an overwrite.
	res = postJSON(t, ts.URL+"/api/notes", map[string]any{
		"title": "ELBO tightness idea", "type": "thought",
	})
	dup := decode[store.NoteDetail](t, res)
	if dup.Path != "thoughts/"+today+"-elbo-tightness-idea-2.md" {
		t.Errorf("dup path = %q", dup.Path)
	}

	// Reading note citing the scan, with a stage and folder.
	res = postJSON(t, ts.URL+"/api/notes", map[string]any{
		"title":   "Measure Theory L3",
		"stage":   "skim",
		"folder":  "analysis",
		"sources": []string{scan.Key},
		"body":    "Sigma algebras etc.",
	})
	if res.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("create reading note: %d %s", res.StatusCode, body)
	}
	reading := decode[store.NoteDetail](t, res)
	if reading.Path != "analysis/measure-theory-l3.md" || reading.Stage != "skim" ||
		reading.Type != "reading" || len(reading.Sources) != 1 || reading.Sources[0] != scan.Key {
		t.Errorf("reading note = %+v", reading.NoteSummary)
	}

	// The type filter separates thoughts from reading notes.
	thoughts := decode[[]store.NoteSummary](t, mustGet(t, ts.URL+"/api/notes?type=thought"))
	if len(thoughts) != 2 {
		t.Errorf("thought filter = %+v", thoughts)
	}
	readingList := decode[[]store.NoteSummary](t, mustGet(t, ts.URL+"/api/notes?type=reading"))
	if len(readingList) != 1 {
		t.Errorf("reading filter = %+v", readingList)
	}

	// Editing time landed as a note_edit event.
	var elapsed int64
	err := srv.Store.DB.QueryRow(
		`SELECT elapsed_ms FROM activity_events WHERE kind = 'note_edit' AND ref = ?`,
		note.Path).Scan(&elapsed)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed != 45000 {
		t.Errorf("note_edit elapsed = %d", elapsed)
	}

	// Rejections: unknown source key, bad stage, bad type, empty title.
	for name, body := range map[string]map[string]any{
		"unknown source": {"title": "x", "sources": []string{"nope"}},
		"bad stage":      {"title": "x", "stage": "cram"},
		"bad type":       {"title": "x", "type": "essay"},
		"empty title":    {"title": "  "},
	} {
		res := postJSON(t, ts.URL+"/api/notes", body)
		res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, res.StatusCode)
		}
	}
}

// TestTranscribeFlow checks the vision request shape (multimodal parts),
// budget accounting under the transcribe purpose, and that card syntax in
// the model output is defused.
func TestTranscribeFlow(t *testing.T) {
	ts, srv, _ := newTestServer(t)

	// Fake OpenRouter that captures the request body.
	var captured []byte
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{
				"content": "# Lecture 3\n\nMeasures are countably additive.\n\nQ: sneaky card?\nA: no.\n\n{{c1::defused}}",
			}}},
			"usage": map[string]any{"prompt_tokens": 900, "completion_tokens": 300, "cost": 0.002},
		})
	}))
	t.Cleanup(or.Close)
	srv.LLM = &llm.Client{APIKey: "test-key", BaseURL: or.URL}

	res := uploadScan(t, ts.URL, "Lecture 3", fakeJPEG, fakePNG)
	scan := decode[store.SourceRow](t, res)

	res = postJSON(t, ts.URL+"/api/llm/transcribe", map[string]any{"source_id": scan.ID})
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("transcribe: %d %s", res.StatusCode, body)
	}
	out := decode[struct {
		Text      string    `json:"text"`
		Model     string    `json:"model"`
		Usage     llm.Usage `json:"usage"`
		SourceKey string    `json:"source_key"`
	}](t, res)

	// Card syntax defused (demoted from line start, text preserved),
	// content intact.
	if !strings.Contains(out.Text, " Q: sneaky") {
		t.Errorf("Q: text lost:\n%s", out.Text)
	}
	if strings.Contains(out.Text, "\nQ:") || strings.HasPrefix(out.Text, "Q:") {
		t.Errorf("Q: line not defused:\n%s", out.Text)
	}
	if strings.Contains(out.Text, "{{c1::") {
		t.Errorf("cloze not defused:\n%s", out.Text)
	}
	if !strings.Contains(out.Text, "countably additive") || out.SourceKey != scan.Key {
		t.Errorf("transcript = %+v", out)
	}

	// The request carried both pages as data-URI image parts.
	var reqBody struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(captured, &reqBody); err != nil {
		t.Fatal(err)
	}
	if len(reqBody.Messages) != 2 {
		t.Fatalf("messages = %d", len(reqBody.Messages))
	}
	var sysContent string
	if err := json.Unmarshal(reqBody.Messages[0].Content, &sysContent); err != nil {
		t.Errorf("system content should be a plain string: %s", reqBody.Messages[0].Content)
	}
	var parts []struct {
		Type     string `json:"type"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(reqBody.Messages[1].Content, &parts); err != nil {
		t.Fatalf("user content should be parts: %v", err)
	}
	var imgs []string
	for _, p := range parts {
		if p.Type == "image_url" {
			imgs = append(imgs, p.ImageURL.URL)
		}
	}
	if len(imgs) != 2 ||
		!strings.HasPrefix(imgs[0], "data:image/jpeg;base64,") ||
		!strings.HasPrefix(imgs[1], "data:image/png;base64,") {
		t.Errorf("image parts = %v", imgs)
	}

	// Budget accounting under the transcribe purpose.
	var purpose string
	var tokens int64
	err := srv.Store.DB.QueryRow(
		`SELECT purpose, tokens_in + tokens_out FROM llm_calls`).Scan(&purpose, &tokens)
	if err != nil {
		t.Fatal(err)
	}
	if purpose != "transcribe" || tokens != 1200 {
		t.Errorf("llm_call = %s / %d tokens", purpose, tokens)
	}

	// Non-scan sources are rejected.
	res = postJSON(t, ts.URL+"/api/sources", map[string]string{
		"kind": "book", "title": "Bishop",
	})
	book := decode[store.SourceRow](t, res)
	res = postJSON(t, ts.URL+"/api/llm/transcribe", map[string]any{"source_id": book.ID})
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("transcribe book: %d, want 400", res.StatusCode)
	}

	// Budget exhaustion → 429.
	srv.Config.LLMDailyTokens = 100
	res = postJSON(t, ts.URL+"/api/llm/transcribe", map[string]any{"source_id": scan.ID})
	res.Body.Close()
	if res.StatusCode != http.StatusTooManyRequests {
		t.Errorf("over budget: %d, want 429", res.StatusCode)
	}
}
