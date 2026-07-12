package store

import "testing"

func TestResolveNoteLinks(t *testing.T) {
	s := openTestStore(t)

	notes := []NoteRow{
		{Path: "DL/extended-material/00_Glossary.md", Title: "Glossary"},
		{Path: "DL/extended-material/concepts/Perceptron.md", Title: "Perceptron"},
		{Path: "EIS/intro.md", Title: "Intro to EIS"},
	}
	for _, n := range notes {
		if err := s.UpsertNote(n); err != nil {
			t.Fatal(err)
		}
	}

	targets := []string{
		"Perceptron",                     // bare stem, cross-dir
		"concepts/Perceptron",            // partial path from another folder
		"concepts/perceptron",            // partial path, case-insensitive
		"/concepts/Perceptron",           // leading slash
		"00_Glossary#Some Heading",       // heading fragment
		"Intro to EIS",                   // title
		"DL/extended-material/00_Glossary.md", // exact path
		"../../EIS/intro",                // relative target, cross-dir
		"./concepts/Perceptron",          // relative target, same dir
		"../../EIS/intro#Heading",        // relative target with fragment
		"../../../outside",               // escapes the vault root
		"concepts/Missing",               // unresolvable partial path
		"Nowhere",                        // unresolvable stem
	}
	if err := s.SetNoteLinks("DL/extended-material/00_Glossary.md", targets); err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveNoteLinks(); err != nil {
		t.Fatal(err)
	}

	links, err := s.NoteLinks("DL/extended-material/00_Glossary.md")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, l := range links {
		got[l.Target] = l.ToPath
	}

	perceptron := "DL/extended-material/concepts/Perceptron.md"
	glossary := "DL/extended-material/00_Glossary.md"
	want := map[string]string{
		"Perceptron":                           perceptron,
		"concepts/Perceptron":                  perceptron,
		"concepts/perceptron":                  perceptron,
		"/concepts/Perceptron":                 perceptron,
		"00_Glossary#Some Heading":             glossary,
		"Intro to EIS":                         "EIS/intro.md",
		"DL/extended-material/00_Glossary.md":  glossary,
		"../../EIS/intro":                      "EIS/intro.md",
		"./concepts/Perceptron":                perceptron,
		"../../EIS/intro#Heading":              "EIS/intro.md",
		"../../../outside":                     "",
		"concepts/Missing":                     "",
		"Nowhere":                              "",
	}
	for target, wantPath := range want {
		if got[target] != wantPath {
			t.Errorf("target %q resolved to %q, want %q", target, got[target], wantPath)
		}
	}
}

// The same relative target text must resolve per linking note, not once
// globally: "../local" means a different note from A/sub than from B/sub.
func TestResolveNoteLinksRelativePerNote(t *testing.T) {
	s := openTestStore(t)

	notes := []NoteRow{
		{Path: "A/local.md", Title: "A Local"},
		{Path: "A/sub/one.md", Title: "One"},
		{Path: "B/local.md", Title: "B Local"},
		{Path: "B/sub/two.md", Title: "Two"},
	}
	for _, n := range notes {
		if err := s.UpsertNote(n); err != nil {
			t.Fatal(err)
		}
	}
	for _, from := range []string{"A/sub/one.md", "B/sub/two.md"} {
		if err := s.SetNoteLinks(from, []string{"../local"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ResolveNoteLinks(); err != nil {
		t.Fatal(err)
	}

	for from, wantPath := range map[string]string{
		"A/sub/one.md": "A/local.md",
		"B/sub/two.md": "B/local.md",
	} {
		links, err := s.NoteLinks(from)
		if err != nil {
			t.Fatal(err)
		}
		if len(links) != 1 || links[0].ToPath != wantPath {
			t.Errorf("from %q: got %+v, want ../local -> %q", from, links, wantPath)
		}
	}
}
