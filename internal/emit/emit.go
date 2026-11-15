// Package emit converts org-chart IR into version-accurate Kueue YAML using the
// synced typed API objects — no templates anywhere. Typed object → JSON (struct
// field order preserved) → JSONToYAML (key order preserved → kubectl-style output).
//
// Mapping rules (from the KubeCon slides 32-48 + the live GPU Booking cluster):
//   - node with children            → Cohort (parentName = nearest ancestor cohort)
//   - node with namespaces          → ClusterQueue (+ one LocalQueue per namespace)
//   - node with both                → Cohort + its own CQ in the same name layer
//   - pool-mode CQ                  → resourceGroups mirror the cohort's resources
//                                     with nominalQuota 0 (borrowing declaration)
//   - guaranteed-mode CQ            → resourceGroups from the node's own quota
package emit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/eformat/cohort-builder/internal/model"
	"k8s.io/apimachinery/pkg/api/resource"
	sigsyaml "sigs.k8s.io/yaml"
)

// Object is one typed Kubernetes object ready for serialization.
type Object struct {
	APIVersion string
	Kind       string
	Name       string
	Namespace  string
	Value      any // the versioned typed object
}

// Summary describes an emitted object for the UI.
type Summary struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// Result is the emitter output.
type Result struct {
	YAML    string
	Objects []Summary
}

// cohortPlan is the version-neutral description of a Cohort.
type cohortPlan struct {
	Name       string
	ParentName string
	Quota      map[string]*model.ResourceQuota
	Flavor     string
	Weight     string
}

// cqPlan is the version-neutral description of a ClusterQueue (+ LocalQueues).
type cqPlan struct {
	Name             string
	CohortName       string
	Namespaces       []string
	LocalQueueName   string
	PresetKey        string
	QueueingStrategy string
	Weight           string
	Mode             model.QuotaMode
	Quota            map[string]*model.ResourceQuota // guaranteed-mode quota
	MirrorResources  []string                        // pool-mode: cohort resources emitted with nominalQuota 0
	Flavor           string
}

// rgSpec / resSpec are neutral resource-group descriptions consumed by the
// versioned constructors.
type resSpec struct {
	Name           string
	NominalQuota   string
	BorrowingLimit *string
	LendingLimit   *string
}

type rgSpec struct {
	CoveredResources []string
	Flavor           string
	Resources        []resSpec
}

// buildResources turns an IR quota map into one resourceGroup (single flavor).
// zero=true emits the pool-mode borrowing declaration (all nominalQuota 0 — the
// live-verified pattern for CQs that borrow from the cohort pool).
func buildResources(quota map[string]*model.ResourceQuota, flavor string, zero bool) ([]rgSpec, error) {
	if len(quota) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(quota))
	for k := range quota {
		names = append(names, k)
	}
	sort.Strings(names)

	resources := make([]resSpec, 0, len(names))
	for _, name := range names {
		q := quota[name]
		if q == nil {
			q = &model.ResourceQuota{}
		}
		nominal := q.NominalQuota
		if zero || nominal == "" {
			nominal = "0"
		}
		if _, err := resource.ParseQuantity(nominal); err != nil {
			return nil, fmt.Errorf("quota %s: %v", name, err)
		}
		rs := resSpec{Name: name, NominalQuota: nominal}
		if !zero {
			for _, p := range []struct {
				v string
				d **string
			}{{q.BorrowingLimit, &rs.BorrowingLimit}, {q.LendingLimit, &rs.LendingLimit}} {
				if p.v == "" {
					continue
				}
				if _, err := resource.ParseQuantity(p.v); err != nil {
					return nil, fmt.Errorf("quota %s limit %q: %v", name, p.v, err)
				}
				*p.d = &p.v
			}
		}
		resources = append(resources, rs)
	}
	return []rgSpec{{CoveredResources: names, Flavor: flavor, Resources: resources}}, nil
}

// inheritFlavor resolves the subtree flavor: nearest self-or-ancestor non-empty
// Flavor, else "default-flavor".
func inheritFlavor(n *model.Node, parents []*model.Node) string {
	if n.Flavor != "" {
		return n.Flavor
	}
	for i := len(parents) - 1; i >= 0; i-- {
		if parents[i].Flavor != "" {
			return parents[i].Flavor
		}
	}
	return "default-flavor"
}

// buildPlans walks the chart and produces ordered plans: cohorts root→leaf, then
// ClusterQueues in tree order. All org→Kueue mapping rules live here.
func buildPlans(chart *model.OrgChart) ([]cohortPlan, []cqPlan) {
	var cohorts []cohortPlan
	var cqs []cqPlan

	_ = chart.Root.Walk(func(n *model.Node, parents []*model.Node) error {
		flavor := inheritFlavor(n, parents)
		if n.HasChildren() && !n.Synthetic {
			cp := cohortPlan{
				Name:   n.Name,
				Quota:  n.Quota,
				Flavor: flavor,
				Weight: n.FairSharingWeight,
			}
			for i := len(parents) - 1; i >= 0; i-- {
				if !parents[i].Synthetic && parents[i].HasChildren() {
					cp.ParentName = parents[i].Name
					break
				}
			}
			cohorts = append(cohorts, cp)
		}
		// A node becomes a ClusterQueue when it declares namespaces OR when it is
		// a leaf (teams are leaves; a planned team may not have namespaces yet —
		// it still emits as a queue that admits nothing until namespaces are set).
		isLeaf := !n.HasChildren()
		if n.HasClusterQueue() || isLeaf {
			cq := cqPlan{
				Name:             n.Name,
				Namespaces:       n.Namespaces,
				LocalQueueName:   n.LocalQueueDefault(),
				PresetKey:        n.Preset,
				QueueingStrategy: n.QueueingStrategy,
				Weight:           n.FairSharingWeight,
				Mode:             n.Mode,
				Quota:            n.Quota,
				Flavor:           flavor,
			}
			anc := model.CohortAncestorNonVirtual(n, parents)
			if anc != nil {
				cq.CohortName = anc.Name
				// Borrowing declaration: mirror the covered resources of the
				// WHOLE ancestor-cohort chain (nearest → root). Pass-through
				// cohorts without own quota sit between the team and the pool
				// (the normal GPUaaS shape: pool at the root, teams borrow up
				// the tree), so mirroring only the nearest cohort would leave
				// the CQ unable to borrow anything.
				seen := map[string]bool{}
				if anc.Quota != nil {
					for k := range anc.Quota {
						seen[k] = true
					}
				}
				if n.HasChildren() && n.Quota != nil {
					for k := range n.Quota { // self-layer node: its cohort is itself
						seen[k] = true
					}
				}
				for i := len(parents) - 1; i >= 0; i-- {
					if parents[i].Synthetic || !parents[i].HasChildren() || parents[i].Quota == nil {
						continue
					}
					for k := range parents[i].Quota {
						seen[k] = true
					}
				}
				for k := range seen {
					cq.MirrorResources = append(cq.MirrorResources, k)
				}
				sort.Strings(cq.MirrorResources)
			}
			cqs = append(cqs, cq)
		}
		return nil
	})
	return cohorts, cqs
}

// Emit renders the chart as multi-document YAML using the version's typed objects.
func Emit(chart *model.OrgChart) (*Result, error) {
	if chart == nil || chart.Root == nil {
		return nil, fmt.Errorf("empty org chart")
	}

	cohorts, cqs := buildPlans(chart)

	var objs []Object
	for i := range cohorts {
		o, err := newCohortObject(chart.Version, &cohorts[i])
		if err != nil {
			return nil, fmt.Errorf("cohort %s: %w", cohorts[i].Name, err)
		}
		objs = append(objs, *o)
	}
	for i := range cqs {
		o, err := newClusterQueueObject(chart.Version, &cqs[i])
		if err != nil {
			return nil, fmt.Errorf("clusterqueue %s: %w", cqs[i].Name, err)
		}
		objs = append(objs, *o)
		// LocalQueues: one per explicit team namespace. Wide namespaceSelectors
		// (RHOAI's default CQ allows hundreds) are admission scope only — they
		// never fan out into LocalQueues.
		for _, ns := range cqs[i].Namespaces {
			if ns == "*" {
				continue // all-namespaces pseudo-selector: no per-namespace LocalQueue
			}
			if len(cqs[i].Namespaces) > 8 {
				continue // wide selector, not an explicit team list
			}
			objs = append(objs, newLocalQueueObject(chart.Version, cqs[i].Name, ns, cqs[i].LocalQueueName))
		}
	}
	for i := range chart.ResourceFlavors {
		objs = append(objs, newResourceFlavorObject(chart.Version, &chart.ResourceFlavors[i]))
	}
	for i := range chart.PriorityClasses {
		o, err := newPriorityClassObject(chart.Version, &chart.PriorityClasses[i])
		if err != nil {
			return nil, fmt.Errorf("priorityclass %s: %w", chart.PriorityClasses[i].Name, err)
		}
		objs = append(objs, *o)
	}

	yaml, err := marshalDocs(objs)
	if err != nil {
		return nil, err
	}
	summaries := make([]Summary, 0, len(objs))
	for _, o := range objs {
		summaries = append(summaries, Summary{Kind: o.Kind, Name: o.Name, Namespace: o.Namespace})
	}
	return &Result{YAML: yaml, Objects: summaries}, nil
}

// marshalDocs serializes typed objects to kubectl-ordered multi-doc YAML:
// typed struct → JSON (field order preserved) → JSONToYAML (key order preserved).
// The top-level `status` object is stripped: Status is declared last in the synced
// structs and its zero value is not skipped by omitempty (struct with int fields /
// empty-object cases), but generated manifests must not carry status — the API
// server owns it. Truncating from the last `,"status":` to the final `}` is exact
// for compact JSON because status is the last top-level field.
func marshalDocs(objs []Object) (string, error) {
	var b strings.Builder
	for _, o := range objs {
		j, err := json.Marshal(o.Value)
		if err != nil {
			return "", fmt.Errorf("marshal %s %s: %w", o.Kind, o.APIVersion, err)
		}
		if idx := bytes.LastIndex(j, []byte(`,"status":`)); idx >= 0 {
			j = append(j[:idx], '}')
		}
		y, err := sigsyaml.JSONToYAML(j)
		if err != nil {
			return "", fmt.Errorf("to yaml %s %s: %w", o.Kind, o.APIVersion, err)
		}
		b.WriteString("---\n")
		b.Write(y)
	}
	return b.String(), nil
}
