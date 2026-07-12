package api

import (
	"net/http"
	"testing"
)

func deleteURL(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestProjectLifecycle drives the full cycle: create -> list -> patch ->
// delete -> 404 on further access.
func TestProjectLifecycle(t *testing.T) {
	ts, _, _ := newTestServer(t)

	res := postJSON(t, ts.URL+"/api/projects", map[string]any{
		"name": "ML Course", "dirs": []string{"ml"}, "deadline": "2026-12-31",
	})
	if res.StatusCode != 200 {
		t.Fatalf("create: %d", res.StatusCode)
	}
	created := decode[ProjectInfo](t, res)
	if created.Name != "ML Course" || len(created.Dirs) != 1 || created.Dirs[0] != "ml" {
		t.Fatalf("created = %+v", created)
	}
	if created.Deadline != "2026-12-31" {
		t.Fatalf("deadline = %q", created.Deadline)
	}
	if created.DaysLeft == nil {
		t.Fatal("days_left is nil, want a value")
	}
	if created.TotalCards != 0 || created.NewCards != 0 || created.DueNow != 0 {
		t.Errorf("stats on a fresh project = %+v", created)
	}

	// List includes it.
	list := decode[[]ProjectInfo](t, mustGet(t, ts.URL+"/api/projects"))
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v", list)
	}

	// Patch: rename and clear the deadline, leave dirs alone.
	res = patchJSON(t, ts.URL+"/api/projects/"+itoa(created.ID),
		map[string]any{"name": "ML Course v2", "deadline": ""})
	if res.StatusCode != 200 {
		t.Fatalf("patch: %d", res.StatusCode)
	}
	patched := decode[ProjectInfo](t, res)
	if patched.Name != "ML Course v2" {
		t.Errorf("patched name = %q", patched.Name)
	}
	if patched.Deadline != "" || patched.DaysLeft != nil {
		t.Errorf("patched deadline = %q, days_left = %v, want cleared", patched.Deadline, patched.DaysLeft)
	}
	if len(patched.Dirs) != 1 || patched.Dirs[0] != "ml" {
		t.Errorf("dirs changed unexpectedly: %v", patched.Dirs)
	}

	// Delete.
	res = deleteURL(t, ts.URL+"/api/projects/"+itoa(created.ID))
	if res.StatusCode != 200 {
		t.Fatalf("delete: %d", res.StatusCode)
	}
	deleted := decode[map[string]int64](t, res)
	if deleted["deleted"] != created.ID {
		t.Errorf("deleted = %+v", deleted)
	}

	// A further patch on the deleted id is a 404.
	res = patchJSON(t, ts.URL+"/api/projects/"+itoa(created.ID), map[string]any{"name": "Ghost"})
	if res.StatusCode != 404 {
		t.Fatalf("patch after delete: %d, want 404", res.StatusCode)
	}
}

// TestProjectCreateBadDeadline rejects an unparseable deadline.
func TestProjectCreateBadDeadline(t *testing.T) {
	ts, _, _ := newTestServer(t)
	res := postJSON(t, ts.URL+"/api/projects", map[string]any{
		"name": "X", "dirs": []string{"a"}, "deadline": "not-a-date",
	})
	if res.StatusCode != 400 {
		t.Fatalf("create with bad deadline: %d, want 400", res.StatusCode)
	}
}

// TestProjectCreateNoDirs rejects a project with no directories.
func TestProjectCreateNoDirs(t *testing.T) {
	ts, _, _ := newTestServer(t)
	res := postJSON(t, ts.URL+"/api/projects", map[string]any{
		"name": "X", "dirs": []string{},
	})
	if res.StatusCode != 400 {
		t.Fatalf("create with no dirs: %d, want 400", res.StatusCode)
	}
}

// TestProjectPatchUnknownID returns 404 for an id that never existed.
func TestProjectPatchUnknownID(t *testing.T) {
	ts, _, _ := newTestServer(t)
	res := patchJSON(t, ts.URL+"/api/projects/999999", map[string]any{"name": "X"})
	if res.StatusCode != 404 {
		t.Fatalf("patch unknown id: %d, want 404", res.StatusCode)
	}
}

// TestProjectDeleteUnknownID returns 404 for an id that never existed.
func TestProjectDeleteUnknownID(t *testing.T) {
	ts, _, _ := newTestServer(t)
	res := deleteURL(t, ts.URL+"/api/projects/999999")
	if res.StatusCode != 404 {
		t.Fatalf("delete unknown id: %d, want 404", res.StatusCode)
	}
}

// TestProjectPatchInvalidID rejects a non-numeric id path segment.
func TestProjectPatchInvalidID(t *testing.T) {
	ts, _, _ := newTestServer(t)
	res := patchJSON(t, ts.URL+"/api/projects/abc", map[string]any{"name": "X"})
	if res.StatusCode != 400 {
		t.Fatalf("patch invalid id: %d, want 400", res.StatusCode)
	}
}
