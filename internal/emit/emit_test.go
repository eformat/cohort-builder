package emit

import (
	"strings"
	"testing"

	"github.com/eformat/cohort-builder/internal/model"
)

// slidesOrg mirrors the KubeCon deck slides 32-48 org (AI Inc → AI Innovators /
// Next Generation → teams) as a hierarchical fair-sharing chart.
func slidesOrg(version string) *model.OrgChart {
	return &model.OrgChart{
		Version: version,
		Root: &model.Node{
			Name:              "ai-inc",
			Kind:              model.KindOrg,
			FairSharingWeight: "1",
			Quota: map[string]*model.ResourceQuota{
				"nvidia.com/gpu": {NominalQuota: "500", LendingLimit: "500"},
			},
			Children: []*model.Node{
				{
					Name:              "ai-innovators",
					Kind:              model.KindDivision,
					FairSharingWeight: "3",
					Quota: map[string]*model.ResourceQuota{
						"nvidia.com/gpu": {NominalQuota: "300"},
					},
					Children: []*model.Node{
						{
							Name:        "data-wizards",
							Kind:        model.KindTeam,
							Mode:        model.ModePool,
							Preset:      "fair-share",
							Namespaces:  []string{"data-wizards-lab"},
							Description: "borrows from ai-innovators cohort pool",
						},
						{
							Name:       "singularity-squad",
							Kind:       model.KindTeam,
							Mode:       model.ModeGuaranteed,
							Preset:     "booked-team",
							Namespaces: []string{"singularity-lab"},
							Quota: map[string]*model.ResourceQuota{
								"nvidia.com/gpu": {NominalQuota: "60"},
							},
						},
					},
				},
				{
					Name:              "next-generation",
					Kind:              model.KindDivision,
					FairSharingWeight: "1",
					Children: []*model.Node{
						{
							Name:       "back-propagators",
							Kind:       model.KindTeam,
							Mode:       model.ModePool,
							Preset:     "fair-share",
							Namespaces: []string{"backprop-lab"},
						},
					},
				},
			},
		},
		ResourceFlavors: []model.ResourceFlavor{
			{
				Name:       "default-flavor",
				NodeLabels: map[string]string{"nvidia.com/gpu.product": "H200"},
				Taints: []model.Taint{
					{Key: "nvidia.com/gpu", Value: "present", Effect: "NoSchedule"},
				},
			},
		},
		PriorityClasses: []model.PriorityClass{
			{Name: "train-high", Value: 1000, Description: "training runs"},
		},
	}
}

// bookingOrg mirrors the live GPU Booking cluster on ibm-tmm (2026-11-13):
// one flat cohort holding the h200 pool, user CQs with guaranteed reservations,
// unreserved + unreserved-priority borrowers.
func bookingOrg(version string) *model.OrgChart {
	gpuResources := func(quota map[string]string) map[string]*model.ResourceQuota {
		q := map[string]*model.ResourceQuota{}
		for k, v := range quota {
			q[k] = &model.ResourceQuota{NominalQuota: v}
		}
		return q
	}
	return &model.OrgChart{
		Version: version,
		Root: &model.Node{
			Name:   "unreserved",
			Kind:   model.KindOrg,
			Flavor: "h200",
			Quota: gpuResources(map[string]string{
				"cpu":                   "159",
				"memory":                "1873Gi",
				"nvidia.com/gpu":        "1",
				"nvidia.com/mig-1g.18gb": "16",
				"nvidia.com/mig-2g.35gb": "8",
				"nvidia.com/mig-3g.71gb": "8",
			}),
			Children: []*model.Node{
				{
					Name:       "unreserved",
					Mode:       model.ModePool,
					Preset:     "unreserved",
					Namespaces: []string{"*"},
					Flavor:     "h200",
				},
				{
					Name:       "unreserved-priority",
					Mode:       model.ModePool,
					Preset:     "unreserved-priority",
					Namespaces: []string{"*"},
					Flavor:     "h200",
				},
				{
					Name:       "user-avitui",
					Mode:       model.ModeGuaranteed,
					Preset:     "booked-team",
					Namespaces: []string{"user-avitui"},
					Flavor:     "h200",
					Quota: gpuResources(map[string]string{
						"cpu":            "26",
						"memory":         "241Gi",
						"nvidia.com/gpu": "1",
					}),
				},
			},
		},
	}
}

func TestEmitSlidesOrgV15(t *testing.T) {
	res, err := Emit(slidesOrg(model.Version15))
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	golden(t, "slides-org-1.5.yaml", res.YAML)

	// structural expectations (each CQ is followed by its LocalQueues)
	wantObjects := []string{
		"Cohort/ai-inc", "Cohort/ai-innovators", "Cohort/next-generation",
		"ClusterQueue/data-wizards", "LocalQueue/default@data-wizards-lab",
		"ClusterQueue/singularity-squad", "LocalQueue/default@singularity-lab",
		"ClusterQueue/back-propagators", "LocalQueue/default@backprop-lab",
		"ResourceFlavor/default-flavor", "WorkloadPriorityClass/train-high",
	}
	if got := strings.Join(summaryKeys(res.Objects), ","); got != strings.Join(wantObjects, ",") {
		t.Errorf("object order/identity mismatch:\n got: %s\nwant: %s", got, strings.Join(wantObjects, ","))
	}

	// pool-mode CQ must declare the cohort resources with nominalQuota 0
	for _, s := range res.Objects {
		if s.Name == "data-wizards" && !strings.Contains(res.YAML, "nominalQuota: \"0\"") {
			t.Errorf("pool-mode CQ should emit zero nominalQuota declarations")
		}
	}
}

func TestEmitBookingOrgV15(t *testing.T) {
	res, err := Emit(bookingOrg(model.Version15))
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	golden(t, "booking-flat-1.5.yaml", res.YAML)
}

func TestEmit14MatchesFieldNames(t *testing.T) {
	// 1.4 must emit v1beta2 field names too (cohortName, parentName) — the two
	// synced snapshots share the JSON surface (verified by the capability matrix).
	for _, v := range []string{model.Version14, model.Version15} {
		res, err := Emit(slidesOrg(v))
		if err != nil {
			t.Fatalf("emit %s: %v", v, err)
		}
		if !strings.Contains(res.YAML, "cohortName: ai-innovators") {
			t.Errorf("%s: expected cohortName: ai-innovators in output", v)
		}
		if !strings.Contains(res.YAML, "parentName: ai-inc") {
			t.Errorf("%s: expected parentName: ai-inc in output", v)
		}
		if strings.Contains(res.YAML, "apiVersion: kueue.x-k8s.io/v1beta1") {
			t.Errorf("%s: deprecated v1beta1 leaked into output", v)
		}
	}
}

func TestEmitUnknownPresetFails(t *testing.T) {
	org := bookingOrg(model.Version15)
	org.Root.Children[2].Preset = "bogus"
	if _, err := Emit(org); err == nil {
		t.Fatal("expected error for unknown preset")
	}
}

func TestEmitEmptyChartFails(t *testing.T) {
	if _, err := Emit(&model.OrgChart{Version: model.Version15}); err == nil {
		t.Fatal("expected error for empty chart")
	}
}

func summaryKeys(summaries []Summary) []string {
	keys := make([]string, 0, len(summaries))
	for _, s := range summaries {
		k := s.Kind + "/" + s.Name
		if s.Namespace != "" {
			k += "@" + s.Namespace
		}
		keys = append(keys, k)
	}
	return keys
}
