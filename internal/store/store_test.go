package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmd184psu/grocery-list/internal/model"
	"github.com/cmd184psu/grocery-list/internal/store"
)

// newTempStore creates a Store backed by a temp file; caller must clean up.
func newTempStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "items.json")
	s, err := store.New(path)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	return s, path
}

// ── Add / List ─────────────────────────────────────────────────────

func TestAdd_DefaultsToNeeded(t *testing.T) {
	s, _ := newTempStore(t)
	item, err := s.Add("Milk", "Dairy")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if item.State != model.StateNeeded {
		t.Errorf("got state %q, want %q", item.State, model.StateNeeded)
	}
	if item.Completed {
		t.Error("new item should not be completed")
	}
}

func TestAdd_AppearsInList(t *testing.T) {
	s, _ := newTempStore(t)
	s.Add("Eggs", "Dairy")
	s.Add("Bread", "Bakery")
	items := s.List()
	if len(items) != 2 {
		t.Fatalf("List: want 2 items, got %d", len(items))
	}
}

// ── Patch ──────────────────────────────────────────────────────

func TestPatch_StateChange(t *testing.T) {
	s, _ := newTempStore(t)
	item, _ := s.Add("Butter", "Dairy")

	s2 := model.StateCheck
	updated, err := s.Patch(item.ID, store.PatchPayload{State: &s2})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if updated.State != model.StateCheck {
		t.Errorf("got %q, want %q", updated.State, model.StateCheck)
	}
}

func TestPatch_CompletedToggle(t *testing.T) {
	s, _ := newTempStore(t)
	item, _ := s.Add("Yogurt", "Dairy")
	true_ := true
	updated, _ := s.Patch(item.ID, store.PatchPayload{Completed: &true_})
	if !updated.Completed {
		t.Error("expected completed=true")
	}
}

func TestPatch_NotFound(t *testing.T) {
	s, _ := newTempStore(t)
	_, err := s.Patch("nonexistent", store.PatchPayload{})
	if err == nil {
		t.Error("expected error for unknown id")
	}
}

// ── Delete ─────────────────────────────────────────────────────

func TestDelete_RemovesItem(t *testing.T) {
	s, _ := newTempStore(t)
	item, _ := s.Add("Cheese", "Dairy")
	if err := s.Delete(item.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(s.List()) != 0 {
		t.Error("list should be empty after delete")
	}
}

func TestDelete_NotFound(t *testing.T) {
	s, _ := newTempStore(t)
	if err := s.Delete("ghost"); err == nil {
		t.Error("expected error for unknown id")
	}
}

// ── Reset ──────────────────────────────────────────────────────

func TestReset_ClearsCompletedAndSetsCheck(t *testing.T) {
	s, _ := newTempStore(t)

	item1, _ := s.Add("Apple", "Produce")
	item2, _ := s.Add("Banana", "Produce")

	true_ := true
	ns := model.StateNotNeeded
	s.Patch(item1.ID, store.PatchPayload{Completed: &true_, State: &ns})
	s.Patch(item2.ID, store.PatchPayload{Completed: &true_})

	result, err := s.Reset()
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	for _, it := range result {
		if it.Completed {
			t.Errorf("item %s: completed should be false after reset", it.ID)
		}
		if it.State != model.StateCheck {
			t.Errorf("item %s: state should be %q, got %q", it.ID, model.StateCheck, it.State)
		}
	}
}

// ── Groups ─────────────────────────────────────────────────────

func TestSaveGroups_OrphansItemsToNoGroup(t *testing.T) {
	s, _ := newTempStore(t)
	s.SaveGroups([]string{"Produce", "Dairy"})

	item, _ := s.Add("Carrot", "Produce")

	// Remove "Produce" — Carrot should move to NoGroup.
	if err := s.SaveGroups([]string{"Dairy"}); err != nil {
		t.Fatalf("SaveGroups: %v", err)
	}

	items := s.List()
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].ID != item.ID {
		t.Error("wrong item returned")
	}
	if items[0].Group != model.NoGroup {
		t.Errorf("item group: got %q, want %q", items[0].Group, model.NoGroup)
	}
}

func TestSaveGroups_ItemsInOtherGroupsUnaffected(t *testing.T) {
	s, _ := newTempStore(t)
	s.SaveGroups([]string{"Produce", "Dairy"})
	s.Add("Milk", "Dairy")
	s.Add("Carrot", "Produce")

	// Remove only Produce.
	s.SaveGroups([]string{"Dairy"})

	for _, it := range s.List() {
		if it.Name == "Milk" && it.Group != "Dairy" {
			t.Errorf("Milk should stay in Dairy, got %q", it.Group)
		}
	}
}

func TestSaveGroups_NoGroupNameReserved(t *testing.T) {
	s, _ := newTempStore(t)
	item, _ := s.Add("Orphan", model.NoGroup)

	s.SaveGroups([]string{"Dairy"})

	var found bool
	for _, it := range s.List() {
		if it.ID == item.ID {
			found = true
			if it.Group != model.NoGroup {
				t.Errorf("orphan group: got %q, want %q", it.Group, model.NoGroup)
			}
		}
	}
	if !found {
		t.Error("orphaned item disappeared from list")
	}
}

// ── Persistence ───────────────────────────────────────────────────

func TestPersistence_RoundTrip(t *testing.T) {
	s, path := newTempStore(t)
	s.SaveGroups([]string{"Frozen"})
	s.Add("Ice Cream", "Frozen")

	s2, err := store.New(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	items := s2.List()
	if len(items) != 1 {
		t.Fatalf("want 1 item after reload, got %d", len(items))
	}
	if items[0].Name != "Ice Cream" {
		t.Errorf("got %q, want %q", items[0].Name, "Ice Cream")
	}
	groups := s2.Groups()
	if len(groups) != 1 || groups[0] != "Frozen" {
		t.Errorf("groups after reload: %v", groups)
	}
}

func TestPersistence_LegacyArrayFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.json")
	legacy := `[{"id":"1","name":"OldItem","group":"A","state":"needed",
		"completed":false,"order":0,"created_at":"2024-01-01T00:00:00Z"}]`
	os.WriteFile(path, []byte(legacy), 0644)

	s, err := store.New(path)
	if err != nil {
		t.Fatalf("load legacy: %v", err)
	}
	items := s.List()
	if len(items) != 1 || items[0].Name != "OldItem" {
		t.Errorf("legacy load failed: %v", items)
	}
}

// ── Reorder / Move ───────────────────────────────────────────────

func TestReorder_SetsOrder(t *testing.T) {
	s, _ := newTempStore(t)
	a, _ := s.Add("A", "G")
	b, _ := s.Add("B", "G")
	c, _ := s.Add("C", "G")

	if err := s.Reorder("G", []string{c.ID, b.ID, a.ID}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	items := s.List()
	orderMap := map[string]int{}
	for _, it := range items {
		orderMap[it.Name] = it.Order
	}
	if !(orderMap["C"] < orderMap["B"] && orderMap["B"] < orderMap["A"]) {
		t.Errorf("unexpected order map: %v", orderMap)
	}
}

func TestMove_ChangesGroup(t *testing.T) {
	s, _ := newTempStore(t)
	item, _ := s.Add("Spinach", "Produce")

	moved, err := s.Move(item.ID, store.MovePayload{Group: "Frozen", OrderIDs: []string{item.ID}})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if moved.Group != "Frozen" {
		t.Errorf("got group %q, want Frozen", moved.Group)
	}
}

// ── Revision ────────────────────────────────────────────────────────────

func TestRevision_IncrementsOnMutation(t *testing.T) {
	s, _ := newTempStore(t)

	r0 := s.Revision()

	item, _ := s.Add("Milk", "Dairy")
	r1 := s.Revision()
	if r1 <= r0 {
		t.Errorf("revision should increase after Add: %d → %d", r0, r1)
	}

	ns := model.StateCheck
	s.Patch(item.ID, store.PatchPayload{State: &ns})
	r2 := s.Revision()
	if r2 <= r1 {
		t.Errorf("revision should increase after Patch: %d → %d", r1, r2)
	}

	s.Delete(item.ID)
	r3 := s.Revision()
	if r3 <= r2 {
		t.Errorf("revision should increase after Delete: %d → %d", r2, r3)
	}
}
