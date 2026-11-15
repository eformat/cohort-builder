package parse

import (
	"strings"
	"testing"

	"github.com/eformat/cohort-builder/internal/emit"
	"github.com/eformat/cohort-builder/internal/model"
)

func TestParseBookingLiveYAML(t *testing.T) {
	// mirrors the live ibm-tmm booking objects (v1beta2, cohortName fields)
	yaml := `
apiVersion: kueue.x-k8s.io/v1beta2
kind: Cohort
metadata:
  name: unreserved
spec:
  resourceGroups:
  - coveredResources: ["cpu", "memory", "nvidia.com/gpu"]
    flavors:
    - name: h200
      resources:
      - {name: cpu, nominalQuota: "159"}
      - {name: memory, nominalQuota: 1873Gi}
      - {name: nvidia.com/gpu, nominalQuota: "1"}
---
apiVersion: kueue.x-k8s.io/v1beta2
kind: ClusterQueue
metadata:
  name: unreserved
spec:
  cohortName: unreserved
  namespaceSelector: {}
  preemption:
    borrowWithinCohort: {policy: Never}
    reclaimWithinCohort: Never
    withinClusterQueue: Never
  flavorFungibility:
    whenCanBorrow: MayStopSearch
    whenCanPreempt: TryNextFlavor
  queueingStrategy: BestEffortFIFO
  resourceGroups:
  - coveredResources: ["cpu", "memory", "nvidia.com/gpu"]
    flavors:
    - name: h200
      resources:
      - {name: cpu, nominalQuota: "0"}
      - {name: memory, nominalQuota: "0"}
      - {name: nvidia.com/gpu, nominalQuota: "0"}
---
apiVersion: kueue.x-k8s.io/v1beta2
kind: ClusterQueue
metadata:
  name: user-avitui
spec:
  cohortName: unreserved
  namespaceSelector:
    matchLabels: {kubernetes.io/metadata.name: user-avitui}
  preemption:
    borrowWithinCohort: {policy: Never}
    reclaimWithinCohort: Any
    withinClusterQueue: Never
  flavorFungibility:
    whenCanBorrow: MayStopSearch
    whenCanPreempt: TryNextFlavor
  queueingStrategy: BestEffortFIFO
  resourceGroups:
  - coveredResources: ["cpu", "memory", "nvidia.com/gpu"]
    flavors:
    - name: h200
      resources:
      - {name: cpu, nominalQuota: "26"}
      - {name: memory, nominalQuota: 241Gi}
      - {name: nvidia.com/gpu, nominalQuota: "1"}
---
apiVersion: kueue.x-k8s.io/v1beta2
kind: LocalQueue
metadata:
  name: default
  namespace: user-avitui
spec:
  clusterQueue: user-avitui
`
	res, problems, err := ChartFromYAML(model.Version15, yaml)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, p := range problems {
		if p.Severity == model.SevError {
			t.Fatalf("unexpected error: %v", p)
		}
	}
	chart := res.Chart
	if chart.Root == nil || chart.Root.Name != "unreserved" {
		t.Fatalf("expected root cohort unreserved, got %+v", chart.Root)
	}
	if len(chart.Root.Children) != 1 {
		t.Fatalf("expected 1 child under unreserved (the merged node IS the unreserved CQ), got %d", len(chart.Root.Children))
	}

	// the merged cohort+CQ node: cohort unreserved also serves all namespaces
	if got := chart.Root.Namespaces; len(got) != 1 || got[0] != "*" {
		t.Errorf("expected merged node namespaces [\"*\"], got %v", got)
	}
	if chart.Root.Mode != model.ModePool {
		t.Errorf("expected merged node pool mode (all-zero quota), got %q", chart.Root.Mode)
	}
	if chart.Root.Preset != "unreserved" {
		t.Errorf("expected unreserved preset recognized, got %q", chart.Root.Preset)
	}

	// user CQ: guaranteed + booked-team
	var user *model.Node
	for _, c := range chart.Root.Children {
		if c.Name == "user-avitui" {
			user = c
		}
	}
	if user == nil {
		t.Fatal("user-avitui not attached under unreserved")
	}
	if user.Mode != model.ModeGuaranteed || user.Preset != "booked-team" {
		t.Errorf("user-avitui: mode=%q preset=%q, want guaranteed/booked-team", user.Mode, user.Preset)
	}
	if q := user.Quota["nvidia.com/gpu"]; q == nil || q.NominalQuota != "1" {
		t.Errorf("user-avitui gpu quota: %+v", user.Quota["nvidia.com/gpu"])
	}
}

func TestRoundTripEmitParseEmit(t *testing.T) {
	org := bookingTestOrg(model.Version15)
	first, err := emit.Emit(org)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	res, problems, err := ChartFromYAML(model.Version15, first.YAML)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, p := range problems {
		if p.Severity == model.SevError {
			t.Fatalf("round-trip parse error: %v", p)
		}
	}
	second, err := emit.Emit(res.Chart)
	if err != nil {
		t.Fatalf("re-emit: %v", err)
	}
	if first.YAML != second.YAML {
		t.Fatalf("round-trip not stable:\n--- first ---\n%s\n--- second ---\n%s", first.YAML, second.YAML)
	}
}

func TestParseCyclesAndUnknownParents(t *testing.T) {
	yaml := `
apiVersion: kueue.x-k8s.io/v1beta2
kind: Cohort
metadata: {name: a}
spec: {parentName: missing}
`
	res, _, err := ChartFromYAML(model.Version15, yaml)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if res.Chart.Root == nil || res.Chart.Root.Name != "a" {
		t.Fatalf("orphan cohort should become root, got %+v", res.Chart.Root)
	}
}

func bookingTestOrg(version string) *model.OrgChart {
	return &model.OrgChart{
		Version: version,
		Root: &model.Node{
			Name:   "unreserved",
			Kind:   model.KindOrg,
			Flavor: "h200",
			Quota: map[string]*model.ResourceQuota{
				"cpu":            {NominalQuota: "159"},
				"memory":         {NominalQuota: "1873Gi"},
				"nvidia.com/gpu": {NominalQuota: "1"},
			},
			Children: []*model.Node{
				{Name: "unreserved", Mode: model.ModePool, Preset: "unreserved", Namespaces: []string{"*"}},
				{Name: "user-avitui", Mode: model.ModeGuaranteed, Preset: "booked-team",
					Namespaces: []string{"user-avitui"},
					Quota: map[string]*model.ResourceQuota{
						"cpu":            {NominalQuota: "26"},
						"memory":         {NominalQuota: "241Gi"},
						"nvidia.com/gpu": {NominalQuota: "1"},
					}},
			},
		},
	}
}

var _ = strings.TrimSpace
