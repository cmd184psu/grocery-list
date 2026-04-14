package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cmd184psu/grocery-list/internal/api"
	"github.com/cmd184psu/grocery-list/internal/model"
	"github.com/cmd184psu/grocery-list/internal/store"
)

// ── test harness ─────────────────────────────────────────────────────────

type harness struct {
	s   *store.Store
	h   *api.Handler
	mux *http.ServeMux
}

func newHarness(t *testing.T, groups []string) *harness {
	t.Helper()
	dir := t.TempDir()
	s, err := store.New(filepath.Join(dir, "items.json"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if len(groups) > 0 {
		s.SaveGroups(groups)
	}
	h := api.NewHandler(s, groups)
	mux := http.NewServeMux()
	h.Register(mux)
	return &harness{s: s, h: h, mux: mux}
}

func (hh *harness) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	hh.mux.ServeHTTP(w, req)
	return w
}

func decodeJSON[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decodeJSON: %v (body: %s)", err, w.Body.String())
	}
	return out
}

// ── /api/items ──────────────────────────────────────────────────────────

func TestHandlerGetItems_Empty(t *testing.T) {
	hh := newHarness(t, nil)
	w := hh.do(t, http.MethodGet, "/api/items", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var items []model.Item
	json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 0 {
		t.Errorf("want empty list, got %d items", len(items))
	}
}

func TestHandlerPostItem_Created(t *testing.T) {
	hh := newHarness(t, []string{"Dairy"})
	w := hh.do(t, http.MethodPost, "/api/items", map[string]string{"name": "Milk", "group": "Dairy"})
	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d — %s", w.Code, w.Body.String())
	}
	item := decodeJSON[model.Item](t, w)
	if item.Name != "Milk" {
		t.Errorf("got name %q, want Milk", item.Name)
	}
	if item.State != model.StateNeeded {
		t.Errorf("got state %q, want needed", item.State)
	}
}

func TestHandlerPostItem_MissingName(t *testing.T) {
	hh := newHarness(t, nil)
	w := hh.do(t, http.MethodPost, "/api/items", map[string]string{"name": ""})
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestHandlerPatchItem_StateChange(t *testing.T) {
	hh := newHarness(t, []string{"Produce"})
	createW := hh.do(t, http.MethodPost, "/api/items",
		map[string]string{"name": "Carrot", "group": "Produce"})
	item := decodeJSON[model.Item](t, createW)

	w := hh.do(t, http.MethodPatch, "/api/items/"+item.ID,
		map[string]string{"state": "check"})
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	updated := decodeJSON[model.Item](t, w)
	if updated.State != model.StateCheck {
		t.Errorf("got %q, want check", updated.State)
	}
}

func TestHandlerDeleteItem(t *testing.T) {
	hh := newHarness(t, []string{"Produce"})
	createW := hh.do(t, http.MethodPost, "/api/items",
		map[string]string{"name": "Kale", "group": "Produce"})
	item := decodeJSON[model.Item](t, createW)

	w := hh.do(t, http.MethodDelete, "/api/items/"+item.ID, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}
	listW := hh.do(t, http.MethodGet, "/api/items", nil)
	var items []model.Item
	json.NewDecoder(listW.Body).Decode(&items)
	if len(items) != 0 {
		t.Errorf("want 0 items after delete, got %d", len(items))
	}
}

// ── /api/reset ──────────────────────────────────────────────────────────

func TestHandlerReset(t *testing.T) {
	hh := newHarness(t, []string{"Produce"})
	hh.do(t, http.MethodPost, "/api/items",
		map[string]string{"name": "Tomato", "group": "Produce"})

	w := hh.do(t, http.MethodPost, "/api/reset", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var items []model.Item
	json.NewDecoder(w.Body).Decode(&items)
	for _, it := range items {
		if it.Completed {
			t.Errorf("item %s still completed after reset", it.ID)
		}
		if it.State != model.StateCheck {
			t.Errorf("item %s state %q after reset, want check", it.ID, it.State)
		}
	}
}

func TestHandlerReset_WrongMethod(t *testing.T) {
	hh := newHarness(t, nil)
	w := hh.do(t, http.MethodGet, "/api/reset", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", w.Code)
	}
}

// ── /api/config/groups ────────────────────────────────────────────────

func TestHandlerGroupsAdd(t *testing.T) {
	hh := newHarness(t, nil)
	w := hh.do(t, http.MethodPost, "/api/config/groups",
		map[string]string{"name": "Frozen"})
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	groups := resp["groups"].([]any)
	if len(groups) != 1 || groups[0] != "Frozen" {
		t.Errorf("unexpected groups: %v", groups)
	}
}

func TestHandlerGroupsAdd_Idempotent(t *testing.T) {
	hh := newHarness(t, []string{"Frozen"})
	w := hh.do(t, http.MethodPost, "/api/config/groups",
		map[string]string{"name": "Frozen"})
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	groups := resp["groups"].([]any)
	if len(groups) != 1 {
		t.Errorf("want 1 group (idempotent), got %d", len(groups))
	}
}

func TestHandlerGroupsAdd_ReservedNoGroup(t *testing.T) {
	hh := newHarness(t, nil)
	w := hh.do(t, http.MethodPost, "/api/config/groups",
		map[string]string{"name": model.NoGroup})
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for reserved name, got %d", w.Code)
	}
}

func TestHandlerGroupsRemove_OrphansItems(t *testing.T) {
	hh := newHarness(t, []string{"Produce", "Dairy"})
	// Add an item to Produce.
	hh.do(t, http.MethodPost, "/api/items",
		map[string]string{"name": "Spinach", "group": "Produce"})

	// Remove Produce.
	w := hh.do(t, http.MethodPost, "/api/config/groups/remove",
		map[string]string{"name": "Produce"})
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — %s", w.Code, w.Body.String())
	}

	var resp struct {
		Groups []string     `json:"groups"`
		Items  []model.Item `json:"items"`
	}
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Groups) != 1 || resp.Groups[0] != "Dairy" {
		t.Errorf("unexpected groups after remove: %v", resp.Groups)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("want 1 item in response, got %d", len(resp.Items))
	}
	if resp.Items[0].Group != model.NoGroup {
		t.Errorf("item group: got %q, want %q", resp.Items[0].Group, model.NoGroup)
	}
}

func TestHandlerGroupsRemove_WrongMethod(t *testing.T) {
	hh := newHarness(t, nil)
	w := hh.do(t, http.MethodDelete, "/api/config/groups/remove", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", w.Code)
	}
}

// ── /api/config ─────────────────────────────────────────────────────────

func TestHandlerGetConfig(t *testing.T) {
	hh := newHarness(t, []string{"Bakery"})
	w := hh.do(t, http.MethodGet, "/api/config", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	groups, ok := resp["groups"].([]any)
	if !ok || len(groups) != 1 || groups[0] != "Bakery" {
		t.Errorf("unexpected config: %v", resp)
	}
}

// Unused import guard.
var _ = os.DevNull
