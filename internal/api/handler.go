package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cmd184psu/grocery-list/internal/model"
	"github.com/cmd184psu/grocery-list/internal/store"
)

// Handler wires HTTP routes to the store.
type Handler struct {
	store    *store.Store
	groups   []string
	progress bool
	broker   *Broker
}

// NewHandler returns a Handler with an initial group list.
// broker is used to push SSE refresh events to connected clients after mutations.
func NewHandler(s *store.Store, groups []string, progress bool, broker *Broker) *Handler {
	return &Handler{store: s, groups: groups, progress: progress, broker: broker}
}

// Register mounts all API routes on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	// Config
	mux.HandleFunc("/api/config",                 h.handleConfig)
	mux.HandleFunc("/api/config/groups",          h.handleConfigGroupsAdd)
	mux.HandleFunc("/api/config/groups/remove",   h.handleConfigGroupsRemove)
	mux.HandleFunc("/api/config/groups/reorder",  h.handleConfigGroupsReorder)

	// Items
	mux.HandleFunc("/api/items",    h.handleItems)
	mux.HandleFunc("/api/items/",   h.handleItem)

	// Bulk operations
	mux.HandleFunc("/api/move",     h.handleMove)
	mux.HandleFunc("/api/reorder",  h.handleReorder)
	mux.HandleFunc("/api/sync",     h.handleSync)
	mux.HandleFunc("/api/reset",    h.handleReset)

	// Live sync
	mux.HandleFunc("/api/revision", h.handleRevision)
	mux.Handle("/api/events",       h.broker)
}

// writeJSON encodes v as JSON with the given HTTP status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error envelope.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// Wrap adds permissive CORS headers and handles preflight OPTIONS.
func Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin",  "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ── /api/config ────────────────────────────────────────────────────────────

// GET /api/config
func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": h.groups, "progress": h.progress})
}

// POST /api/config/groups  {"name":"Dairy"}  → add a group
func (h *Handler) handleConfigGroupsAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	name, ok := decodeName(w, r)
	if !ok {
		return
	}
	if name == model.NoGroup {
		writeError(w, http.StatusBadRequest, `"No Group" is a reserved name`)
		return
	}
	for _, g := range h.groups {
		if g == name {
			// Idempotent: already exists.
			writeJSON(w, http.StatusOK, map[string]any{"groups": h.groups})
			return
		}
	}
	h.groups = append(h.groups, name)
	if err := h.store.SaveGroups(h.groups); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": h.groups})
	h.broker.Notify()
}

// POST /api/config/groups/remove  {"name":"Dairy"}  → delete a group
// (Items whose group matches are moved to the virtual "No Group".)
func (h *Handler) handleConfigGroupsRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	name, ok := decodeName(w, r)
	if !ok {
		return
	}
	newGroups := make([]string, 0, len(h.groups))
	for _, g := range h.groups {
		if g != name {
			newGroups = append(newGroups, g)
		}
	}
	h.groups = newGroups
	// store.SaveGroups orphans items from the removed group → NoGroup.
	if err := h.store.SaveGroups(h.groups); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Return updated groups AND the full item list so the client can refresh.
	writeJSON(w, http.StatusOK, map[string]any{
		"groups": h.groups,
		"items":  h.store.List(),
	})
	h.broker.Notify()
}

// POST /api/config/groups/reorder  {"groups":[...]}  → persist new group order
func (h *Handler) handleConfigGroupsReorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Groups []string `json:"groups"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Groups) == 0 {
		writeError(w, http.StatusBadRequest, "groups array required")
		return
	}
	h.groups = body.Groups
	if err := h.store.SaveGroups(h.groups); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": h.groups})
	h.broker.Notify()
}

// decodeName reads {"name":"..."} from the request body.
func decodeName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return "", false
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return "", false
	}
	return name, true
}

// ── /api/items ─────────────────────────────────────────────────────────────

// GET  /api/items        → list all items
// POST /api/items        → {name, group} create item
func (h *Handler) handleItems(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, h.store.List())

	case http.MethodPost:
		var body struct {
			Name  string `json:"name"`
			Group string `json:"group"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
			strings.TrimSpace(body.Name) == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		if body.Group == "" {
			if len(h.groups) > 0 {
				body.Group = h.groups[0]
			} else {
				body.Group = model.NoGroup
			}
		}
		item, err := h.store.Add(strings.TrimSpace(body.Name), body.Group)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, item)
		h.broker.Notify()

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// PATCH  /api/items/:id  → partial update (state, completed)
// DELETE /api/items/:id  → remove item
func (h *Handler) handleItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/items/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var p store.PatchPayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		item, err := h.store.Patch(id, p)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
		h.broker.Notify()

	case http.MethodDelete:
		if err := h.store.Delete(id); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
		h.broker.Notify()

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ── Bulk operations ────────────────────────────────────────────────────────

// POST /api/move  {id, group, order_ids}
func (h *Handler) handleMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		ID string `json:"id"`
		store.MovePayload
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		writeError(w, http.StatusBadRequest, "id and group required")
		return
	}
	item, err := h.store.Move(body.ID, body.MovePayload)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
	h.broker.Notify()
}

// POST /api/reorder  {group, ids}
func (h *Handler) handleReorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Group string   `json:"group"`
		IDs   []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.store.Reorder(body.Group, body.IDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, h.store.List())
	h.broker.Notify()
}

// POST /api/sync  [...items]
func (h *Handler) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var items []*model.Item
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	merged, err := h.store.BulkSync(items)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, merged)
	h.broker.Notify()
}

// POST /api/reset  → all items: completed=false, state="check"
func (h *Handler) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	result, err := h.store.Reset()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
	h.broker.Notify()
}

// GET /api/revision → {"revision": N}
// Lightweight endpoint clients can poll to detect changes without fetching all items.
func (h *Handler) handleRevision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"revision": h.store.Revision()})
}
