// Structural validation against the exact CRDs the kueue-operator ships for the
// selected release — the same schemas the API server enforces on apply.
package validate

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	"github.com/eformat/cohort-builder/internal/crds"
		"github.com/eformat/cohort-builder/internal/model"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/runtime"
	sigsyaml "sigs.k8s.io/yaml"
)

// structural validates objects of one Kueue release.
type structural struct {
	validators map[string]validation.SchemaCreateValidator // kind → validator
}

// loadStructural parses the embedded CRDs for a release and builds per-kind
// schema validators.
func loadStructural(version string) (*structural, error) {
	dir := crds.DirFor(version)
	if dir == "" {
		return nil, fmt.Errorf("unsupported version %q", version)
	}
	entries, err := fs.ReadDir(crds.FS, dir)
	if err != nil {
		return nil, fmt.Errorf("read embedded CRDs: %w", err)
	}
	s := &structural{validators: map[string]validation.SchemaCreateValidator{}}
	conv := runtime.NewScheme()
	if err := apiextensionsv1.AddToScheme(conv); err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		raw, err := fs.ReadFile(crds.FS, dir+"/"+e.Name())
		if err != nil {
			return nil, err
		}
		// Decode the external v1 CRD and convert ONLY the JSONSchemaProps:
		// the whole-CRD scheme conversion silently drops versions[].schema
		// for CRDs with webhook conversion (ResourceFlavor/Topologies).
		var crdV1 apiextensionsv1.CustomResourceDefinition
		if err := sigsyaml.Unmarshal(raw, &crdV1); err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		var schemaV1 *apiextensionsv1.JSONSchemaProps
		for _, v := range crdV1.Spec.Versions {
			if v.Name == "v1beta2" && v.Schema != nil && v.Schema.OpenAPIV3Schema != nil {
				schemaV1 = v.Schema.OpenAPIV3Schema
				break
			}
		}
		if schemaV1 == nil {
			continue
		}
		schema := &apiextensions.JSONSchemaProps{}
		if err := conv.Convert(schemaV1, schema, nil); err != nil {
			return nil, fmt.Errorf("convert schema %s: %w", e.Name(), err)
		}
		// defaulting-adjacent metadata lives at the object root: extend the
		// CRD schema root to accept apiVersion/kind/metadata like the server does
		root := schema
		if root.Properties == nil {
			root.Properties = map[string]apiextensions.JSONSchemaProps{}
		}
		if _, ok := root.Properties["apiVersion"]; !ok {
			root.Properties["apiVersion"] = apiextensions.JSONSchemaProps{Type: "string"}
		}
		if _, ok := root.Properties["kind"]; !ok {
			root.Properties["kind"] = apiextensions.JSONSchemaProps{Type: "string"}
		}
		if _, ok := root.Properties["metadata"]; !ok {
			root.Properties["metadata"] = apiextensions.JSONSchemaProps{Type: "object"}
		}
		v, _, err := validation.NewSchemaValidator(root)
		if err != nil {
			return nil, fmt.Errorf("validator for %s: %w", crdV1.Spec.Names.Kind, err)
		}
		s.validators[crdV1.Spec.Names.Kind] = v
	}
	return s, nil
}

// validateObject checks one object (JSON bytes including apiVersion/kind) against
// its kind's CRD schema.
func (s *structural) validateObject(kind string, objJSON []byte) []model.Problem {
	v, ok := s.validators[kind]
	if !ok {
		return []model.Problem{{
			Severity: model.SevWarning,
			Path:     "$",
			Message:  fmt.Sprintf("no CRD schema for kind %q in this release — skipped structural validation", kind),
		}}
	}
	obj := map[string]any{}
	if err := json.Unmarshal(objJSON, &obj); err != nil {
		return []model.Problem{{Severity: model.SevError, Path: "$", Message: "object is not valid JSON: " + err.Error()}}
	}
	errs := validation.ValidateCustomResource(nil, obj, v)
	out := make([]model.Problem, 0, len(errs))
	for _, e := range errs {
		out = append(out, model.Problem{
			Severity: model.SevError,
			Path:     "$." + strings.TrimPrefix(e.Field, ""),
			Message:  e.ErrorBody(),
		})
	}
	return out
}
