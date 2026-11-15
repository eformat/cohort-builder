package parse

import (
	"strings"
	"testing"

	"github.com/eformat/cohort-builder/internal/emit"
	"github.com/eformat/cohort-builder/internal/model"
)

// TestForestRoundTripNoSyntheticWiring — importing a cluster with several
// top-level objects (booking cohort + RHOAI default CQ) synthesizes an org-root
// UI grouping; emission must be TRANSPARENT to it: no org-root Cohort, no
// parentName/cohortName wired through it, and no LocalQueue flood from the
// wide namespace selector.
func TestForestRoundTripNoSyntheticWiring(t *testing.T) {
	yaml := `
apiVersion: kueue.x-k8s.io/v1beta2
kind: Cohort
metadata: {name: unreserved}
spec:
  resourceGroups:
  - coveredResources: ["nvidia.com/gpu"]
    flavors:
    - name: h200
      resources:
      - {name: nvidia.com/gpu, nominalQuota: "1"}
---
apiVersion: kueue.x-k8s.io/v1beta2
kind: ClusterQueue
metadata: {name: user-avitui}
spec:
  cohortName: unreserved
  namespaceSelector: {matchLabels: {kubernetes.io/metadata.name: user-avitui}}
---
apiVersion: kueue.x-k8s.io/v1beta2
kind: LocalQueue
metadata: {name: default, namespace: user-avitui}
spec: {clusterQueue: user-avitui}
---
apiVersion: kueue.x-k8s.io/v1beta2
kind: ClusterQueue
metadata: {name: default}
spec:
  namespaceSelector:
    matchExpressions:
    - {key: kubernetes.io/metadata.name, operator: In, values: [ns-a, ns-b, ns-c, ns-d, ns-e, ns-f, ns-g, ns-h, ns-i, ns-j]}
`
	res, problems, err := ChartFromYAML(model.Version15, yaml)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, p := range problems {
		if p.Severity == model.SevError {
			t.Fatalf("parse error: %v", p)
		}
	}
	if !res.Chart.Root.Synthetic {
		t.Fatal("expected synthetic org-root for a multi-root forest")
	}

	first, err := emit.Emit(res.Chart)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	y := first.YAML

	// transparency checks
	if strings.Contains(y, "name: org-root") {
		t.Error("synthetic org-root must not be emitted as a Cohort")
	}
	if strings.Contains(y, "parentName: org-root") {
		t.Error("unreserved must remain a ROOT cohort (no parentName wiring through org-root)")
	}
	if strings.Contains(y, "cohortName: org-root") {
		t.Error("standalone default CQ must not gain cohortName: org-root")
	}
	if lqCount := strings.Count(y, "kind: LocalQueue"); lqCount != 1 {
		t.Errorf("expected exactly 1 LocalQueue (user-avitui), got %d:\n%s", lqCount, y)
	}
	if strings.Contains(y, "namespace: ns-a") {
		t.Error("wide-selector CQ must not fan out LocalQueues")
	}
	if !strings.Contains(y, "namespace: user-avitui") {
		t.Error("explicit single-namespace CQ should keep its LocalQueue")
	}

	// re-parse must produce the same forest (stable round trip)
	res2, _, err := ChartFromYAML(model.Version15, y)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if !res2.Chart.Root.Synthetic || res2.Chart.Root.Name != "org-root" {
		t.Fatalf("re-parse should re-synthesize the org-root grouping, got %+v", res2.Chart.Root)
	}
	var cqNames []string
	walkNames(res2.Chart.Root, &cqNames)
	for _, want := range []string{"user-avitui", "default"} {
		found := false
		for _, n := range cqNames {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Errorf("re-parse lost CQ %q (found %v)", want, cqNames)
		}
	}
}

func walkNames(n *model.Node, out *[]string) {
	if n.HasClusterQueue() {
		*out = append(*out, n.Name)
	}
	for _, c := range n.Children {
		walkNames(c, out)
	}
}
