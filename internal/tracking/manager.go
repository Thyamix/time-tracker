package tracking

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"timetrack/internal/db"
	"timetrack/internal/models"
)

type Manager struct {
	mu         sync.Mutex
	db         *db.DB
	state      models.TrackingState
	number     int
	checkpoint time.Time
	now        func() time.Time
}

func New(database *db.DB) (*Manager, error) {
	config, checkpoint, err := database.TrackingConfig()
	if err != nil {
		return nil, err
	}
	m := &Manager{db: database, state: models.TrackingState{TrackingConfig: config}, number: -1, checkpoint: time.Unix(checkpoint, 0), now: time.Now}
	// Workspace timers cannot include time while the server was not observing Sway.
	if config.Mode == "sway" {
		if err := database.ApplyTracking(nil, 0, m.checkpoint, "", false); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (m *Manager) Snapshot() (models.TrackingState, *models.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.state
	state.FocusedWorkspaceNumber = m.number
	state.Assignments = append([]models.WorkspaceAssignment{}, state.Assignments...)
	active, err := m.db.GetActiveSession()
	return state, active, err
}

func (m *Manager) target(config models.TrackingConfig) int64 {
	if config.Mode != "sway" || !config.Enabled || !m.state.Connected {
		return 0
	}
	// Numeric bindings also match named workspaces such as "8: code".
	for _, a := range config.Assignments {
		if a.Workspace == strconv.Itoa(m.number) {
			return a.ProjectID
		}
	}
	for _, a := range config.Assignments {
		if a.Workspace == m.state.FocusedWorkspace {
			return a.ProjectID
		}
	}
	return 0
}

func (m *Manager) Configure(config models.TrackingConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if config.Mode != "global" && config.Mode != "sway" {
		return fmt.Errorf("mode must be global or sway")
	}
	seen := make(map[string]bool)
	config.Assignments = append([]models.WorkspaceAssignment{}, config.Assignments...)
	for i := range config.Assignments {
		a := &config.Assignments[i]
		a.Workspace = strings.TrimSpace(a.Workspace)
		if n, err := strconv.Atoi(a.Workspace); err == nil {
			if n < 0 {
				return fmt.Errorf("workspace numbers must be nonnegative")
			}
			a.Workspace = strconv.Itoa(n)
		}
		if a.Workspace == "" || seen[a.Workspace] {
			return fmt.Errorf("workspace assignments must be nonempty and unique")
		}
		seen[a.Workspace] = true
		project, err := m.db.GetProject(a.ProjectID)
		if err != nil {
			return err
		}
		if project == nil {
			return fmt.Errorf("project %d not found", a.ProjectID)
		}
	}
	at := m.now()
	target := m.target(config)
	// Saving settings in global mode must not stop an existing global timer.
	if config.Mode == "global" && m.state.Mode == "global" {
		active, err := m.db.GetActiveSession()
		if err != nil {
			return err
		}
		if active != nil {
			target = active.ProjectID
		}
	}
	if err := m.db.ApplyTracking(&config, target, at, "", config.Mode != m.state.Mode); err != nil {
		return err
	}
	m.state.TrackingConfig = config
	m.checkpoint = at
	return nil
}

func (m *Manager) Observe(name string, number int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Connected = true
	m.state.FocusedWorkspace = name
	m.number = number
	if m.state.Mode != "sway" {
		return
	}
	at := m.now()
	// A suspended machine or stalled watcher is not observed working time.
	if at.Sub(m.checkpoint) > 3*time.Second {
		if err := m.db.ApplyTracking(nil, 0, m.checkpoint, "", false); err != nil {
			log.Printf("tracking: %v", err)
			return
		}
	}
	target := m.target(m.state.TrackingConfig)
	if target != 0 {
		project, err := m.db.GetProject(target)
		if err != nil {
			log.Printf("tracking: %v", err)
			return
		}
		if project == nil {
			target = 0
		}
	}
	if err := m.db.ApplyTracking(nil, target, at, "", false); err != nil {
		log.Printf("tracking: %v", err)
		return
	}
	m.checkpoint = at
}

func (m *Manager) Disconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Connected = false
	m.state.FocusedWorkspace = ""
	m.number = -1
	if m.state.Mode == "sway" {
		if err := m.db.ApplyTracking(nil, 0, m.checkpoint, "", false); err != nil {
			log.Printf("tracking disconnect: %v", err)
		}
	}
}

func (m *Manager) Start(projectID int64) (*models.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Mode == "sway" {
		return nil, fmt.Errorf("workspace tracking is automatic; use assignments or switch to global mode")
	}
	if err := m.db.ApplyTracking(nil, projectID, m.now(), "", true); err != nil {
		return nil, err
	}
	return m.db.GetActiveSession()
}

func (m *Manager) Stop(note string) (*models.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	active, err := m.db.GetActiveSession()
	if err != nil {
		return nil, err
	}
	config := m.state.TrackingConfig
	if config.Mode == "sway" {
		config.Enabled = false
	}
	at := m.now()
	if err := m.db.ApplyTracking(&config, 0, at, note, false); err != nil {
		return nil, err
	}
	m.state.TrackingConfig = config
	m.checkpoint = at
	if active != nil {
		active.End = &at
		active.Note = note
	}
	return active, nil
}

// EditSession serializes manual edits with automatic focus transitions.
func (m *Manager) EditSession(id, projectID int64, start time.Time, end *time.Time, note string) (*models.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Mode == "sway" {
		if end == nil {
			return nil, fmt.Errorf("manual sessions need an end time in Sway mode")
		}
		if id != 0 {
			session, err := m.db.GetSession(id)
			if err != nil {
				return nil, err
			}
			if session != nil && session.End == nil {
				return nil, fmt.Errorf("pause workspace tracking before editing its active segment")
			}
		}
	}
	if id == 0 {
		return m.db.CreateSession(projectID, start, end, note)
	}
	if err := m.db.UpdateSession(id, start, end, note); err != nil {
		return nil, err
	}
	return m.db.GetSession(id)
}

func (m *Manager) DeleteProject(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.db.DeleteProject(id); err != nil {
		return err
	}
	config := m.state.TrackingConfig
	config.Assignments = []models.WorkspaceAssignment{}
	for _, a := range m.state.Assignments {
		project, err := m.db.GetProject(a.ProjectID)
		if err != nil {
			return err
		}
		if project != nil {
			config.Assignments = append(config.Assignments, a)
		}
	}
	target := m.target(config)
	if config.Mode == "global" {
		active, err := m.db.GetActiveSession()
		if err != nil {
			return err
		}
		if active != nil {
			target = active.ProjectID
		}
	}
	if err := m.db.ApplyTracking(&config, target, m.now(), "", false); err != nil {
		return err
	}
	m.state.TrackingConfig = config
	return nil
}
