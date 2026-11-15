package parse

import (
	"fmt"

	"github.com/eformat/cohort-builder/internal/model"
)

// buildChartImpl assembles the node tree from cohort + CQ documents.
func buildChartImpl(version string, cohorts map[string]*cohortDoc, cqs map[string]*cqDoc, flavors []model.ResourceFlavor, pclasses []model.PriorityClass) (*model.OrgChart, []model.Problem) {
	var problems []model.Problem
	nodes := map[string]*model.Node{}

	// 1. nodes for every cohort
	for name, c := range cohorts {
		n := &model.Node{
			Name:              name,
			Kind:              model.KindDivision,
			Quota:             nonEmptyQuota(c.quota, c.hasQuota),
			Flavor:            c.flavor,
			FairSharingWeight: c.weight,
		}
		if c.parentName == "" {
			n.Kind = model.KindOrg
		}
		nodes[name] = n
	}

	// 2. nodes for CQs that are not already cohort nodes (Cohort "x" + CQ "x"
	// merge into one node with children AND namespaces — the live self-layer pattern)
	for name, q := range cqs {
		if _, isCohort := cohorts[name]; isCohort {
			continue
		}
		nodes[name] = &model.Node{
			Name:              name,
			Kind:              model.KindTeam,
			Quota:             nonEmptyQuota(q.quota, q.hasQuota),
			Flavor:            q.flavor,
			FairSharingWeight: q.weight,
			Namespaces:        q.namespaces,
			QueueingStrategy:  q.strategy,
		}
	}

	// 3. merged cohort+CQ nodes get the CQ's queue attributes
	for name, q := range cqs {
		n, ok := nodes[name]
		if !ok || n.Namespaces != nil {
			continue
		}
		n.Namespaces = q.namespaces
		n.QueueingStrategy = q.strategy
	}

	// 4. classify every CQ-bearing node: mode + preset
	for name, q := range cqs {
		n, ok := nodes[name]
		if !ok {
			continue
		}
		n.Mode = model.ModeGuaranteed
		if q.allZero && q.cohortName != "" {
			n.Mode = model.ModePool
		}
		n.Preset = recognizePreset(q.preemption, q.fungibility, q.strategy)
	}

	// 5. wire cohort parents
	for name, c := range cohorts {
		if c.parentName == "" {
			continue
		}
		parent, ok := nodes[c.parentName]
		if !ok {
			problems = append(problems, model.Problem{Severity: model.SevWarning,
				Path:    name,
				Message: fmt.Sprintf("Cohort %q references unknown parent %q — shown as a root until the parent is added", name, c.parentName)})
			continue
		}
		parent.Children = append(parent.Children, nodes[name])
	}

	// 6. attach CQs to their cohort nodes. A merged cohort+CQ node ("unreserved"
	// joining cohort "unreserved") is already its own cohort — self-membership
	// is implicit in the node, so attaching it to itself would create a cycle.
	for name, q := range cqs {
		n, ok := nodes[name]
		if !ok || q.cohortName == "" || name == q.cohortName {
			continue
		}
		cn, ok := nodes[q.cohortName]
		if !ok {
			problems = append(problems, model.Problem{Severity: model.SevWarning,
				Path:    name,
				Message: fmt.Sprintf("ClusterQueue %q references unknown cohort %q — shown as a root until the cohort is added", name, q.cohortName)})
			continue
		}
		cn.Children = append(cn.Children, n)
	}

	// 6b. break parentName cycles (Cohort a → b → a): Kueue disables all members
	// of a cyclic cohort tree, so a cyclic import is reported and the last edge
	// is cut (the tail becomes a root) to keep the canvas a tree.
	for name := range cohorts {
		seen := map[string]bool{name: true}
		cur := cohorts[name]
		for cur != nil && cur.parentName != "" {
			if seen[cur.parentName] {
				problems = append(problems, model.Problem{Severity: model.SevError,
					Path: name,
					Message: fmt.Sprintf("Cohort parentName cycle detected involving %q (Kueue disables all members of a cyclic tree) — edge %q → %q dropped for the canvas",
						name, cur.name, cur.parentName)})
				if n, ok := nodes[cur.name]; ok {
					n.Kind = model.KindOrg // demote to root-like
				}
				// cut the edge: rebuild children lists without the cyclic parent link
				if parent, ok := nodes[cur.parentName]; ok {
					filtered := parent.Children[:0]
					for _, c := range parent.Children {
						if c.Name != cur.name {
							filtered = append(filtered, c)
						}
					}
					parent.Children = filtered
				}
				break
			}
			seen[cur.parentName] = true
			cur = cohorts[cur.parentName]
		}
	}

	// 7. roots: cohorts without a parent (or with an unknown one), CQs without a cohort
	var roots []*model.Node
	for name, c := range cohorts {
		if c.parentName == "" {
			roots = append(roots, nodes[name])
		} else if _, parentExists := nodes[c.parentName]; !parentExists {
			roots = append(roots, nodes[name])
		}
	}
	for name, q := range cqs {
		if _, isCohort := cohorts[name]; isCohort {
			continue
		}
		if q.cohortName == "" {
			roots = append(roots, nodes[name])
		} else if _, ok := nodes[q.cohortName]; !ok {
			roots = append(roots, nodes[name])
		}
	}

	// 8. synthetic root when the forest has several trees
	var root *model.Node
	switch len(roots) {
	case 0:
		return nil, append(problems, model.Problem{Severity: model.SevError,
			Path: "$", Message: "no root Cohort/ClusterQueue found in the YAML"})
	case 1:
		root = roots[0]
	default:
		root = &model.Node{Name: "org-root", Kind: model.KindOrg, Synthetic: true, Description: "UI grouping — not emitted as a Cohort (multiple top-level objects found)"}
		root.Children = roots
		problems = append(problems, model.Problem{Severity: model.SevWarning,
			Path:    "$.root",
			Message: fmt.Sprintf("multiple top-level objects (%s) — synthesized an org-root node", joinNamesRoots(roots))})
	}

	return &model.OrgChart{
		Version:         version,
		Root:            root,
		ResourceFlavors: flavors,
		PriorityClasses: pclasses,
	}, problems
}

func nonEmptyQuota(q map[string]*model.ResourceQuota, has bool) map[string]*model.ResourceQuota {
	if !has || len(q) == 0 {
		return nil
	}
	return q
}

func joinNamesRoots(nodes []*model.Node) string {
	out := ""
	for i, n := range nodes {
		if i > 0 {
			out += ", "
		}
		out += n.Name
	}
	return out
}
