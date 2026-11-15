// Package llm talks to the MaaS OpenAI-compatible endpoint (glm-5.3-flash) to
// extract an org chart from a diagram image. The LLM only ever produces the IR
// — emission stays in the typed emitter.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/eformat/cohort-builder/internal/model"
)

// Client calls the OpenAI-compatible chat completions API.
type Client struct {
	BaseURL string // e.g. https://api.maas.../v1 (no trailing slash)
	APIKey  string
	Model   string
	HTTP    *http.Client
}

// New builds a client from environment-style settings.
func New(baseURL, apiKey, model string) *Client {
	if model == "" {
		model = "publishers/prelude-maas/models/glm-53-flash"
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}
}

// Configured reports whether the client has the minimum settings to call out.
func (c *Client) Configured() bool { return c.BaseURL != "" && c.APIKey != "" }

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	Temperature    float64         `json:"temperature"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"` // "json_object"
}

type chatMessage struct {
	Role    string        `json:"role"`
	Content any           `json:"content"` // string or []contentPart
}

type contentPart struct {
	Type     string    `json:"type"` // "text" | "image_url"
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"` // data: URL or https URL
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// irRules is the modeling contract embedded in the system prompt.
const irRules = `You extract a GPU platform org chart from a diagram image into JSON.

Model the diagram with these rules:
- The output is a single JSON object: {"version": "%s", "root": NODE, "resourceFlavors": [], "priorityClasses": []}
- NODE = {"name": "<dns-lowercase>", "kind": "org"|"division"|"team", "quota": {...}, "fairSharingWeight": "<qty>", "namespaces": [...], "mode": "pool"|"guaranteed", "preset": "...", "flavor": "...", "children": [NODE...]}
- Every ORG LEVEL (company, division, group) becomes a node with "children". Every TEAM at the bottom becomes a node with "namespaces" (its target namespaces; use ["*"] only if the diagram shows an all-namespaces shared queue).
- "quota" maps resource names to {"nominalQuota": "<qty>", "borrowingLimit": "<qty>", "lendingLimit": "<qty>"}. K8s quantity strings: "500", "1873Gi". Resources look like "nvidia.com/gpu", "cpu", "memory", "nvidia.com/mig-1g.18gb".
- Root/district quota = shared pool (additive to member queues). Team quota = that team's reservation (guaranteed mode) — set "mode": "guaranteed" when the diagram shows per-team guaranteed amounts, "pool" when teams borrow from a shared pool.
- "fairSharingWeight" is the fair-sharing weight when the diagram shows weights (e.g. "3").
- "preset" for teams: "booked-team" (reserved team that reclaims from borrowers), "unreserved" (pure borrower, no preemption rights), "unreserved-priority" (preempts lower priorities only, threshold 100), "fair-share" (weighted fair sharing). Omit for custom.
- Names must be lowercase DNS-1123 (letters, digits, '-', '.'). Derive from the diagram labels; replace spaces with '-'.
- If a number is unreadable, omit the field rather than guessing.
- NEVER invent teams that are not in the image. Return ONLY the JSON object.`

// ExtractOrgChart sends the image and returns a validated chart. The response is
// schema-validated by construction (parse + validate); LLM errors surface as problems.
func (c *Client) ExtractOrgChart(ctx context.Context, version, imageDataURL string) (*model.OrgChart, []model.Problem, error) {
	if !c.Configured() {
		return nil, nil, fmt.Errorf("LLM not configured (set OPENAI_BASE_URL and OPENAI_API_KEY)")
	}

	req := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: fmt.Sprintf(irRules, version)},
			{Role: "user", Content: []contentPart{
				{Type: "text", Text: "Extract the org chart from this diagram image as the JSON described."},
				{Type: "image_url", ImageURL: &imageURL{URL: imageDataURL}},
			}},
		},
		MaxTokens:      4096,
		Temperature:    0.1,
		ResponseFormat: &responseFormat{Type: "json_object"},
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("LLM request: %w", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// some OpenAI-compatible gateways reject response_format — retry without it
		if req.ResponseFormat != nil && strings.Contains(string(raw), "response_format") {
			req.ResponseFormat = nil
			raw2, err := c.do(ctx, req)
			if err != nil {
				return nil, nil, err
			}
			return c.decode(raw2)
		}
		return nil, nil, fmt.Errorf("LLM returned %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	return c.decode(raw)
}

func (c *Client) do(ctx context.Context, req chatRequest) ([]byte, error) {
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("LLM request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM returned %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	return raw, nil
}

func (c *Client) decode(raw []byte) (*model.OrgChart, []model.Problem, error) {
	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return nil, nil, fmt.Errorf("LLM response parse: %w", err)
	}
	if cr.Error != nil {
		return nil, nil, fmt.Errorf("LLM error: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return nil, nil, fmt.Errorf("LLM returned no choices")
	}

	content := strings.TrimSpace(cr.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var chart model.OrgChart
	if err := json.Unmarshal([]byte(content), &chart); err != nil {
		return nil, []model.Problem{{
			Severity: model.SevError,
			Path:     "$",
			Message:  "LLM output is not a valid org chart: " + err.Error(),
		}}, nil
	}
	problems := chart.Validate()
	return &chart, problems, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
