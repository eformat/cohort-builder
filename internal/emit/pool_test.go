package emit

import (
	"strings"
	"testing"

	"github.com/eformat/cohort-builder/internal/model"
)

// TestPoolBorrowThroughPassThroughCohort — the normal GPUaaS shape from the
// KubeCon slides: the hardware pool sits on the ROOT cohort, intermediate
// cohorts are pass-through levels, and teams borrow (nominalQuota 0) through
// the chain. A team's borrowing declaration must cover resources that live
// anywhere up its ancestor chain, not just the nearest cohort.
func TestPoolBorrowThroughPassThroughCohort(t *testing.T) {
	chart := &model.OrgChart{
		Version: model.Version15,
		Root: &model.Node{
			Name: "ai-inc",
			Kind: model.KindOrg,
			Quota: map[string]*model.ResourceQuota{
				"nvidia.com/gpu": {NominalQuota: "8", LendingLimit: "8"},
			},
			Children: []*model.Node{
				{
					Name: "ai-innovators",
					Kind: model.KindDivision,
					// pass-through: no quota
					Children: []*model.Node{
						{
							Name:       "data-wizards",
							Kind:       model.KindTeam,
							Mode:       model.ModePool,
							Preset:     "fair-share",
							Namespaces: []string{"data-wizards-lab"},
						},
					},
				},
			},
		},
	}

	res, err := Emit(chart)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	y := res.YAML

	// the team CQ must declare nvidia.com/gpu with nominalQuota 0 (borrowing)
	cqDoc := docFor(t, y, "ClusterQueue", "data-wizards")
	if !strings.Contains(cqDoc, "nvidia.com/gpu") {
		t.Fatalf("pool team must mirror the root pool resource, got:\n%s", cqDoc)
	}
	if !strings.Contains(cqDoc, `nominalQuota: "0"`) {
		t.Fatalf("pool team must borrow with nominalQuota 0, got:\n%s", cqDoc)
	}
	if strings.Contains(cqDoc, `nominalQuota: "8"`) {
		t.Fatalf("pool team must NOT carry the pool on itself, got:\n%s", cqDoc)
	}
	// and the root cohort holds the hardware
	cohortDoc := docFor(t, y, "Cohort", "ai-inc")
	if !strings.Contains(cohortDoc, `nominalQuota: "8"`) {
		t.Fatalf("root cohort must hold the pool, got:\n%s", cohortDoc)
	}
}

// TestStandaloneLeafFillsGuaranteed — a single team with no cohort anywhere
// cannot borrow; the sane default for it is its own (guaranteed) quota.
func TestStandaloneLeafFillsGuaranteed(t *testing.T) {
	chart := &model.OrgChart{
		Version: model.Version15,
		Root: &model.Node{
			Name:  "lone-team",
			Kind:  model.KindTeam,
			Mode:  model.ModeGuaranteed,
			Quota: map[string]*model.ResourceQuota{"nvidia.com/gpu": {NominalQuota: "8"}},
		},
	}
	res, err := Emit(chart)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	cqDoc := docFor(t, res.YAML, "ClusterQueue", "lone-team")
	if !strings.Contains(cqDoc, `nominalQuota: "8"`) || strings.Contains(cqDoc, "cohortName") {
		t.Fatalf("standalone team should carry its own quota with no cohort, got:\n%s", cqDoc)
	}
}

func docFor(t *testing.T, multi, kind, name string) string {
	t.Helper()
	for _, doc := range strings.Split(multi, "---") {
		if strings.Contains(doc, "kind: "+kind) && strings.Contains(doc, "name: "+name) {
			return doc
		}
	}
	t.Fatalf("doc not found: %s/%s", kind, name)
	return ""
}
