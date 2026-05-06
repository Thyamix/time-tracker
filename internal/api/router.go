package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
	"timetrack/internal/db"
	"timetrack/internal/models"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	db *db.DB
}

func NewServer(database *db.DB) *Server {
	return &Server{db: database}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == "OPTIONS" {
				w.WriteHeader(204)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	// Waybar / rofi status
	r.Get("/api/status", s.getStatus)

	// Projects
	r.Get("/api/projects", s.getProjects)
	r.Post("/api/projects", s.createProject)
	r.Patch("/api/projects/{id}", s.updateProject)
	r.Delete("/api/projects/{id}", s.deleteProject)

	// Sessions
	r.Get("/api/sessions", s.getSessions)
	r.Post("/api/sessions", s.createSession)
	r.Patch("/api/sessions/{id}", s.updateSession)
	r.Delete("/api/sessions/{id}", s.deleteSession)

	// Tracking
	r.Post("/api/track/start", s.startTracking)
	r.Post("/api/track/stop", s.stopTracking)

	// Stats
	r.Get("/api/stats", s.getStats)

	// Import
	r.Post("/api/import", s.importSessions)

	return r
}

// ── Status ────────────────────────────────────────────────────────────────────

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	active, err := s.db.GetActiveSession()
	if err != nil {
		jsonError(w, err, 500)
		return
	}

	status := &models.Status{}

	if active == nil {
		status.WaybarText = ""
		status.WaybarTooltip = "No active session"
		status.WaybarClass = "inactive"
		jsonOK(w, status)
		return
	}

	status.Active = true
	status.Session = active
	status.Elapsed = int64(time.Since(active.Start).Seconds())

	path, _ := s.db.GetProjectPath(active.ProjectID)
	status.Path = path

	total, _ := s.db.GetProjectTotalWithActive(active.ProjectID)
	status.Total = total

	elapsed := status.Elapsed
	h := elapsed / 3600
	m := (elapsed % 3600) / 60

	var sessionStr string
	if h > 0 {
		sessionStr = fmt.Sprintf("%dh %dm", h, m)
	} else {
		sessionStr = fmt.Sprintf("%dm", m)
	}

	th := total / 3600
	tm := (total % 3600) / 60
	var totalStr string
	if th > 0 {
		totalStr = fmt.Sprintf("%dh %dm", th, tm)
	} else {
		totalStr = fmt.Sprintf("%dm", tm)
	}

	status.WaybarText = fmt.Sprintf("⏱ %s  %s (%s)", path, sessionStr, totalStr)
	status.WaybarTooltip = fmt.Sprintf("Tracking: %s\nClick to stop", path)
	status.WaybarClass = "active"

	jsonOK(w, status)
}

// ── Projects ──────────────────────────────────────────────────────────────────

func (s *Server) getProjects(w http.ResponseWriter, r *http.Request) {
	flat, err := s.db.GetAllProjects()
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	totals, err := s.db.GetProjectTotals()
	if err != nil {
		jsonError(w, err, 500)
		return
	}

	// Add active session to totals
	active, _ := s.db.GetActiveSession()
	if active != nil {
		elapsed := int64(time.Since(active.Start).Seconds())
		totals[active.ProjectID] += elapsed
	}

	tree := db.BuildTree(flat, totals)
	jsonOK(w, tree)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		ParentID *int64 `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		jsonError(w, fmt.Errorf("name required"), 400)
		return
	}
	p, err := s.db.CreateProject(body.Name, body.ParentID)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonOK(w, p)
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	id := paramInt(r, "id")
	var body struct {
		Name     string `json:"name"`
		ParentID *int64 `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, err, 400)
		return
	}
	if err := s.db.UpdateProject(id, body.Name, body.ParentID); err != nil {
		jsonError(w, err, 500)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	id := paramInt(r, "id")
	if err := s.db.DeleteProject(id); err != nil {
		jsonError(w, err, 500)
		return
	}
	w.WriteHeader(204)
}

// ── Sessions ──────────────────────────────────────────────────────────────────

func (s *Server) getSessions(w http.ResponseWriter, r *http.Request) {
	var projectID *int64
	if p := r.URL.Query().Get("project_id"); p != "" {
		v, _ := strconv.ParseInt(p, 10, 64)
		projectID = &v
	}
	var from, to *time.Time
	if f := r.URL.Query().Get("from"); f != "" {
		v, _ := time.Parse(time.RFC3339, f)
		from = &v
	}
	if t := r.URL.Query().Get("to"); t != "" {
		v, _ := time.Parse(time.RFC3339, t)
		to = &v
	}
	sessions, err := s.db.GetSessions(projectID, from, to)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	if sessions == nil {
		sessions = []*models.Session{}
	}
	jsonOK(w, sessions)
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProjectID int64      `json:"project_id"`
		Start     time.Time  `json:"start"`
		End       *time.Time `json:"end"`
		Note      string     `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, err, 400)
		return
	}
	sess, err := s.db.CreateSession(body.ProjectID, body.Start, body.End, body.Note)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonOK(w, sess)
}

func (s *Server) updateSession(w http.ResponseWriter, r *http.Request) {
	id := paramInt(r, "id")
	var body struct {
		Start time.Time  `json:"start"`
		End   *time.Time `json:"end"`
		Note  string     `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, err, 400)
		return
	}
	if err := s.db.UpdateSession(id, body.Start, body.End, body.Note); err != nil {
		jsonError(w, err, 500)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	id := paramInt(r, "id")
	if err := s.db.DeleteSession(id); err != nil {
		jsonError(w, err, 500)
		return
	}
	w.WriteHeader(204)
}

// ── Tracking ──────────────────────────────────────────────────────────────────

func (s *Server) startTracking(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProjectID int64 `json:"project_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProjectID == 0 {
		jsonError(w, fmt.Errorf("project_id required"), 400)
		return
	}
	sess, err := s.db.StartTracking(body.ProjectID)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonOK(w, sess)
}

func (s *Server) stopTracking(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Note string `json:"note"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	sess, err := s.db.StopTracking(body.Note)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	if sess == nil {
		jsonError(w, fmt.Errorf("no active session"), 404)
		return
	}
	jsonOK(w, sess)
}

// ── Stats ─────────────────────────────────────────────────────────────────────

func (s *Server) getStats(w http.ResponseWriter, r *http.Request) {
	rangeParam := r.URL.Query().Get("range")
	var from, to *time.Time

	now := time.Now()
	switch rangeParam {
	case "today":
		t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		from = &t
	case "week":
		t := now.AddDate(0, 0, -7)
		from = &t
	case "month":
		t := now.AddDate(0, -1, 0)
		from = &t
	}

	perProject, err := s.db.GetStats(from, to)
	if err != nil {
		jsonError(w, err, 500)
		return
	}

	// Get project tree for context
	flat, _ := s.db.GetAllProjects()
	tree := db.BuildTree(flat, perProject)

	jsonOK(w, map[string]any{
		"range":    rangeParam,
		"projects": tree,
		"raw":      perProject,
	})
}

// ── Import ────────────────────────────────────────────────────────────────────

func (s *Server) importSessions(w http.ResponseWriter, r *http.Request) {
	var sessions []models.ImportSession
	if err := json.NewDecoder(r.Body).Decode(&sessions); err != nil {
		jsonError(w, err, 400)
		return
	}

	// Cache project name → id
	projectCache := make(map[string]int64)
	imported := 0

	for _, imp := range sessions {
		pid, ok := projectCache[imp.Project]
		if !ok {
			p, err := s.db.CreateProject(imp.Project, nil)
			if err != nil {
				jsonError(w, err, 500)
				return
			}
			pid = p.ID
			projectCache[imp.Project] = pid
		}
		end := time.Unix(imp.End, 0)
		s.db.CreateSession(pid, time.Unix(imp.Start, 0), &end, "")
		imported++
	}

	jsonOK(w, map[string]any{"imported": imported})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func paramInt(r *http.Request, key string) int64 {
	v, _ := strconv.ParseInt(chi.URLParam(r, key), 10, 64)
	return v
}
