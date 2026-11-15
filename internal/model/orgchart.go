// Package model defines the version-agnostic org-chart intermediate representation
// (IR). Every Kueue version snapshot (internal/kueueapi/*) emits from and parses to
// this single shape, so the UI, the emitters, and the importers share one contract.
package model

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// Supported Kueue-operator release versions (the version selector values).
const (
	Version14 = "1.4"
	Version15 = "1.5"
)

// Versions lists the selector values in display order.
var Versions = []string{Version14, Version15}

// IsSupportedVersion reports whether v is a known kueue-operator release selector.
func IsSupportedVersion(v string) bool {
	for _, s := range Versions {
		if v == s {
			return true
		}
	}
	return false
}

// KueuePackageDir maps a release selector to its synced API package directory.
func KueuePackageDir(v string) string {
	switch v {
	case Version14:
		return "v0_18_1"
	case Version15:
		return "v0_19_0"
	default:
		return ""
	}
}

// QuotaMode selects how a team node's ClusterQueue relates to the cohort pool.
type QuotaMode string

const (
	// ModePool: the CQ declares the cohort's covered resources with nominalQuota 0
	// and borrows from the cohort pool (verified against the live GPU Booking
	// cluster: borrowing CQs must still declare the flavors they borrow).
	ModePool QuotaMode = "pool"
	// ModeGuaranteed: the CQ carries its own nominalQuota (additive to any cohort
	// pool — Kueue cohort quota is additive to member CQ quota, issue #5289).
	ModeGuaranteed QuotaMode = "guaranteed"
)

// NodeKind is display-only metadata for the canvas.
type NodeKind string

const (
	KindOrg      NodeKind = "org"
	KindDivision NodeKind = "division"
	KindTeam     NodeKind = "team"
)

// ResourceQuota is the per-resource quota contribution of a node. Quantities are
// K8s quantity strings ("159", "1873Gi"); empty means unset.
type ResourceQuota struct {
	NominalQuota   string `json:"nominalQuota,omitempty"`
	BorrowingLimit string `json:"borrowingLimit,omitempty"`
	LendingLimit   string `json:"lendingLimit,omitempty"`
}

// Node is one card on the org canvas. A node with children becomes a Cohort; a
// node with namespaces becomes a ClusterQueue (+ one LocalQueue per namespace);
// a node with both gets the Cohort and its own ClusterQueue in the same name
// layer (the KubeCon slide-36 "dedicated quota" pattern).
type Node struct {
	Name        string `json:"name"`
	Kind        NodeKind `json:"kind,omitempty"`
	Description string `json:"description,omitempty"`

	// Quota is what THIS layer contributes. On a node with children it lands on
	// the Cohort's resourceGroups; on a team leaf in guaranteed mode it lands on
	// the ClusterQueue. Keyed by resource name ("cpu", "nvidia.com/gpu", ...).
	Quota map[string]*ResourceQuota `json:"quota,omitempty"`

	// FairSharingWeight is the fair-sharing weight (K8s quantity string, e.g. "3").
	FairSharingWeight string `json:"fairSharingWeight,omitempty"`

	// Namespaces makes this node a ClusterQueue (one LocalQueue per namespace).
	// The special value "*" means all namespaces (empty namespaceSelector —
	// the GPU Booking "unreserved" pattern).
	Namespaces []string `json:"namespaces,omitempty"`

	// Mode selects pool vs guaranteed quota placement (team nodes).
	Mode QuotaMode `json:"mode,omitempty"`

	// Preset keys the preemption policy preset ("" = custom / API defaults).
	Preset string `json:"preset,omitempty"`

	// QueueingStrategy overrides the preset's strategy ("StrictFIFO"|"BestEffortFIFO").
	QueueingStrategy string `json:"queueingStrategy,omitempty"`

	// Flavor is the resource flavor name used by this subtree; children inherit
	// it when unset (default "default-flavor").
	Flavor string `json:"flavor,omitempty"`

	// LocalQueueName is the LocalQueue name created in each namespace (default "default").
	LocalQueueName string `json:"localQueueName,omitempty"`

	// Synthetic marks a UI-only grouping node (e.g. a forest root created when an
	// import yields several top-level objects). Synthetic nodes are never emitted
	// as Cohorts and are transparent for parentName/cohortName wiring.
	Synthetic bool `json:"synthetic,omitempty"`

	Children []*Node `json:"children,omitempty"`
}

// ResourceFlavor mirrors the subset of Kueue ResourceFlavor the builder emits.
type ResourceFlavor struct {
	Name       string            `json:"name"`
	NodeLabels map[string]string `json:"nodeLabels,omitempty"`
	Taints     []Taint           `json:"taints,omitempty"`
}

// Taint is a node taint applied via a ResourceFlavor.
type Taint struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Effect string `json:"effect,omitempty"` // NoSchedule|PreferNoSchedule|NoExecute
}

// PriorityClass mirrors WorkloadPriorityClass.
type PriorityClass struct {
	Name        string `json:"name"`
	Value       int32  `json:"value"`
	Description string `json:"description,omitempty"`
}

// Defaults is the page-level default GPU profile applied to nodes that lack
// settings (diagram import, new teams). Cluster imports never receive it —
// real cluster data wins.
type Defaults struct {
	Resource string `json:"resource,omitempty"` // e.g. nvidia.com/gpu
	Count    int    `json:"count,omitempty"`    // e.g. 8
	Flavor   string `json:"flavor,omitempty"`   // e.g. h200
}

// OrgChart is the root IR document.
type OrgChart struct {
	Version         string           `json:"version"`
	Root            *Node            `json:"root"`
	Defaults        *Defaults        `json:"defaults,omitempty"`
	ResourceFlavors []ResourceFlavor `json:"resourceFlavors,omitempty"`
	PriorityClasses []PriorityClass  `json:"priorityClasses,omitempty"`
}

// HasChildren reports whether the node becomes a Cohort.
func (n *Node) HasChildren() bool { return len(n.Children) > 0 }

// HasClusterQueue reports whether the node becomes a ClusterQueue.
func (n *Node) HasClusterQueue() bool { return len(n.Namespaces) > 0 }

// FlavorName resolves the subtree flavor (default "default-flavor").
func (n *Node) FlavorName() string {
	if n.Flavor != "" {
		return n.Flavor
	}
	return "default-flavor"
}

// LocalQueueDefault resolves the LocalQueue name (default "default").
func (n *Node) LocalQueueDefault() string {
	if n.LocalQueueName != "" {
		return n.LocalQueueName
	}
	return "default"
}

// Path renders a human-readable JSON-path-ish location for diagnostics.
func (n *Node) Path() string {
	return "$.root"
}

// Walk visits the tree preorder. parents excludes n. Cycles (a node appearing
// twice on its own ancestor path) are skipped defensively — the tree invariants
// are enforced upstream, but the walker never recurses forever.
func (n *Node) Walk(fn func(n *Node, parents []*Node) error) error {
	var rec func(cur *Node, parents []*Node, onPath map[*Node]bool) error
	rec = func(cur *Node, parents []*Node, onPath map[*Node]bool) error {
		if err := fn(cur, parents); err != nil {
			return err
		}
		for _, c := range cur.Children {
			if onPath[c] {
				continue // cycle guard: never revisit an ancestor
			}
			next := make(map[*Node]bool, len(onPath)+1)
			for k := range onPath {
				next[k] = true
			}
			next[cur] = true
			if err := rec(c, append(parents, cur), next); err != nil {
				return err
			}
		}
		return nil
	}
	if n == nil {
		return nil
	}
	return rec(n, nil, map[*Node]bool{})
}

// CohortAncestor returns the nearest node on the ancestor chain (including n
// itself) that has children — i.e. the Cohort this node's ClusterQueue joins.
// Returns nil when no cohort exists on the chain (standalone CQ).
func CohortAncestor(n *Node, parents []*Node) *Node {
	if n.HasChildren() {
		return n
	}
	for i := len(parents) - 1; i >= 0; i-- {
		if parents[i].HasChildren() {
			return parents[i]
		}
	}
	return nil
}

// CohortAncestorNonVirtual returns the nearest ancestor (including n) that has
// children and is NOT a synthetic grouping node. Synthetic roots organize the
// canvas but are transparent for Kueue wiring.
func CohortAncestorNonVirtual(n *Node, parents []*Node) *Node {
	if !n.Synthetic && n.HasChildren() {
		return n
	}
	for i := len(parents) - 1; i >= 0; i-- {
		if !parents[i].Synthetic && parents[i].HasChildren() {
			return parents[i]
		}
	}
	return nil
}

// Validate runs version-agnostic structural checks on the chart. Version-specific
// capability checks live in internal/validate.
func (c *OrgChart) Validate() []Problem {
	var problems []Problem
	add := func(sev Severity, path, msg string) {
		problems = append(problems, Problem{Severity: sev, Path: path, Message: msg})
	}

	if !IsSupportedVersion(c.Version) {
		add(SevError, "$.version", fmt.Sprintf("unknown version %q (supported: %s)", c.Version, strings.Join(Versions, ", ")))
	}
	if c.Root == nil {
		add(SevError, "$.root", "org chart has no root node")
		return problems
	}
	if c.Root.Name == "" {
		add(SevError, "$.root.name", "root node needs a name")
	}

	seen := map[string]string{}   // cohort-name -> path of first owner
	seenCQ := map[string]string{} // clusterqueue-name -> path of first owner
	namespaces := map[string]string{}
	wideClaim := map[string]bool{}
	_ = c.Root.Walk(func(n *Node, parents []*Node) error {
		path := nodePath(parents, n.Name)
		if n.Name == "" {
			add(SevError, path+".name", "node needs a name")
			return nil
		}
		if msgs := validation.IsDNS1123Subdomain(strings.ToLower(n.Name)); len(msgs) > 0 && n.Name != strings.ToLower(n.Name) {
			add(SevError, path+".name", fmt.Sprintf("name %q is not a valid DNS-1123 name (lowercase alphanumeric, '-', '.'): %s", n.Name, msgs[0]))
		} else if n.Name != strings.ToLower(n.Name) {
			add(SevError, path+".name", fmt.Sprintf("Kueue names must be lowercase: %q", n.Name))
		}
		// A Cohort and a ClusterQueue may share a name (the live "unreserved"
		// self-layer pattern); duplicates are checked per object kind.
		if n.HasChildren() {
			if prev, dup := seen[n.Name]; dup {
				add(SevError, path+".name", fmt.Sprintf("duplicate Cohort name %q (also at %s) — Cohorts are cluster-scoped", n.Name, prev))
			} else {
				seen[n.Name] = path
			}
		}
		if n.HasClusterQueue() {
			if prev, dup := seenCQ[n.Name]; dup {
				add(SevError, path+".name", fmt.Sprintf("duplicate ClusterQueue name %q (also at %s) — ClusterQueues are cluster-scoped", n.Name, prev))
			} else {
				seenCQ[n.Name] = path
			}
			for _, ns := range n.Namespaces {
				if ns == "*" {
					continue // all-namespaces pseudo-selector
				}
				if msgs := validation.IsDNS1123Label(strings.ToLower(ns)); len(msgs) > 0 {
					add(SevError, path+".namespaces", fmt.Sprintf("namespace %q invalid: %s", ns, msgs[0]))
				}
				if prev, dup := namespaces[ns]; dup {
					// A namespaceSelector only ALLOWS namespaces — the LocalQueue
					// decides which queue actually serves them. Overlap with a
					// wide selector (RHOAI's default CQ claims hundreds) is normal;
					// only flag explicit small lists as a real footgun.
					if len(n.Namespaces) <= 8 && !wideClaim[ns] {
						add(SevWarning, path+".namespaces", fmt.Sprintf("namespace %q is allowed by both %q and %q — only the LocalQueue placement decides which queue serves it", ns, prev, n.Name))
					} else {
						wideClaim[ns] = true
					}
				} else {
					namespaces[ns] = n.Name
				}
			}
		}
		if n.Mode != "" && n.Mode != ModePool && n.Mode != ModeGuaranteed {
			add(SevError, path+".mode", fmt.Sprintf("mode %q must be %q or %q", n.Mode, ModePool, ModeGuaranteed))
		}
		for res, q := range n.Quota {
			if q == nil {
				continue
			}
			for _, field := range []struct {
				name  string
				value string
			}{{"nominalQuota", q.NominalQuota}, {"borrowingLimit", q.BorrowingLimit}, {"lendingLimit", q.LendingLimit}} {
				if field.value == "" {
					continue
				}
				if err := parseQuantity(field.value); err != nil {
					add(SevError, fmt.Sprintf("%s.quota.%s.%s", path, res, field.name), err.Error())
				}
			}
		}
		if n.FairSharingWeight != "" {
			if err := parseQuantity(n.FairSharingWeight); err != nil {
				add(SevError, path+".fairSharingWeight", err.Error())
			}
		}
		return nil
	})

	for i, rf := range c.ResourceFlavors {
		path := fmt.Sprintf("$.resourceFlavors[%d]", i)
		if rf.Name == "" {
			add(SevError, path+".name", "resourceFlavor needs a name")
		}
		for j, t := range rf.Taints {
			if t.Key == "" {
				add(SevError, fmt.Sprintf("%s.taints[%d].key", path, j), "taint key required")
			}
		}
	}
	for i, pc := range c.PriorityClasses {
		path := fmt.Sprintf("$.priorityClasses[%d]", i)
		if pc.Name == "" {
			add(SevError, path+".name", "priorityClass needs a name")
		}
	}
	return problems
}

// Problem is a validation diagnostic.
type Problem struct {
	Severity Severity `json:"severity"`
	Path     string   `json:"path"`
	Message  string   `json:"message"`
}

type Severity string

const (
	SevError   Severity = "error"
	SevWarning Severity = "warning"
	SevInfo    Severity = "info"
)

func nodePath(parents []*Node, name string) string {
	parts := make([]string, 0, len(parents)+1)
	for _, p := range parents {
		parts = append(parts, p.Name)
	}
	if name != "" {
		parts = append(parts, name)
	}
	return "$.root." + strings.Join(parts, ".")
}
