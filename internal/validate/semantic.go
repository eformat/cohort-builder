// Semantic validation: the invariants the Kueue webhook/scheduler enforce plus
// the operational lessons from running GPU Booking on RHOAI. Structural schema
// checks catch shape errors; these catch configurations that are well-formed
// but wrong.
package validate

import (
	"fmt"
	"sort"

	"github.com/eformat/cohort-builder/internal/model"
	"github.com/eformat/cohort-builder/internal/presets"
	"k8s.io/apimachinery/pkg/api/resource"
)

// semanticChecks runs over the chart tree and returns problems.
func semanticChecks(chart *model.OrgChart) []model.Problem {
	var out []model.Problem

	add := func(sev model.Severity, path, msg string) {
		out = append(out, model.Problem{Severity: sev, Path: path, Message: msg})
	}

	_ = chart.Root.Walk(func(n *model.Node, parents []*model.Node) error {
		path := nodePath(parents, n.Name)

		// Cohort-level limits require a parent cohort (Kueue webhook rejects
		// borrowing/lending limits on root cohorts).
		if n.HasChildren() && len(parents) == 0 && n.Quota != nil {
			for res, q := range n.Quota {
				if q == nil {
					continue
				}
				if q.BorrowingLimit != "" {
					add(model.SevError, fmt.Sprintf("%s.quota.%s.borrowingLimit", path, res),
						fmt.Sprintf("borrowingLimit on root Cohort %q is rejected by the Kueue webhook — limits are only valid on cohorts with a parent", n.Name))
				}
				if q.LendingLimit != "" {
					add(model.SevError, fmt.Sprintf("%s.quota.%s.lendingLimit", path, res),
						fmt.Sprintf("lendingLimit on root Cohort %q is rejected by the Kueue webhook — limits are only valid on cohorts with a parent", n.Name))
				}
			}
		}

		// Preset consistency for CQ nodes.
		if n.HasClusterQueue() || (!n.HasChildren() && n != chart.Root) {
			pr, err := presets.Get(n.Preset)
			if err != nil {
				add(model.SevError, path+".preset", err.Error())
			} else if pr.Preemption != nil && pr.Preemption.BorrowPolicy != "" && pr.Preemption.BorrowPolicy != "Never" {
				// borrowing-within-cohort only means something inside a cohort
				if model.CohortAncestorNonVirtual(n, parents) == nil {
					add(model.SevWarning, path+".preset",
						fmt.Sprintf("queue %q uses preset %q with borrowWithinCohort but has no cohort ancestor", n.Name, pr.Key))
				}
			}

			// Pool-mode CQ with nothing to borrow from admits nothing.
			if n.Mode == model.ModePool && model.CohortAncestorNonVirtual(n, parents) == nil {
				add(model.SevWarning, path+".mode",
					fmt.Sprintf("queue %q is pool-mode but has no cohort ancestor — with nominalQuota 0 it can never admit; use guaranteed mode or add a cohort parent", n.Name))
			}

			// A CQ with no quota and no cohort admits nothing.
			if n.Mode != model.ModePool && len(n.Quota) == 0 && model.CohortAncestorNonVirtual(n, parents) == nil {
				add(model.SevWarning, path+".quota",
					fmt.Sprintf("queue %q declares no quota and no cohort — it can never admit", n.Name))
			}

			// Team queue without namespaces: planned but not yet wired.
			if !n.HasChildren() && len(n.Namespaces) == 0 {
				add(model.SevWarning, path+".namespaces",
					fmt.Sprintf("team queue %q has no namespaces — it admits nothing until namespaces are set (Node pane → Namespaces, or * for all)", n.Name))
			}
		}

		// Fair-sharing weights only matter with the FairSharing feature gate on.
		if n.FairSharingWeight != "" {
			add(model.SevInfo, path+".fairSharingWeight",
				"fairSharing.weight only takes effect when FairSharing is enabled in the Kueue configuration (featureGate FairSharing=true)")
		}
		return nil
	})

	// ResourceFlavor taints vs RHOAI auto-queue management (ops finding 2026-10-26:
	// the RHOAI operator's queue management stripped RF specs within ~30s).
	for i, rf := range chart.ResourceFlavors {
		if len(rf.Taints) > 0 {
			add(model.SevWarning, fmt.Sprintf("$.resourceFlavors[%d].taints", i),
				fmt.Sprintf("ResourceFlavor %q carries taints — RHOAI's kueue queue-management (autoCreateQueues) has been observed stripping ResourceFlavor specs; deliver GPU node placement via HardwareProfile tolerations instead if queues are RHOAI-managed", rf.Name))
		}
	}

	out = append(out, additiveAccounting(chart)...)
	return out
}

// additiveAccounting checks guaranteed-mode child CQ quota against the cohort
// pool. Kueue cohort quota is ADDITIVE to member CQ quota (issue #5289) — a pool
// plus reservations over-commits the physical cluster silently.
func additiveAccounting(chart *model.OrgChart) []model.Problem {
	var out []model.Problem
	_ = chart.Root.Walk(func(n *model.Node, parents []*model.Node) error {
		if !n.HasChildren() || n.Quota == nil {
			return nil
		}
		path := nodePath(parents, n.Name)
		for res, q := range n.Quota {
			if q == nil || q.NominalQuota == "" {
				continue
			}
			pool, err := resource.ParseQuantity(q.NominalQuota)
			if err != nil {
				continue // already reported by model validation
			}
			var reserved resource.Quantity
			var contributors []string
			for _, c := range n.Children {
				if !c.HasClusterQueue() || c.Mode != model.ModeGuaranteed {
					continue
				}
				cq, ok := c.Quota[res]
				if !ok || cq.NominalQuota == "" {
					continue
				}
				qty, err := resource.ParseQuantity(cq.NominalQuota)
				if err != nil {
					continue
				}
				reserved.Add(qty)
				contributors = append(contributors, c.Name)
			}
			if len(contributors) == 0 {
				continue
			}
			sort.Strings(contributors)
			if reserved.Cmp(pool) > 0 {
				out = append(out, model.Problem{
					Severity: model.SevInfo,
					Path:     path + ".quota." + res,
					Message: fmt.Sprintf(
						"cohort %q pool for %s is %s and guaranteed child queues (%s) reserve %s = %s total. This is the normal booking-style shape: reservations carve out of, and are ADDITIVE to, the shared pool (kueue#5289). Only warn if total exceeds the node's physical capacity (not yet tracked here).",
						n.Name, res, pool.String(), joinNames(contributors), reserved.String(), sumOr(pool, reserved)),
				})
			} else {
				out = append(out, model.Problem{
					Severity: model.SevInfo,
					Path:     path + ".quota." + res,
					Message: fmt.Sprintf(
						"cohort %q pool %s + %s reserved by %s = %s total (cohort quota is additive, kueue#5289)",
						n.Name, pool.String(), reserved.String(), joinNames(contributors), sumOr(pool, reserved)),
				})
			}
		}
		return nil
	})
	return out
}

func joinNames(names []string) string {
	out := ""
	for i, s := range names {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

func sumOr(a, b resource.Quantity) string {
	s := a.DeepCopy()
	s.Add(b)
	return s.String()
}

func nodePath(parents []*model.Node, name string) string {
	p := "$.root"
	for _, pa := range parents {
		p += "." + pa.Name
	}
	if name != "" {
		p += "." + name
	}
	return p
}
