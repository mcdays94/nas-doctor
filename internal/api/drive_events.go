package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcdays94/nas-doctor/internal/storage"
)

// registerDriveEventRoutes wires the four CRUD endpoints for the
// per-drive maintenance log (issue #130) onto the given router. The
// {slot_key} param is URL-path-encoded by the client; chi handles the
// decode automatically via URLParam.
func (s *Server) registerDriveEventRoutes(r chi.Router) {
	r.Get("/api/v1/drives/{slot_key}/events", s.handleListDriveEvents)
	r.Post("/api/v1/drives/{slot_key}/events", s.handleCreateDriveEvent)
	r.Put("/api/v1/drives/{slot_key}/events/{id}", s.handleUpdateDriveEvent)
	r.Delete("/api/v1/drives/{slot_key}/events/{id}", s.handleDeleteDriveEvent)
}

// handleListDriveEvents returns all events for the given slot_key,
// newest first.
func (s *Server) handleListDriveEvents(w http.ResponseWriter, r *http.Request) {
	slotKey := chi.URLParam(r, "slot_key")
	if strings.TrimSpace(slotKey) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slot_key required"})
		return
	}
	events, err := s.store.ListDriveEvents(slotKey)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if events == nil {
		events = []storage.DriveEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

// createDriveEventReq is the POST request body.
//
// event_time is optional; if omitted the server uses time.Now().UTC().
// content is required and must be non-empty.
type createDriveEventReq struct {
	EventTime string `json:"event_time,omitempty"`
	Content   string `json:"content"`
}

// handleCreateDriveEvent inserts a new manual (is_auto=0) event.
// The content is the freeform text the user typed in the Add Entry form.
func (s *Server) handleCreateDriveEvent(w http.ResponseWriter, r *http.Request) {
	slotKey := chi.URLParam(r, "slot_key")
	if strings.TrimSpace(slotKey) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slot_key required"})
		return
	}

	var req createDriveEventReq
	raw, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	defer r.Body.Close()
	if err := json.Unmarshal(raw, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content is required"})
		return
	}

	evTime := time.Now().UTC()
	if strings.TrimSpace(req.EventTime) != "" {
		parsed, err := time.Parse(time.RFC3339, req.EventTime)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid event_time; expected RFC3339"})
			return
		}
		evTime = parsed.UTC()
	}

	platform := s.currentPlatform()
	id, err := s.store.SaveDriveEvent(storage.DriveEvent{
		SlotKey:   slotKey,
		Platform:  platform,
		EventType: "note",
		EventTime: evTime,
		Content:   req.Content,
		IsAuto:    false,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	ev, err := s.store.GetDriveEvent(id)
	if err != nil || ev == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to re-read created event"})
		return
	}
	writeJSON(w, http.StatusCreated, ev)
}

// updateDriveEventReq is the PUT request body. Both fields are optional
// but at least one must be set; an empty body is a no-op 200.
type updateDriveEventReq struct {
	EventTime *string `json:"event_time,omitempty"`
	Content   *string `json:"content,omitempty"`
}

// handleUpdateDriveEvent modifies a manual event. Auto events (is_auto=1)
// return 403.
func (s *Server) handleUpdateDriveEvent(w http.ResponseWriter, r *http.Request) {
	slotKey := chi.URLParam(r, "slot_key")
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if strings.TrimSpace(slotKey) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slot_key required"})
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	defer r.Body.Close()

	var req updateDriveEventReq
	if err := json.Unmarshal(raw, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	var evTime *time.Time
	if req.EventTime != nil && strings.TrimSpace(*req.EventTime) != "" {
		parsed, err := time.Parse(time.RFC3339, *req.EventTime)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid event_time; expected RFC3339"})
			return
		}
		u := parsed.UTC()
		evTime = &u
	}

	if err := s.store.UpdateDriveEvent(slotKey, id, evTime, req.Content); err != nil {
		switch {
		case storage.IsDriveEventImmutableErr(err):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		case storage.IsDriveEventNotFoundErr(err):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	ev, err := s.store.GetDriveEvent(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if ev == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "event disappeared"})
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// handleDeleteDriveEvent removes a manual event. Auto events return 403.
func (s *Server) handleDeleteDriveEvent(w http.ResponseWriter, r *http.Request) {
	slotKey := chi.URLParam(r, "slot_key")
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if strings.TrimSpace(slotKey) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slot_key required"})
		return
	}
	if err := s.store.DeleteDriveEvent(slotKey, id); err != nil {
		switch {
		case storage.IsDriveEventImmutableErr(err):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		case storage.IsDriveEventNotFoundErr(err):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// currentPlatform returns the platform string captured by the latest
// snapshot, or "" if unknown. Used to stamp drive_events rows at
// creation time; downstream readers can filter or enrich by platform.
func (s *Server) currentPlatform() string {
	if s.scheduler != nil {
		if snap := s.scheduler.Latest(); snap != nil {
			return snap.System.Platform
		}
	}
	snap, _ := s.store.GetLatestSnapshot()
	if snap == nil {
		return ""
	}
	return snap.System.Platform
}
