// Constructors building v0.19.0 (kueue-operator release-1.5) typed objects from
// neutral plans. Compile-time checked against the synced API snapshot: any field
// drift between Kueue versions fails the build here, not on a cluster.
package emit

import (
	"fmt"

	"github.com/eformat/cohort-builder/internal/kueueapi/v0_19_0"
	"github.com/eformat/cohort-builder/internal/model"
	"github.com/eformat/cohort-builder/internal/presets"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const apiV15 = v0_19_0.APIVersion

func newCohortObjectV0_19_0(p *cohortPlan) (*Object, error) {
	rgs, err := buildResources(p.Quota, p.Flavor, false)
	if err != nil {
		return nil, err
	}
	c := &v0_19_0.Cohort{
		TypeMeta: metav1.TypeMeta{Kind: "Cohort", APIVersion: apiV15},
		ObjectMeta: metav1.ObjectMeta{
			Name: p.Name,
		},
	}
	spec := v0_19_0.CohortSpec{}
	if p.ParentName != "" {
		spec.ParentName = v0_19_0.CohortReference(p.ParentName)
	}
	for _, rg := range rgs {
		spec.ResourceGroups = append(spec.ResourceGroups, resourceGroupV0_19_0(rg))
	}
	if p.Weight != "" {
		w, err := resource.ParseQuantity(p.Weight)
		if err != nil {
			return nil, fmt.Errorf("fairSharing weight: %w", err)
		}
		spec.FairSharing = &v0_19_0.FairSharing{Weight: &w}
	}
	c.Spec = spec
	return &Object{APIVersion: apiV15, Kind: "Cohort", Name: p.Name, Value: c}, nil
}

func newClusterQueueObjectV0_19_0(p *cqPlan) (*Object, error) {
	cq := &v0_19_0.ClusterQueue{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterQueue", APIVersion: apiV15},
		ObjectMeta: metav1.ObjectMeta{Name: p.Name},
	}
	spec := v0_19_0.ClusterQueueSpec{}
	if p.CohortName != "" {
		spec.CohortName = v0_19_0.CohortReference(p.CohortName)
	}

	// Quota placement: pool mode mirrors the cohort's covered resources with
	// nominalQuota 0 (the borrowing declaration); guaranteed mode carries the
	// node's own quota.
	switch {
	case p.Mode == model.ModePool && len(p.MirrorResources) > 0:
		quota := map[string]*model.ResourceQuota{}
		for _, r := range p.MirrorResources {
			quota[r] = &model.ResourceQuota{}
		}
		rgs, err := buildResources(quota, p.Flavor, true)
		if err != nil {
			return nil, err
		}
		for _, rg := range rgs {
			spec.ResourceGroups = append(spec.ResourceGroups, resourceGroupV0_19_0(rg))
		}
	default:
		rgs, err := buildResources(p.Quota, p.Flavor, false)
		if err != nil {
			return nil, err
		}
		for _, rg := range rgs {
			spec.ResourceGroups = append(spec.ResourceGroups, resourceGroupV0_19_0(rg))
		}
	}

	// Namespace admission: one CQ serves all its namespaces via an In expression.
	// ["*"] means all namespaces (empty selector, the live "unreserved" pattern).
	if len(p.Namespaces) == 1 && p.Namespaces[0] == "*" {
		spec.NamespaceSelector = &metav1.LabelSelector{}
	} else if len(p.Namespaces) == 1 {
		spec.NamespaceSelector = &metav1.LabelSelector{
			MatchLabels: map[string]string{"kubernetes.io/metadata.name": p.Namespaces[0]},
		}
	} else if len(p.Namespaces) > 1 {
		spec.NamespaceSelector = &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "kubernetes.io/metadata.name",
				Operator: metav1.LabelSelectorOpIn,
				Values:   p.Namespaces,
			}},
		}
	}

	// Preset → preemption/fungibility/queueing.
	pr, err := presets.Get(p.PresetKey)
	if err != nil {
		return nil, err
	}
	strategy := pr.QueueingStrategy
	if p.QueueingStrategy != "" {
		strategy = p.QueueingStrategy
	}
	if strategy != "" {
		spec.QueueingStrategy = v0_19_0.QueueingStrategy(strategy)
	}
	if pr.Preemption != nil {
		pre := &v0_19_0.ClusterQueuePreemption{}
		if pr.Preemption.ReclaimWithinCohort != "" {
			pre.ReclaimWithinCohort = v0_19_0.PreemptionPolicy(pr.Preemption.ReclaimWithinCohort)
		}
		if pr.Preemption.WithinClusterQueue != "" {
			pre.WithinClusterQueue = v0_19_0.PreemptionPolicy(pr.Preemption.WithinClusterQueue)
		}
		if pr.Preemption.BorrowPolicy != "" {
			bw := &v0_19_0.BorrowWithinCohort{Policy: v0_19_0.BorrowWithinCohortPolicy(pr.Preemption.BorrowPolicy)}
			if pr.Preemption.BorrowMaxPriorityThrh != 0 {
				t := pr.Preemption.BorrowMaxPriorityThrh
				bw.MaxPriorityThreshold = &t
			}
			pre.BorrowWithinCohort = bw
		}
		spec.Preemption = pre
	}
	if pr.FlavorFungibility != nil {
		spec.FlavorFungibility = &v0_19_0.FlavorFungibility{
			WhenCanBorrow:  v0_19_0.FlavorFungibilityPolicy(pr.FlavorFungibility.WhenCanBorrow),
			WhenCanPreempt: v0_19_0.FlavorFungibilityPolicy(pr.FlavorFungibility.WhenCanPreempt),
		}
	}
	if p.Weight != "" {
		w, err := resource.ParseQuantity(p.Weight)
		if err != nil {
			return nil, fmt.Errorf("fairSharing weight: %w", err)
		}
		spec.FairSharing = &v0_19_0.FairSharing{Weight: &w}
	}
	cq.Spec = spec
	return &Object{APIVersion: apiV15, Kind: "ClusterQueue", Name: p.Name, Value: cq}, nil
}

func resourceGroupV0_19_0(rg rgSpec) v0_19_0.ResourceGroup {
	out := v0_19_0.ResourceGroup{
		Flavors: []v0_19_0.FlavorQuotas{{
			Name: v0_19_0.ResourceFlavorReference(rg.Flavor),
		}},
	}
	for _, r := range rg.CoveredResources {
		out.CoveredResources = append(out.CoveredResources, corev1.ResourceName(r))
	}
	for _, r := range rg.Resources {
		fq := v0_19_0.ResourceQuota{
			Name:         corev1.ResourceName(r.Name),
			NominalQuota: resource.MustParse(r.NominalQuota),
		}
		if r.BorrowingLimit != nil {
			b := resource.MustParse(*r.BorrowingLimit)
			fq.BorrowingLimit = &b
		}
		if r.LendingLimit != nil {
			l := resource.MustParse(*r.LendingLimit)
			fq.LendingLimit = &l
		}
		out.Flavors[0].Resources = append(out.Flavors[0].Resources, fq)
	}
	return out
}

func newLocalQueueObjectV0_19_0(cqName, namespace, lqName string) Object {
	lq := &v0_19_0.LocalQueue{
		TypeMeta: metav1.TypeMeta{Kind: "LocalQueue", APIVersion: apiV15},
		ObjectMeta: metav1.ObjectMeta{
			Name:      lqName,
			Namespace: namespace,
		},
		Spec: v0_19_0.LocalQueueSpec{
			ClusterQueue: v0_19_0.ClusterQueueReference(cqName),
		},
	}
	return Object{APIVersion: apiV15, Kind: "LocalQueue", Name: lqName, Namespace: namespace, Value: lq}
}

func newResourceFlavorObjectV0_19_0(rf *model.ResourceFlavor) Object {
	out := &v0_19_0.ResourceFlavor{
		TypeMeta:   metav1.TypeMeta{Kind: "ResourceFlavor", APIVersion: apiV15},
		ObjectMeta: metav1.ObjectMeta{Name: rf.Name},
	}
	spec := v0_19_0.ResourceFlavorSpec{}
	if len(rf.NodeLabels) > 0 {
		spec.NodeLabels = rf.NodeLabels
	}
	for _, t := range rf.Taints {
		spec.NodeTaints = append(spec.NodeTaints, corev1.Taint{
			Key:    t.Key,
			Value:  t.Value,
			Effect: corev1.TaintEffect(t.Effect),
		})
	}
	out.Spec = spec
	return Object{APIVersion: apiV15, Kind: "ResourceFlavor", Name: rf.Name, Value: out}
}

func newPriorityClassObjectV0_19_0(pc *model.PriorityClass) (*Object, error) {
	out := &v0_19_0.WorkloadPriorityClass{
		TypeMeta:    metav1.TypeMeta{Kind: "WorkloadPriorityClass", APIVersion: apiV15},
		ObjectMeta:  metav1.ObjectMeta{Name: pc.Name},
		Value:       pc.Value,
		Description: pc.Description,
	}
	return &Object{APIVersion: apiV15, Kind: "WorkloadPriorityClass", Name: pc.Name, Value: out}, nil
}
