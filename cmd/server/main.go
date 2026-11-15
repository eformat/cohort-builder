// cohort-builder server: typed Kueue YAML builder, validator and org-chart
// workspace. Local-first: one binary serves the API + the embedded UI.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/eformat/cohort-builder/internal/server"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	dbPath := flag.String("db", defaultDBPath(), "sqlite database path")
	kubeconfig := flag.String("kubeconfig", "", "explicit kubeconfig path for read-only cluster import (default: KUBECONFIG env or ~/.kube/config)")
	flag.Parse()

	// .env from the working directory (gitignored) — secrets stay out of argv
	loadDotEnv(".env")

	// LLM config: env only (secrets never in argv); COHORT_LLM_MODEL wins,
	// then OPENAI_MODEL_NAME (the MaaS convention), then the glm default
	baseURL := os.Getenv("OPENAI_BASE_URL")
	apiKey := os.Getenv("OPENAI_API_KEY")
	llmModel := os.Getenv("COHORT_LLM_MODEL")
	if llmModel == "" {
		llmModel = os.Getenv("OPENAI_MODEL_NAME")
	}

	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o755); err != nil {
		log.Fatalf("create db dir: %v", err)
	}

	srv, err := server.New(*dbPath, *kubeconfig, baseURL, apiKey, llmModel)
	if err != nil {
		log.Fatalf("server: %v", err)
	}
	defer srv.Close()

	llmState := "not configured"
	if baseURL != "" && apiKey != "" {
		llmState = "configured (" + llmModel + ")"
	}
	fmt.Printf("cohort-builder listening on http://%s (db: %s, llm: %s)\n", *addr, *dbPath, llmState)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

// loadDotEnv applies KEY=VALUE pairs from path, overriding the inherited env:
// the repo's .env is the explicit local config (the ambient shell env has been
// observed drifting — e.g. stale KUBECONFIG pointing at a sandbox cluster).
func loadDotEnv(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if k != "" && v != "" {
			_ = os.Setenv(k, v)
		}
	}
}

func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "cohort.db"
	}
	return filepath.Join(home, ".local", "share", "cohort-builder", "cohort.db")
}
