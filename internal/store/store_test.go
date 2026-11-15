package store

import (
	"path/filepath"
	"testing"
)

func TestProjectPageCRUD(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	p, err := s.CreateProject("demo")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("project id not assigned")
	}

	pages, err := s.ListProjects()
	if err != nil || len(pages) != 1 {
		t.Fatalf("list projects: %v (%d)", err, len(pages))
	}

	page, err := s.CreatePageForTest(p.ID, "main", `{"version":"1.5"}`)
	if err != nil {
		t.Fatalf("create page: %v", err)
	}

	got, err := s.GetPage(page.ID)
	if err != nil || got.State != `{"version":"1.5"}` {
		t.Fatalf("get page: %v (%q)", err, got.State)
	}

	page.State = `{"version":"1.4"}`
	if _, err := s.SavePage(page); err != nil {
		t.Fatalf("save page: %v", err)
	}
	got, _ = s.GetPage(page.ID)
	if got.State != `{"version":"1.4"}` || got.Version != "1.5" {
		t.Fatalf("update lost: %+v", got)
	}

	if err := s.DeletePage(page.ID); err != nil {
		t.Fatalf("delete page: %v", err)
	}
	if _, err := s.GetPage(page.ID); err == nil {
		t.Fatal("expected not-found after delete")
	}

	if err := s.DeleteProject(p.ID); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	remaining, _ := s.ListProjects()
	if len(remaining) != 0 {
		t.Fatalf("project not deleted: %d", len(remaining))
	}
}
