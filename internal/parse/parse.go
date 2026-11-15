// Package parse reconstructs an org chart (IR) from Kueue YAML — Direction A.
// It accepts multi-doc YAML carrying Cohorts, ClusterQueues, LocalQueues,
// ResourceFlavors and WorkloadPriorityClasses for the selected release.
//
// Reverse-mapping rules (inverse of internal/emit):
//   - Cohort.parentName edges build the tree; quota → node.quota
//   - ClusterQueue.cohortName attaches the CQ under its cohort node
//   - namespaceSelector → namespaces ("{}" → ["*"], matchLabels → [ns],
//     matchExpressions In → values)
//   - all-zero nominalQuota on a cohorted CQ → pool mode, else guaranteed
//   - preemption/fungibility/queueing matched against presets; unmatched → custom
package parse

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/eformat/cohort-builder/internal/model"
	"github.com/eformat/cohort-builder/internal/presets"
	sigsyaml "sigs.k8s.io/yaml"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// Result carries the reconstructed chart plus diagnostics.
type Result struct {
	Chart   *model.OrgChart
	Objects []string // "Kind/name" in input order
}

// cohortDoc / cqDoc are the intermediate per-object views.
type cohortDoc struct {
	name       string
	parentName string
	quota      map[string]*model.ResourceQuota
	flavor     string
	weight     string
	hasQuota   bool
}

type cqDoc struct {
	name        string
	cohortName  string
	namespaces  []string
	quota       map[string]*model.ResourceQuota
	flavor      string
	weight      string
	strategy    string
	allZero     bool
	hasQuota    bool
	preemption  map[string]any // raw spec.preemption
	fungibility map[string]any // raw spec.flavorFungibility
}

// ChartFromYAML parses multi-doc Kueue YAML into an IR chart.
func ChartFromYAML(version string, multi string) (*Result, []model.Problem, error) {
	if !model.IsSupportedVersion(version) {
		return nil, nil, fmt.Errorf("unsupported version %q", version)
	}
	docs := splitDocs(multi)
	if len(docs) == 0 {
		return nil, nil, fmt.Errorf("no YAML documents found")
	}

	cohorts := map[string]*cohortDoc{}
	cqs := map[string]*cqDoc{}
	flavors := []model.ResourceFlavor{}
	pclasses := []model.PriorityClass{}
	var order []string
	problems := []model.Problem{}

	for i, doc := range docs {
		j, err := sigsyaml.YAMLToJSON([]byte(doc))
		if err != nil {
			problems = append(problems, model.Problem{Severity: model.SevError,
				Path: fmt.Sprintf("doc[%d]", i), Message: "invalid YAML: " + err.Error()})
			continue
		}
		var obj struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			Metadata   struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Spec map[string]any `json:"spec"`
		}
		if err := json.Unmarshal(j, &obj); err != nil {
			problems = append(problems, model.Problem{Severity: model.SevError,
				Path: fmt.Sprintf("doc[%d]", i), Message: "not a Kubernetes object: " + err.Error()})
			continue
		}
		switch obj.Kind {
		case "Cohort":
			c := &cohortDoc{name: obj.Metadata.Name, quota: map[string]*model.ResourceQuota{}}
			if v, ok := obj.Spec["parentName"].(string); ok {
				c.parentName = v
			}
			c.quota, c.hasQuota = parseResourceGroups(obj.Spec["resourceGroups"])
			c.flavor = firstFlavor(obj.Spec["resourceGroups"])
			c.weight = weightOf(obj.Spec["fairSharing"])
			cohorts[c.name] = c
			order = append(order, "Cohort/"+c.name)
		case "ClusterQueue":
			q := &cqDoc{name: obj.Metadata.Name, quota: map[string]*model.ResourceQuota{}, allZero: true}
			if v, ok := obj.Spec["cohortName"].(string); ok {
				q.cohortName = v
			} else if v, ok := obj.Spec["cohort"].(string); ok {
				q.cohortName = v // legacy v1beta1 field
				problems = append(problems, model.Problem{Severity: model.SevInfo,
					Path:    fmt.Sprintf("doc[%d]", i),
					Message: "used legacy field spec.cohort (v1beta1); current API is spec.cohortName"})
			}
			q.quota, q.hasQuota = parseResourceGroups(obj.Spec["resourceGroups"])
			q.flavor = firstFlavor(obj.Spec["resourceGroups"])
			for _, fq := range flavorQuotasOf(obj.Spec["resourceGroups"]) {
				if fq.nominal != "0" {
					q.allZero = false
				}
			}
			if ns, ok := obj.Spec["namespaceSelector"].(map[string]any); ok {
				q.namespaces = parseNamespaceSelector(ns)
			}
			q.weight = weightOf(obj.Spec["fairSharing"])
			if s, ok := obj.Spec["queueingStrategy"].(string); ok {
				q.strategy = s
			}
			q.preemption, _ = obj.Spec["preemption"].(map[string]any)
			q.fungibility, _ = obj.Spec["flavorFungibility"].(map[string]any)
			cqs[q.name] = q
			order = append(order, "ClusterQueue/"+q.name)
		case "LocalQueue":
			cqName, _ := obj.Spec["clusterQueue"].(string)
			if target, ok := cqs[cqName]; ok {
				// namespace where the LocalQueue lives implies the CQ serves it —
				// unless the CQ already serves all namespaces ("*")
				if obj.Metadata.Namespace != "" && !hasStar(target.namespaces) {
					target.namespaces = appendUnique(target.namespaces, obj.Metadata.Namespace)
				}
			} else {
				problems = append(problems, model.Problem{Severity: model.SevWarning,
					Path:    fmt.Sprintf("doc[%d]", i),
					Message: fmt.Sprintf("LocalQueue %s/%s points at unknown ClusterQueue %q", obj.Metadata.Namespace, obj.Metadata.Name, cqName)})
			}
			order = append(order, "LocalQueue/"+obj.Metadata.Name)
		case "ResourceFlavor":
			rf := model.ResourceFlavor{Name: obj.Metadata.Name}
			if nl, ok := obj.Spec["nodeLabels"].(map[string]any); ok {
				rf.NodeLabels = map[string]string{}
				for k, v := range nl {
					rf.NodeLabels[k], _ = v.(string)
				}
			}
			if ts, ok := obj.Spec["nodeTaints"].([]any); ok {
				for _, tt := range ts {
					if m, ok := tt.(map[string]any); ok {
						t := model.Taint{}
						t.Key, _ = m["key"].(string)
						t.Value, _ = m["value"].(string)
						t.Effect, _ = m["effect"].(string)
						rf.Taints = append(rf.Taints, t)
					}
				}
			}
			flavors = append(flavors, rf)
			order = append(order, "ResourceFlavor/"+rf.Name)
		case "WorkloadPriorityClass":
			pc := model.PriorityClass{Name: obj.Metadata.Name}
			if v, ok := obj.Spec["value"].(float64); ok {
				pc.Value = int32(v)
			}
			if v, ok := obj.Spec["description"].(string); ok {
				pc.Description = v
			}
			pclasses = append(pclasses, pc)
			order = append(order, "WorkloadPriorityClass/"+pc.Name)
		default:
			problems = append(problems, model.Problem{Severity: model.SevWarning,
				Path:    fmt.Sprintf("doc[%d]", i),
				Message: fmt.Sprintf("skipped %q object %q — not part of the org-chart model", obj.Kind, obj.Metadata.Name)})
		}
	}

	chart, buildProblems := buildChartImpl(version, cohorts, cqs, flavors, pclasses)
	problems = append(problems, buildProblems...)
	return &Result{Chart: chart, Objects: order}, problems, nil
}

// --- small yaml-shape helpers -----------------------------------------------

type fqEntry struct {
	flavor  string
	name    string
	nominal string
}

func flavorQuotasOf(specAny any) []fqEntry {
	out := []fqEntry{}
	rgs, ok := specAny.([]any)
	if !ok {
		return out
	}
	for _, rg := range rgs {
		rgm, ok := rg.(map[string]any)
		if !ok {
			continue
		}
		fls, ok := rgm["flavors"].([]any)
		if !ok {
			continue
		}
		for _, f := range fls {
			fm, ok := f.(map[string]any)
			if !ok {
				continue
			}
			fname, _ := fm["name"].(string)
			res, ok := fm["resources"].([]any)
			if !ok {
				continue
			}
			for _, r := range res {
				rm, ok := r.(map[string]any)
				if !ok {
					continue
				}
				rname, _ := rm["name"].(string)
				nom, _ := rm["nominalQuota"].(string)
				if nom == "" {
					if n, ok := rm["nominalQuota"].(float64); ok {
						nom = fmt.Sprintf("%v", n)
					}
				}
				out = append(out, fqEntry{flavor: fname, name: rname, nominal: nom})
			}
		}
	}
	return out
}

// firstFlavor returns the first flavor name referenced by the resourceGroups.
func firstFlavor(specAny any) string {
	fqs := flavorQuotasOf(specAny)
	if len(fqs) > 0 {
		return fqs[0].flavor
	}
	return ""
}

func parseResourceGroups(specAny any) (map[string]*model.ResourceQuota, bool) {
	into := map[string]*model.ResourceQuota{}
	has := false
	for _, fq := range flavorQuotasOf(specAny) {
		q := into[fq.name]
		if q == nil {
			q = &model.ResourceQuota{}
			into[fq.name] = q
		}
		q.NominalQuota = fq.nominal
		has = true
	}
	return into, has
}

func parseNamespaceSelector(sel map[string]any) []string {
	if len(sel) == 0 {
		return []string{"*"}
	}
	if ml, ok := sel["matchLabels"].(map[string]any); ok && len(ml) == 1 {
		for _, v := range ml {
			if s, ok := v.(string); ok {
				return []string{s}
			}
		}
	}
	if me, ok := sel["matchExpressions"].([]any); ok {
		for _, e := range me {
			em, ok := e.(map[string]any)
			if !ok {
				continue
			}
			op, _ := em["operator"].(string)
			if op != "In" {
				continue
			}
			vals, ok := em["values"].([]any)
			if !ok {
				continue
			}
			out := []string{}
			for _, v := range vals {
				if s, ok := v.(string); ok {
					out = append(out, s)
				}
			}
			return out
		}
	}
	return nil
}

func weightOf(fsAny any) string {
	fs, ok := fsAny.(map[string]any)
	if !ok {
		return ""
	}
	if w, ok := fs["weight"].(string); ok {
		return w
	}
	if w, ok := fs["weight"].(float64); ok {
		return fmt.Sprintf("%v", w)
	}
	return ""
}

func hasStar(s []string) bool {
	for _, e := range s {
		if e == "*" {
			return true
		}
	}
	return false
}

func appendUnique(s []string, v string) []string {
	for _, e := range s {
		if e == v {
			return s
		}
	}
	return append(s, v)
}

func splitDocs(multi string) []string {
	var docs []string
	r := yaml.NewYAMLReader(bufio.NewReader(bytes.NewReader([]byte(multi))))
	for {
		doc, err := r.Read()
		if len(bytes.TrimSpace(doc)) > 0 {
			docs = append(docs, string(doc))
		}
		if err != nil {
			break
		}
	}
	return docs
}

// recognizePreset matches a CQ's policy fields against known presets.
func recognizePreset(preemption, fungibility map[string]any, strategy string) string {
	got := func(m map[string]any, key string) string {
		if m == nil {
			return ""
		}
		s, _ := m[key].(string)
		return s
	}
	gotNum := func(m map[string]any, key string) string {
		if m == nil {
			return ""
		}
		if f, ok := m[key].(float64); ok {
			return fmt.Sprintf("%d", int64(f))
		}
		return ""
	}
	bw := func(m map[string]any) (string, string) {
		if m == nil {
			return "", ""
		}
		b, ok := m["borrowWithinCohort"].(map[string]any)
		if !ok {
			return "", ""
		}
		return got(b, "policy"), gotNum(b, "maxPriorityThreshold")
	}

	gR := got(preemption, "reclaimWithinCohort")
	gW := got(preemption, "withinClusterQueue")
	gBP, gBT := bw(preemption)
	gWB := got(fungibility, "whenCanBorrow")
	gWP := got(fungibility, "whenCanPreempt")

	for _, p := range presets.All {
		pv, fv := p.Preemption, p.FlavorFungibility
		rw, wq, bp, bt := "", "", "", ""
		wb, wp := "", ""
		if pv != nil {
			rw, wq, bp = pv.ReclaimWithinCohort, pv.WithinClusterQueue, pv.BorrowPolicy
			if pv.BorrowMaxPriorityThrh != 0 {
				bt = fmt.Sprintf("%d", pv.BorrowMaxPriorityThrh)
			}
		}
		if fv != nil {
			wb, wp = fv.WhenCanBorrow, fv.WhenCanPreempt
		}
		if rw == gR && wq == gW && bp == gBP && bt == gBT && wb == gWB && wp == gWP && p.QueueingStrategy == strategy {
			return p.Key
		}
	}
	return "custom"
}

