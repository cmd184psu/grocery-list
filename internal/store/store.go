package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cmd184psu/grocery-list/internal/model"
)

// storeData is the on-disk format.
type storeData struct {
	Groups []string      `json:"groups,omitempty"`
	Items  []*model.Item `json:"items"`
}

// Store is a thread-safe, JSON-backed item/group store.
type Store struct {
	mu       sync.RWMutex
	items    map[string]*model.Item
	groups   []string
	filePath string
	revision int64 // atomically incremented on every save
}

// Revision returns the current monotonic write counter.
// Clients can compare this value to detect server-side changes cheaply.
func (s *Store) Revision() int64 {
	return atomic.LoadInt64(&s.revision)
}

// New creates (or loads) a Store backed by filePath.
func New(filePath string) (*Store, error) {
	s := &Store{items: make(map[string]*model.Item), filePath: filePath}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("loading store: %w", err)
	}
	return s, nil
}

// Groups returns the current named group list (excludes virtual NoGroup).
func (s *Store) Groups() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string{}, s.groups...)
}

// SaveGroups persists an updated group list and orphans items from removed groups.
func (s *Store) SaveGroups(groups []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Build a set of valid groups for fast lookup.
	valid := make(map[string]bool, len(groups))
	for _, g := range groups {
		valid[g] = true
	}
	// Items whose group is no longer in the list fall into NoGroup.
	for _, item := range s.items {
		if item.Group != model.NoGroup && !valid[item.Group] {
			item.Group = model.NoGroup
		}
	}
	s.groups = append([]string{}, groups...)
	return s.save()
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	// Support legacy plain-array format.
	if len(raw) > 0 && raw[0] == '[' {
		var items []*model.Item
		if err := json.Unmarshal(raw, &items); err != nil {
			return err
		}
		for _, item := range items {
			s.items[item.ID] = item
		}
		return nil
	}
	var sd storeData
	if err := json.Unmarshal(raw, &sd); err != nil {
		return err
	}
	for _, item := range sd.Items {
		s.items[item.ID] = item
	}
	s.groups = sd.Groups
	return nil
}

func (s *Store) save() error {
	sd := storeData{
		Groups: s.groups,
		Items:  s.sortedUnsafe(),
	}
	data, err := json.MarshalIndent(sd, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.filePath); err != nil {
		return err
	}
	atomic.AddInt64(&s.revision, 1)
	return nil
}

// sortedUnsafe must be called with the lock already held.
func (s *Store) sortedUnsafe() []*model.Item {
	items := make([]*model.Item, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Group != items[j].Group {
			return items[i].Group < items[j].Group
		}
		if items[i].Order != items[j].Order {
			return items[i].Order < items[j].Order
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items
}

// List returns all items in stable sort order.
func (s *Store) List() []*model.Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sortedUnsafe()
}

// Add creates a new item and persists it.
func (s *Store) Add(name, group string) (*model.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	maxOrder := 0
	for _, item := range s.items {
		if item.Group == group && item.Order >= maxOrder {
			maxOrder = item.Order + 1
		}
	}
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0755); err != nil {
		return nil, err
	}
	item := &model.Item{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Name:      name,
		Group:     group,
		State:     model.StateNeeded,
		Completed: false,
		Order:     maxOrder,
		CreatedAt: time.Now(),
	}
	s.items[item.ID] = item
	return item, s.save()
}

// PatchPayload carries optional fields for a partial update.
type PatchPayload struct {
	State     *model.ItemState `json:"state,omitempty"`
	Completed *bool            `json:"completed,omitempty"`
}

// Patch applies a partial update to an item by ID.
func (s *Store) Patch(id string, p PatchPayload) (*model.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return nil, fmt.Errorf("item not found: %s", id)
	}
	if p.State != nil {
		item.State = *p.State
	}
	if p.Completed != nil {
		item.Completed = *p.Completed
	}
	return item, s.save()
}

// Delete removes an item by ID.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return fmt.Errorf("item not found: %s", id)
	}
	delete(s.items, id)
	return s.save()
}

// MovePayload carries the destination group and desired order of IDs.
type MovePayload struct {
	Group    string   `json:"group"`
	OrderIDs []string `json:"order_ids"`
}

// Move changes an item's group and re-orders the destination group.
func (s *Store) Move(id string, p MovePayload) (*model.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return nil, fmt.Errorf("item not found: %s", id)
	}
	item.Group = p.Group
	for i, oid := range p.OrderIDs {
		if it, ok := s.items[oid]; ok {
			it.Order = i
		}
	}
	return item, s.save()
}

// Reorder sets the Order field for each item in a group according to the
// provided slice of IDs.
func (s *Store) Reorder(group string, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, id := range ids {
		if item, ok := s.items[id]; ok && item.Group == group {
			item.Order = i
		}
	}
	return s.save()
}

// BulkSync merges an incoming slice of items (offline edits) into the store.
func (s *Store) BulkSync(incoming []*model.Item) ([]*model.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, inc := range incoming {
		if existing, ok := s.items[inc.ID]; ok {
			existing.State     = inc.State
			existing.Completed = inc.Completed
			existing.Group     = inc.Group
			existing.Order     = inc.Order
		} else {
			s.items[inc.ID] = inc
		}
	}
	if err := s.save(); err != nil {
		return nil, err
	}
	return s.sortedUnsafe(), nil
}

// Reset sets every item to completed=false, state="check" and persists.
func (s *Store) Reset() ([]*model.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		item.Completed = false
		item.State     = model.StateCheck
	}
	if err := s.save(); err != nil {
		return nil, err
	}
	return s.sortedUnsafe(), nil
}
