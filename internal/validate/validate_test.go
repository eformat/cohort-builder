package validate

import (
	"strings"
	"testing"

	"github.com/eformat/cohort-builder/internal/model"
	"github.com/eformat/cohort-builder/internal/presets"
)

func mustValidator(t *testing.T, version string) *Validator {
	t.Helper()
	v, err := New(version)
	if err != nil {
		t.Fatalf("New(%s): %v", version, version_err(version))
	}
	return v
}

func version_err(version string) string { return "" }

func TestValidChartClean(t *testing.T) {
	v := mustValidator(t, model.Version15)
	problems := v.ValidateChart(bookingTestChart(t))
	var errs []string
	for _, p := range problems {
		if p.Severity == model.SevError {
			errs = append(errs, p.Path+": "+p.Message)
		}
	}
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestLimitsOnRootCohortRejected(t *testing.T) {
	v := mustValidator(t, model.Version15)
	chart := bookingTestChart(t)
	chart.Root.Quota["nvidia.com/gpu"].BorrowingLimit = "4"
	problems := v.ValidateChart(chart)
	if !hasErrorContaining(problems, "borrowingLimit on root Cohort") {
		t.Fatalf("expected root-cohort borrowingLimit error, got %v", problems)
	}
}

func TestPoolModeWithoutCohortWarns(t *testing.T) {
	v := mustValidator(t, model.Version15)
	chart := &model.OrgChart{
		Version: model.Version15,
		Root: &model.Node{
			Name: "lonely-team", Mode: model.ModePool, Preset: "unreserved",
			Namespaces: []string{"lonely"},
		},
	}
	if !hasErrorContaining(v.ValidateChart(chart), "no cohort ancestor") {
		t.Fatal("expected pool-mode-without-cohort warning")
	}
}

func TestStructuralCatchesBadEnum(t *testing.T) {
	// reclaimWithinCohort: Sometimes is not in the CRD enum — the structural
	// layer must catch what the API server would reject.
	v := mustValidator(t, model.Version15)
	chart := bookingTestChart(t)
	chart.Root.Children[1].Preset = "" // custom: emit nothing by default
	// inject an invalid enum by setting a preset-less CQ with raw strategy
	chart.Root.Children[1].QueueingStrategy = "NotAStrategy"
	problems := v.ValidateChart(chart)
	if !hasErrorContaining(problems, "queueingStrategy") {
		t.Fatalf("expected structural enum error for queueingStrategy, got %v", problems)
	}
}

func TestValidateYAMLAgainstCRDs(t *testing.T) {
	v := mustValidator(t, model.Version15)
	good := `apiVersion: kueue.x-k8s.io/v1beta2
kind: ClusterQueue
metadata:
  name: x
spec:
  cohortName: c
  queueingStrategy: BestEffortFIFO
`
	bad := `apiVersion: kueue.x-k8s.io/v1beta2
kind: ClusterQueue
metadata:
  name: x
spec:
  cohortName: c
  preemption:
    reclaimWithinCohort: Sometimes
`
	if probs := v.ValidateYAML([]string{good}); hasError(probs) {
		t.Fatalf("good yaml should validate: %v", probs)
	}
	probs := v.ValidateYAML([]string{bad})
	if !hasErrorContaining(probs, "reclaimWithinCohort") {
		t.Fatalf("expected structural error for reclaimWithinCohort, got %v", probs)
	}
}

func TestPresetTableMatchesLiveCluster(t *testing.T) {
	// The presets must match the values observed on the live ibm-tmm cluster.
	if presets.BookedTeam.Preemption.ReclaimWithinCohort != "Any" ||
		presets.BookedTeam.Preemption.BorrowPolicy != "Never" ||
		presets.BookedTeam.Preemption.WithinClusterQueue != "Never" {
		t.Fatal("booked-team preset drifted from the live GPU Booking config")
	}
	if presets.UnreservedPriority.Preemption.BorrowMaxPriorityThrh != 100 {
		t.Fatal("unreserved-priority threshold drifted from the live config")
	}
	if presets.Unreserved.Preemption.ReclaimWithinCohort != "Never" {
		t.Fatal("unreserved preset drifted from the live config")
	}
}

func bookingTestChart(t *testing.T) *model.OrgChart {
	t.Helper()
	return &model.OrgChart{
		Version: model.Version15,
		Root: &model.Node{
			Name:   "unreserved",
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

func hasError(problems []model.Problem) bool {
	for _, p := range problems {
		if p.Severity == model.SevError {
			return true
		}
	}
	return false
}

func hasErrorContaining(problems []model.Problem, substr string) bool {
	for _, p := range problems {
		full := p.Path + " " + p.Message
		if p.Severity == model.SevError && strings.Contains(full, substr) {
			return true
		}
		if p.Severity == model.SevWarning && strings.Contains(full, substr) && !hasError(problems) {
			return true
		}
	}
	return false
}
