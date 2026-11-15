// Package store persists projects and pages in SQLite (WAL mode) — the
// server-side source of truth. Page state_json holds the org-chart IR plus
// editor prefs, the same shape the UI keeps in memory.
package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Project is a named collection of org-chart pages.
type Project struct {
	ID      int64     `json:"id"`
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

// Page is one canvas (org chart) inside a project.
type Page struct {
	ID       int64     `json:"id"`
	Project  int64     `json:"projectId"`
	Name     string    `json:"name"`
	Position int       `json:"position"`
	Version  string    `json:"version"` // kueue release selector ("1.4"/"1.5")
	State    string    `json:"state"`   // arbitrary JSON (org chart IR + UI prefs)
	Updated  time.Time `json:"updated"`
}

// Store wraps the SQLite handle.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS projects (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	name     TEXT NOT NULL,
	created  TEXT NOT NULL,
	updated  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS pages (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	name       TEXT NOT NULL,
	position   INTEGER NOT NULL DEFAULT 0,
	version    TEXT NOT NULL DEFAULT '1.5',
	state      TEXT NOT NULL DEFAULT '{}',
	updated    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pages_project ON pages(project_id);
`

// Open opens (creating if needed) the database and applies migrations.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // modernc sqlite: single writer; serialize all access
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// CreateProject inserts a project and returns it.
func (s *Store) CreateProject(name string) (*Project, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`INSERT INTO projects (name, created, updated) VALUES (?, ?, ?)`, name, now, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Project{ID: id, Name: name, Created: parseTime(now), Updated: parseTime(now)}, nil
}

// ListProjects returns all projects, newest-updated first.
func (s *Store) ListProjects() ([]Project, error) {
	rows, err := s.db.Query(`SELECT id, name, created, updated FROM projects ORDER BY updated DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		var c, u string
		if err := rows.Scan(&p.ID, &p.Name, &c, &u); err != nil {
			return nil, err
		}
		p.Created, p.Updated = parseTime(c), parseTime(u)
		out = append(out, p)
	}
	return out, rows.Err()
}

// RenameProject updates a project's name.
func (s *Store) RenameProject(id int64, name string) error {
	_, err := s.db.Exec(`UPDATE projects SET name = ?, updated = ? WHERE id = ?`, name, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// DeleteProject removes a project and its pages (FK cascade).
func (s *Store) DeleteProject(id int64) error {
	_, err := s.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	return err
}

// ListPages returns a project's pages in position order.
func (s *Store) ListPages(project int64) ([]Page, error) {
	rows, err := s.db.Query(
		`SELECT id, project_id, name, position, version, state, updated FROM pages WHERE project_id = ? ORDER BY position, id`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Page
	for rows.Next() {
		var p Page
		var u string
		if err := rows.Scan(&p.ID, &p.Project, &p.Name, &p.Position, &p.Version, &p.State, &u); err != nil {
			return nil, err
		}
		p.Updated = parseTime(u)
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPage returns one page.
func (s *Store) GetPage(id int64) (*Page, error) {
	var p Page
	var u string
	err := s.db.QueryRow(`SELECT id, project_id, name, position, version, state, updated FROM pages WHERE id = ?`, id).
		Scan(&p.ID, &p.Project, &p.Name, &p.Position, &p.Version, &p.State, &u)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("page %d not found", id)
	}
	if err != nil {
		return nil, err
	}
	p.Updated = parseTime(u)
	return &p, nil
}

// SavePage upserts a page. Zero ID inserts; non-zero updates.
func (s *Store) SavePage(p *Page) (*Page, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if p.ID == 0 {
		res, err := s.db.Exec(
			`INSERT INTO pages (project_id, name, position, version, state, updated) VALUES (?, ?, ?, ?, ?, ?)`,
			p.Project, p.Name, p.Position, p.Version, p.State, now)
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		p.ID, p.Updated = id, parseTime(now)
		_, err = s.db.Exec(`UPDATE projects SET updated = ? WHERE id = ?`, now, p.Project)
		return p, err
	}
	_, err := s.db.Exec(
		`UPDATE pages SET name = ?, position = ?, version = ?, state = ?, updated = ? WHERE id = ?`,
		p.Name, p.Position, p.Version, p.State, now, p.ID)
	if err != nil {
		return nil, err
	}
	p.Updated = parseTime(now)
	_, err = s.db.Exec(`UPDATE projects SET updated = ? WHERE id = ?`, now, p.Project)
	return p, err
}

// DeletePage removes one page.
func (s *Store) DeletePage(id int64) error {
	_, err := s.db.Exec(`DELETE FROM pages WHERE id = ?`, id)
	return err
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// CreatePageForTest is a test helper creating a page in one call.
func (s *Store) CreatePageForTest(project int64, name, state string) (*Page, error) {
	return s.SavePage(&Page{Project: project, Name: name, Version: "1.5", State: state})
}
