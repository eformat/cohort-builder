// Package validate wires the two validation layers: structural (emitted objects
// checked against the synced operator CRDs) and semantic (Kueue invariants +
// operational lessons). It is the backend for the UI's validation pane.
package validate

import (
	"encoding/json"
	"fmt"

	"github.com/eformat/cohort-builder/internal/emit"
	"github.com/eformat/cohort-builder/internal/model"
)

// Validator validates charts for one Kueue release.
type Validator struct {
	version string
	st      *structural
}

// New builds a validator for a release selector ("1.4"/"1.5").
func New(version string) (*Validator, error) {
	st, err := loadStructural(version)
	if err != nil {
		return nil, err
	}
	return &Validator{version: version, st: st}, nil
}

// ValidateChart runs model, semantic and structural checks. Structural checks
// run against the emitted typed objects (what would actually be applied).
func (v *Validator) ValidateChart(chart *model.OrgChart) []model.Problem {
	var problems []model.Problem

	// 1. version-agnostic structural checks on the IR
	problems = append(problems, chart.Validate()...)

	// 2. semantic invariants
	problems = append(problems, semanticChecks(chart)...)

	// 3. structural: emit typed objects and validate each against the release CRDs
	if chart.Root == nil || !model.IsSupportedVersion(chart.Version) {
		return problems
	}
	res, err := emit.Emit(chart)
	if err != nil {
		problems = append(problems, model.Problem{
			Severity: model.SevError,
			Path:     "$",
			Message:  "emission failed: " + err.Error(),
		})
		return problems
	}
	for _, doc := range splitDocs(res.YAML) {
		kind, objJSON, err := docKindJSON(doc)
		if err != nil {
			problems = append(problems, model.Problem{Severity: model.SevError, Path: "$", Message: err.Error()})
			continue
		}
		problems = append(problems, v.st.validateObject(kind, objJSON)...)
	}
	return problems
}

// ValidateYAML parses arbitrary multi-doc Kueue YAML and validates each object
// against the release CRDs (Direction A entry point; typed round-trip lives in
// internal/parse).
func (v *Validator) ValidateYAML(docs []string) []model.Problem {
	var problems []model.Problem
	for i, doc := range docs {
		kind, objJSON, err := docKindJSON(doc)
		if err != nil {
			problems = append(problems, model.Problem{Severity: model.SevError, Path: fmt.Sprintf("doc[%d]", i), Message: err.Error()})
			continue
		}
		for _, p := range v.st.validateObject(kind, objJSON) {
			p.Path = fmt.Sprintf("doc[%d]%s", i, p.Path)
			problems = append(problems, p)
		}
	}
	return problems
}

func docKindJSON(doc string) (kind string, objJSON []byte, err error) {
	j, err := sigsyamlToJSON(doc)
	if err != nil {
		return "", nil, fmt.Errorf("invalid YAML: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(j, &m); err != nil {
		return "", nil, fmt.Errorf("document is not an object: %v", err)
	}
	k, _ := m["kind"].(string)
	if k == "" {
		return "", nil, fmt.Errorf("document has no kind")
	}
	return k, j, nil
}
