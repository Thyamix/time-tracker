package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"timetrack/internal/db"
	"timetrack/internal/models"
	"timetrack/internal/tracking"
)

func TestRenameProject(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	parent, err := database.CreateProject("Parent", nil)
	if err != nil {
		t.Fatal(err)
	}
	router := NewServer(database, nil).Router()
	for _, parentID := range []*int64{nil, &parent.ID} {
		project, err := database.CreateProject("Original", parentID)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(map[string]any{"name": " Renamed ", "parent_id": parentID})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("PATCH", fmt.Sprintf("/api/projects/%d", project.ID), strings.NewReader(string(payload))))
		if response.Code != 204 {
			t.Fatalf("rename status = %d: %s", response.Code, response.Body.String())
		}
		renamed, err := database.GetProject(project.ID)
		if err != nil {
			t.Fatal(err)
		}
		if renamed.Name != "Renamed" {
			t.Errorf("name = %q; want Renamed", renamed.Name)
		}
		if (parentID == nil) != (renamed.ParentID == nil) || parentID != nil && *parentID != *renamed.ParentID {
			t.Errorf("parent changed during rename: %+v", renamed)
		}
		for _, name := range []string{"", "   "} {
			response := httptest.NewRecorder()
			payload := fmt.Sprintf(`{"name":%q}`, name)
			router.ServeHTTP(response, httptest.NewRequest("PATCH", fmt.Sprintf("/api/projects/%d", project.ID), strings.NewReader(payload)))
			if response.Code != 400 {
				t.Errorf("blank rename status = %d; want 400", response.Code)
			}
		}
		renamed, err = database.GetProject(project.ID)
		if err != nil {
			t.Fatal(err)
		}
		if renamed.Name != "Renamed" {
			t.Errorf("invalid rename changed name to %q", renamed.Name)
		}
	}
}

func TestWaybarStatus(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	tracker, err := tracking.New(database)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(database, tracker)
	project, err := database.CreateProject("Work", nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	middle := midnight.Add(now.Sub(midnight) / 2)
	if _, err := database.CreateSession(project.ID, midnight.Add(-time.Hour), &middle, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateSession(project.ID, middle, nil, ""); err != nil {
		t.Fatal(err)
	}

	getStatus := func() models.Status {
		t.Helper()
		response := httptest.NewRecorder()
		server.getStatus(response, httptest.NewRequest("GET", "/api/status", nil))
		if response.Code != 200 {
			t.Fatalf("status = %d: %s", response.Code, response.Body.String())
		}
		var status models.Status
		if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		return status
	}
	formatDuration := func(seconds int64) string {
		if seconds >= 3600 {
			return fmt.Sprintf("%dh %dm", seconds/3600, seconds%3600/60)
		}
		return fmt.Sprintf("%dm", seconds/60)
	}
	before := time.Now()
	status := getStatus()
	after := time.Now()
	for at := before.Unix(); ; at++ {
		day := at - midnight.Unix()
		want := fmt.Sprintf("⏱ Work  %s today (%s)", formatDuration(day), formatDuration(day+3600))
		if status.WaybarText == want {
			break
		}
		if at >= after.Unix() {
			t.Fatalf("Waybar text = %q; want %q", status.WaybarText, want)
		}
	}
	if !status.Active || status.WaybarClass != "active" || status.Total < 3600 || status.Elapsed > after.Unix()-middle.Unix() {
		t.Fatalf("unexpected active status: %+v", status)
	}

	if err := tracker.Configure(models.TrackingConfig{Mode: "sway", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	status = getStatus()
	if status.Active || status.WaybarText != "Tracking paused" || status.WaybarClass != "paused" {
		t.Fatalf("unexpected paused status: %+v", status)
	}
	if status.WaybarTooltip != "Workspace tracking paused\nClick for workspace controls" {
		t.Errorf("paused tooltip = %q", status.WaybarTooltip)
	}
}
