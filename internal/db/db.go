package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"timetrack/internal/models"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

var migrations = []struct {
	version int
	sql     string
}{
	{1, `
        CREATE TABLE IF NOT EXISTS projects (
            id         INTEGER PRIMARY KEY AUTOINCREMENT,
            name       TEXT    NOT NULL,
            parent_id  INTEGER REFERENCES projects(id) ON DELETE CASCADE,
            created_at INTEGER NOT NULL DEFAULT (unixepoch())
        );
        CREATE TABLE IF NOT EXISTS sessions (
            id         INTEGER PRIMARY KEY AUTOINCREMENT,
            project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
            start      INTEGER NOT NULL,
            end        INTEGER,
            note       TEXT NOT NULL DEFAULT ''
        );
        CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_id);
        CREATE INDEX IF NOT EXISTS idx_sessions_start   ON sessions(start);
    `},
	{2, `
        ALTER TABLE projects ADD COLUMN archived INTEGER NOT NULL DEFAULT 0;
    `},
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	d := &DB{conn: conn}
	return d, d.migrate()
}

func (d *DB) Close() error { return d.conn.Close() }

func (d *DB) migrate() error {
	// Create the version-tracking table itself
	_, err := d.conn.Exec(`
        CREATE TABLE IF NOT EXISTS schema_migrations (
            version    INTEGER PRIMARY KEY,
            applied_at INTEGER NOT NULL DEFAULT (unixepoch())
        )
    `)
	if err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	for _, m := range migrations {
		var exists int
		err := d.conn.QueryRow(
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("checking migration %d: %w", m.version, err)
		}
		if exists > 0 {
			continue
		}

		if _, err := d.conn.Exec(m.sql); err != nil {
			return fmt.Errorf("applying migration %d: %w", m.version, err)
		}
		if _, err := d.conn.Exec(
			`INSERT INTO schema_migrations (version) VALUES (?)`, m.version,
		); err != nil {
			return fmt.Errorf("recording migration %d: %w", m.version, err)
		}
	}
	return nil
}

// ── Projects ──────────────────────────────────────────────────────────────────

func (d *DB) GetAllProjects() ([]*models.Project, error) {
	rows, err := d.conn.Query(`SELECT id, name, parent_id FROM projects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []*models.Project
	for rows.Next() {
		p := &models.Project{}
		if err := rows.Scan(&p.ID, &p.Name, &p.ParentID); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, nil
}

func (d *DB) GetProject(id int64) (*models.Project, error) {
	p := &models.Project{}
	err := d.conn.QueryRow(`SELECT id, name, parent_id FROM projects WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.ParentID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (d *DB) CreateProject(name string, parentID *int64) (*models.Project, error) {
	res, err := d.conn.Exec(`INSERT INTO projects (name, parent_id) VALUES (?, ?)`, name, parentID)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.Project{ID: id, Name: name, ParentID: parentID}, nil
}

func (d *DB) UpdateProject(id int64, name string, parentID *int64) error {
	_, err := d.conn.Exec(`UPDATE projects SET name = ?, parent_id = ? WHERE id = ?`, name, parentID, id)
	return err
}

func (d *DB) DeleteProject(id int64) error {
	_, err := d.conn.Exec(`DELETE FROM projects WHERE id = ?`, id)
	return err
}

// BuildTree takes a flat list and returns root projects with Children populated.
func BuildTree(flat []*models.Project, totals map[int64]int64) []*models.Project {
	byID := make(map[int64]*models.Project, len(flat))
	for _, p := range flat {
		p.TotalSeconds = totals[p.ID]
		byID[p.ID] = p
	}

	var roots []*models.Project
	for _, p := range flat {
		if p.ParentID == nil {
			roots = append(roots, p)
		} else if parent, ok := byID[*p.ParentID]; ok {
			parent.Children = append(parent.Children, p)
		}
	}

	// Roll up totals from children into parents (bottom-up)
	var rollup func(p *models.Project)
	rollup = func(p *models.Project) {
		for _, c := range p.Children {
			rollup(c)
			p.TotalSeconds += c.TotalSeconds
		}
	}
	for _, r := range roots {
		rollup(r)
	}
	return roots
}

// GetProjectTotals returns total seconds per project_id (completed sessions only)
func (d *DB) GetProjectTotals() (map[int64]int64, error) {
	rows, err := d.conn.Query(`
		SELECT project_id, SUM(end - start)
		FROM sessions
		WHERE end IS NOT NULL
		GROUP BY project_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	totals := make(map[int64]int64)
	for rows.Next() {
		var pid, total int64
		rows.Scan(&pid, &total)
		totals[pid] = total
	}
	return totals, nil
}

// ── Sessions ──────────────────────────────────────────────────────────────────

func (d *DB) GetSessions(projectID *int64, from, to *time.Time) ([]*models.Session, error) {
	query := `SELECT id, project_id, start, end, note FROM sessions WHERE 1=1`
	args := []any{}

	if projectID != nil {
		query += ` AND project_id = ?`
		args = append(args, *projectID)
	}
	if from != nil {
		query += ` AND start >= ?`
		args = append(args, from.Unix())
	}
	if to != nil {
		query += ` AND start <= ?`
		args = append(args, to.Unix())
	}
	query += ` ORDER BY start DESC`

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

func (d *DB) GetSession(id int64) (*models.Session, error) {
	row := d.conn.QueryRow(`SELECT id, project_id, start, end, note FROM sessions WHERE id = ?`, id)
	sessions, err := scanSessions(&rowsWrapper{row})
	if err != nil || len(sessions) == 0 {
		return nil, err
	}
	return sessions[0], nil
}

func (d *DB) CreateSession(projectID int64, start time.Time, end *time.Time, note string) (*models.Session, error) {
	var endUnix *int64
	if end != nil {
		v := end.Unix()
		endUnix = &v
	}
	res, err := d.conn.Exec(
		`INSERT INTO sessions (project_id, start, end, note) VALUES (?, ?, ?, ?)`,
		projectID, start.Unix(), endUnix, note,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &models.Session{ID: id, ProjectID: projectID, Start: start, End: end, Note: note}, nil
}

func (d *DB) UpdateSession(id int64, start time.Time, end *time.Time, note string) error {
	var endUnix *int64
	if end != nil {
		v := end.Unix()
		endUnix = &v
	}
	_, err := d.conn.Exec(
		`UPDATE sessions SET start = ?, end = ?, note = ? WHERE id = ?`,
		start.Unix(), endUnix, note, id,
	)
	return err
}

func (d *DB) DeleteSession(id int64) error {
	_, err := d.conn.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// ── Tracking ──────────────────────────────────────────────────────────────────

func (d *DB) GetActiveSession() (*models.Session, error) {
	row := d.conn.QueryRow(`SELECT id, project_id, start, end, note FROM sessions WHERE end IS NULL LIMIT 1`)
	sessions, err := scanSessions(&rowsWrapper{row})
	if err != nil || len(sessions) == 0 {
		return nil, err
	}
	return sessions[0], nil
}

func (d *DB) StartTracking(projectID int64) (*models.Session, error) {
	// Stop any active session first
	if err := d.stopActiveSession(""); err != nil {
		return nil, err
	}
	return d.CreateSession(projectID, time.Now(), nil, "")
}

func (d *DB) StopTracking(note string) (*models.Session, error) {
	active, err := d.GetActiveSession()
	if err != nil || active == nil {
		return active, err
	}
	if err := d.stopActiveSession(note); err != nil {
		return nil, err
	}
	active.End = func() *time.Time { t := time.Now(); return &t }()
	active.Note = note
	return active, nil
}

func (d *DB) stopActiveSession(note string) error {
	_, err := d.conn.Exec(
		`UPDATE sessions SET end = ?, note = ? WHERE end IS NULL`,
		time.Now().Unix(), note,
	)
	return err
}

// GetProjectTotalWithActive returns total seconds for a project including any active session
func (d *DB) GetProjectTotalWithActive(projectID int64) (int64, error) {
	var total int64
	err := d.conn.QueryRow(`
		SELECT COALESCE(SUM(CASE WHEN end IS NOT NULL THEN end - start ELSE unixepoch() - start END), 0)
		FROM sessions WHERE project_id = ?`, projectID).Scan(&total)
	return total, err
}

// GetProjectPath returns the full path string e.g. "Chess > Library"
func (d *DB) GetProjectPath(id int64) (string, error) {
	var parts []string
	cur := id
	for {
		var name string
		var parentID *int64
		err := d.conn.QueryRow(`SELECT name, parent_id FROM projects WHERE id = ?`, cur).Scan(&name, &parentID)
		if err != nil {
			break
		}
		parts = append([]string{name}, parts...)
		if parentID == nil {
			break
		}
		cur = *parentID
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("project not found")
	}
	return strings.Join(parts, " > "), nil
}

// GetStats returns total seconds per project for a time range, including descendant projects
func (d *DB) GetStats(from, to *time.Time) (map[int64]int64, error) {
	query := `SELECT project_id, SUM(CASE WHEN end IS NOT NULL THEN end - start ELSE unixepoch() - start END) FROM sessions WHERE 1=1`
	args := []any{}
	if from != nil {
		query += ` AND start >= ?`
		args = append(args, from.Unix())
	}
	if to != nil {
		query += ` AND start <= ?`
		args = append(args, to.Unix())
	}
	query += ` GROUP BY project_id`

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64]int64)
	for rows.Next() {
		var pid, secs int64
		rows.Scan(&pid, &secs)
		result[pid] = secs
	}
	return result, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

type rowsWrapper struct{ *sql.Row }

func (r *rowsWrapper) Next() bool          { return true }
func (r *rowsWrapper) Scan(d ...any) error { return r.Row.Scan(d...) }

func scanSessions(rows interface {
	Scan(...any) error
}) ([]*models.Session, error) {
	switch r := rows.(type) {
	case *sql.Rows:
		var out []*models.Session
		for r.Next() {
			s, err := scanOneSession(r)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, nil
	case *rowsWrapper:
		s, err := scanOneSession(r.Row)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return []*models.Session{s}, nil
	}
	return nil, nil
}

func scanOneSession(s scanner) (*models.Session, error) {
	sess := &models.Session{}
	var startUnix int64
	var endUnix *int64
	if err := s.Scan(&sess.ID, &sess.ProjectID, &startUnix, &endUnix, &sess.Note); err != nil {
		return nil, err
	}
	sess.Start = time.Unix(startUnix, 0)
	if endUnix != nil {
		t := time.Unix(*endUnix, 0)
		sess.End = &t
		sess.Duration = *endUnix - startUnix
	}
	return sess, nil
}
