// Package server wires the HTTP API: typed emit/parse/validate per version,
// LLM image import, read-only cluster import, and SQLite-backed projects.
package server

import (
	"embed"
	"encoding/json"
	"io"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/eformat/cohort-builder/internal/cluster"
	"github.com/eformat/cohort-builder/internal/kueueapi"
	"github.com/eformat/cohort-builder/internal/crds"
	"github.com/eformat/cohort-builder/internal/emit"
	"github.com/eformat/cohort-builder/internal/gpuprofiles"
	"github.com/eformat/cohort-builder/internal/llm"
	"github.com/eformat/cohort-builder/internal/model"
	"github.com/eformat/cohort-builder/internal/parse"
	"github.com/eformat/cohort-builder/internal/presets"
	"github.com/eformat/cohort-builder/internal/store"
	"github.com/eformat/cohort-builder/internal/validate"
)

//go:embed static
var staticFS embed.FS

// Server holds the wired dependencies.
type Server struct {
	store      *store.Store
	llm        *llm.Client
	reader     *cluster.Reader
	mux        *http.ServeMux
	validators map[string]*validate.Validator
}

// New wires a server. dbPath is the SQLite file; kubeconfig an optional explicit
// path; llmBaseURL/llmAPIKey/llmModel configure image import.
func New(dbPath, kubeconfig, llmBaseURL, llmAPIKey, llmModel string) (*Server, error) {
	st, err := store.Open(dbPath)
	if err != nil {
		return nil, err
	}
	s := &Server{
		store:      st,
		llm:        llm.New(llmBaseURL, llmAPIKey, llmModel),
		reader:     cluster.NewReader(kubeconfig),
		mux:        http.NewServeMux(),
		validators: map[string]*validate.Validator{},
	}
	s.routes()
	return s, nil
}

// Close releases resources.
func (s *Server) Close() error { return s.store.Close() }

// Handler returns the root handler.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	m := s.mux

	// metadata
	m.HandleFunc("GET /api/versions", s.handleVersions)
	m.HandleFunc("GET /api/presets", s.handlePresets)
	m.HandleFunc("GET /api/gpu-profiles", s.handleGPUProfiles)
	m.HandleFunc("GET /api/capabilities", s.handleCapabilities)

	// engine
	m.HandleFunc("POST /api/validate", s.handleValidate)
	m.HandleFunc("POST /api/parse", s.handleParse)
	m.HandleFunc("POST /api/emit", s.handleEmit)

	// imports
	m.HandleFunc("POST /api/import/image", s.handleImportImage)
	m.HandleFunc("GET /api/cluster/contexts", s.handleClusterContexts)
	m.HandleFunc("GET /api/cluster/snapshot", s.handleClusterSnapshot)
	m.HandleFunc("POST /api/cluster/import", s.handleClusterImport)

	// projects & pages
	m.HandleFunc("GET /api/projects", s.handleProjectsList)
	m.HandleFunc("POST /api/projects", s.handleProjectsCreate)
	m.HandleFunc("PUT /api/projects/{id}", s.handleProjectRename)
	m.HandleFunc("DELETE /api/projects/{id}", s.handleProjectDelete)
	m.HandleFunc("GET /api/projects/{id}/pages", s.handlePagesList)
	m.HandleFunc("POST /api/projects/{id}/pages", s.handlePageCreate)
	m.HandleFunc("GET /api/pages/{id}", s.handlePageGet)
	m.HandleFunc("PUT /api/pages/{id}", s.handlePageSave)
	m.HandleFunc("DELETE /api/pages/{id}", s.handlePageDelete)

	// static UI (embedded dist; dev mode proxies to vite instead)
	sub, _ := fs.Sub(staticFS, "static")
	m.Handle("GET /", http.FileServerFS(sub))
}

func (s *Server) validator(version string) (*validate.Validator, error) {
	if v, ok := s.validators[version]; ok {
		return v, nil
	}
	v, err := validate.New(version)
	if err != nil {
		return nil, err
	}
	s.validators[version] = v
	return v, nil
}

// --- helpers -----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode[T any](w http.ResponseWriter, r *http.Request, dst *T) bool {
	body, err := io_Limit(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read body: "+err.Error())
		return false
	}
	if err := json.Unmarshal(body, dst); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func io_Limit(r *http.Request) ([]byte, error) {
	// 64 MiB cap (images base64-encoded can be large)
	return readAll(r.Body, 64<<20)
}

func readAll(r io.Reader, n int64) ([]byte, error) {
	buf := make([]byte, 0, 4096)
	var total int64
	tmp := make([]byte, 32*1024)
	for {
		c, err := r.Read(tmp)
		if c > 0 {
			total += int64(c)
			if total > n {
				return nil, fmt.Errorf("body too large (>%d bytes)", n)
			}
			buf = append(buf, tmp[:c]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf, nil
			}
			return buf, err
		}
	}
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

// --- metadata ----------------------------------------------------------------

func (s *Server) handleVersions(w http.ResponseWriter, r *http.Request) {
	type v struct {
		Key     string `json:"key"`
		Label   string `json:"label"`
		Operand string `json:"operand"`
	}
	out := []v{
		{Key: model.Version14, Label: "1.4 (upstream/release-1.4)", Operand: "Kueue v0.18.1"},
		{Key: model.Version15, Label: "1.5 (upstream/release-1.5)", Operand: "Kueue v0.19.0"},
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handlePresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, presets.All)
}

func (s *Server) handleGPUProfiles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, gpuprofiles.All)
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	version := r.URL.Query().Get("version")
	dir := crds.DirFor(version)
	if dir == "" {
		writeErr(w, http.StatusBadRequest, "unknown version")
		return
	}
	// serve the whole matrix; the UI gates by version
	_, _ = w.Write(kueueapi.CapabilitiesJSON)
}

// --- engine ------------------------------------------------------------------

type validateReq struct {
	Version string `json:"version"`
	YAML    string `json:"yaml"`
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	var req validateReq
	if !decode(w, r, &req) {
		return
	}
	v, err := s.validator(req.Version)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	docs := splitYAML(req.YAML)
	writeJSON(w, http.StatusOK, map[string]any{"problems": nilSafe(v.ValidateYAML(docs))})
}

type parseReq struct {
	Version string `json:"version"`
	YAML    string `json:"yaml"`
}

func (s *Server) handleParse(w http.ResponseWriter, r *http.Request) {
	var req parseReq
	if !decode(w, r, &req) {
		return
	}
	res, problems, err := parse.ChartFromYAML(req.Version, req.YAML)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"chart": nil, "problems": problemsWith(err.Error(), model.SevError)})
		return
	}
	// add structural validation of the input against the release CRDs
	if v, err := s.validator(req.Version); err == nil {
		problems = append(problems, v.ValidateYAML(splitYAML(req.YAML))...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"chart": res.Chart, "objects": res.Objects, "problems": nilSafe(problems)})
}

type emitReq struct {
	Version string          `json:"version"`
	Chart   *model.OrgChart `json:"chart"`
}

func (s *Server) handleEmit(w http.ResponseWriter, r *http.Request) {
	var req emitReq
	if !decode(w, r, &req) {
		return
	}
	if req.Chart == nil {
		writeErr(w, http.StatusBadRequest, "chart is required")
		return
	}
	req.Chart.Version = req.Version
	res, err := emit.Emit(req.Chart)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var problems []model.Problem
	if v, err := s.validator(req.Version); err == nil {
		problems = v.ValidateChart(req.Chart)
	}
	writeJSON(w, http.StatusOK, map[string]any{"yaml": res.YAML, "objects": res.Objects, "problems": nilSafe(problems)})
}

// --- imports -----------------------------------------------------------------

type importImageReq struct {
	Version string `json:"version"`
	Image   string `json:"image"` // data URL (data:image/png;base64,...)
}

func (s *Server) handleImportImage(w http.ResponseWriter, r *http.Request) {
	var req importImageReq
	if !decode(w, r, &req) {
		return
	}
	if !s.llm.Configured() {
		writeErr(w, http.StatusServiceUnavailable, "LLM not configured (OPENAI_BASE_URL / OPENAI_API_KEY)")
		return
	}
	chart, problems, err := s.llm.ExtractOrgChart(r.Context(), req.Version, req.Image)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chart": chart, "problems": nilSafe(problems)})
}

func (s *Server) handleClusterContexts(w http.ResponseWriter, r *http.Request) {
	names, current, err := s.reader.Contexts()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"contexts": names, "current": current})
}

func (s *Server) handleClusterSnapshot(w http.ResponseWriter, r *http.Request) {
	snap, err := s.reader.Snapshot(r.Context(), r.URL.Query().Get("context"))
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

type clusterImportReq struct {
	Version string `json:"version"`
	Context string `json:"context"`
}

func (s *Server) handleClusterImport(w http.ResponseWriter, r *http.Request) {
	var req clusterImportReq
	if !decode(w, r, &req) {
		return
	}
	res, problems, err := s.reader.Import(r.Context(), req.Version, req.Context)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chart": res.Chart, "objects": res.Objects, "problems": nilSafe(problems)})
}

// --- projects ----------------------------------------------------------------

func (s *Server) handleProjectsList(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.ListProjects()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if projects == nil {
		projects = []store.Project{}
	}
	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) handleProjectsCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	p, err := s.store.CreateProject(strings.TrimSpace(req.Name))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleProjectRename(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := s.store.RenameProject(id, req.Name); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleProjectDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.store.DeleteProject(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handlePagesList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	pages, err := s.store.ListPages(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pages == nil {
		pages = []store.Page{}
	}
	writeJSON(w, http.StatusOK, pages)
}

func (s *Server) handlePageCreate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req store.Page
	if !decode(w, r, &req) {
		return
	}
	req.Project = id
	p, err := s.store.SavePage(&req)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handlePageGet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	p, err := s.store.GetPage(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handlePageSave(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req store.Page
	if !decode(w, r, &req) {
		return
	}
	req.ID = id
	// merge with the stored page: empty fields are preserved, never wiped
	// (autosave PUTs state only — name/version must survive).
	existing, err := s.store.GetPage(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if req.Project == 0 {
		req.Project = existing.Project
	}
	if req.State == "" {
		req.State = existing.State
	}
	if req.Name == "" {
		req.Name = existing.Name
	}
	if req.Version == "" {
		req.Version = existing.Version
	}
	p, err := s.store.SavePage(&req)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handlePageDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.store.DeletePage(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func problemsWith(msg string, sev model.Severity) []model.Problem {
	return []model.Problem{{Severity: sev, Path: "$", Message: msg}}
}

// nilSafe ensures slices marshal as [] not null (the UI maps over them).
func nilSafe(p []model.Problem) []model.Problem {
	if p == nil {
		return []model.Problem{}
	}
	return p
}
