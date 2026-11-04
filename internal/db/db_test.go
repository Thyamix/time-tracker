package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestUpdateProjectArchiveState(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	project, err := database.CreateProject("Original", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, original := range []int8{0, 1} {
		for _, update := range []*int8{nil, new(int8(0)), new(int8(1))} {
			if _, err := database.conn.Exec(`UPDATE projects SET archived = ? WHERE id = ?`, original, project.ID); err != nil {
				t.Fatal(err)
			}
			if err := database.UpdateProject(project.ID, "Renamed", nil, update); err != nil {
				t.Fatal(err)
			}
			var name string
			var archived int8
			if err := database.conn.QueryRow(`SELECT name, archived FROM projects WHERE id = ?`, project.ID).Scan(&name, &archived); err != nil {
				t.Fatal(err)
			}
			want := original
			if update != nil {
				want = *update
			}
			if name != "Renamed" || archived != want {
				t.Errorf("after rename: name = %q, archived = %d; want Renamed, %d", name, archived, want)
			}
		}
	}
}

func TestGetProjectDayTotal(t *testing.T) {
	location := time.FixedZone("UTC+2", 2*3600)
	now := time.Date(2026, time.November, 4, 12, 0, 0, 0, location)
	midnight := time.Date(2026, time.November, 4, 0, 0, 0, 0, location)
	tests := []struct {
		name  string
		start time.Time
		end   time.Time // Zero means an active session.
		want  int64
	}{
		{"completed today", now.Add(-2 * time.Hour), now.Add(-time.Hour), 3600},
		{"previous day", midnight.Add(-2 * time.Hour), midnight.Add(-time.Hour), 0},
		{"crosses midnight", midnight.Add(-time.Hour), midnight.Add(time.Hour), 3600},
		{"ends at midnight", midnight.Add(-time.Hour), midnight, 0},
		{"active today", now.Add(-time.Hour), time.Time{}, 3600},
		{"active across midnight", midnight.Add(-time.Hour), time.Time{}, 12 * 3600},
		{"future session", now.Add(time.Hour), now.Add(2 * time.Hour), 0},
		{"active in future", now.Add(time.Hour), time.Time{}, 0},
		{"ends in future", now.Add(-time.Hour), now.Add(time.Hour), 3600},
		{"starts now", now, time.Time{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			database, err := Open(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { database.Close() })
			project, err := database.CreateProject("Work", nil)
			if err != nil {
				t.Fatal(err)
			}
			other, err := database.CreateProject("Other", nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := database.CreateSession(other.ID, midnight, &now, ""); err != nil {
				t.Fatal(err)
			}
			if total, err := database.GetProjectDayTotal(project.ID, now); err != nil || total != 0 {
				t.Fatalf("empty project total = %d, %v; want 0", total, err)
			}
			var end *time.Time
			if !tt.end.IsZero() {
				end = &tt.end
			}
			if _, err := database.CreateSession(project.ID, tt.start, end, ""); err != nil {
				t.Fatal(err)
			}
			total, err := database.GetProjectDayTotal(project.ID, now)
			if err != nil {
				t.Fatal(err)
			}
			if total != tt.want {
				t.Errorf("total = %d; want %d", total, tt.want)
			}
		})
	}
}
