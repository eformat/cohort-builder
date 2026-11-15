// Package cluster reads live Kueue resources from a cluster, READ-ONLY. The
// kubeconfig comes from an explicit flag / the KUBECONFIG env via standard
// client-go loading rules — never from searching the filesystem. Only
// list/get verbs exist in this package by construction; the app has no
// cluster write paths in this phase.
package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/eformat/cohort-builder/internal/model"
	"github.com/eformat/cohort-builder/internal/parse"
	sigsyaml "sigs.k8s.io/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Snapshot is the raw read-only listing used by the UI and the importer.
type Snapshot struct {
	Context   string   `json:"context"`
	CSV       string   `json:"csv"`                 // kueue-operator CSV version, best effort
	Cohorts   []string `json:"cohorts"`             // names
	ClusterQs []string `json:"clusterQueues"`       // names
	Flavors   []string `json:"resourceFlavors"`     // names
	LocalQs   []string `json:"localQueues"`         // ns/name
	Classes   []string `json:"priorityClasses"`     // names
}

// Reader lists Kueue resources from one cluster context.
type Reader struct {
	kubeconfig string // explicit path ("" → standard loading rules)
}

// NewReader builds a reader. kubeconfigPath may be empty to use KUBECONFIG /
// ~/.kube/config (standard client-go rules only — no path hunting).
func NewReader(kubeconfigPath string) *Reader {
	return &Reader{kubeconfig: kubeconfigPath}
}

// restConfig resolves how to talk to a cluster, in precedence order:
//  1. explicit --kubeconfig path (flag)
//  2. KUBECONFIG env
//  3. ~/.kube/config (standard client-go rules)
//  4. in-cluster service-account config (when running as a pod)
//
// The kubeconfig path is read by the tool itself — never placed in argv.
// No filesystem hunting beyond the standard rules.
func (r *Reader) restConfig(contextName string) (*rest.Config, string, error) {
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil &&
		r.kubeconfig == "" && os.Getenv("KUBECONFIG") == "" && contextName == "" {
		restCfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, "", fmt.Errorf("in-cluster config: %w", err)
		}
		return restCfg, "in-cluster", nil
	}
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if r.kubeconfig != "" {
		loadingRules.ExplicitPath = r.kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}
	cfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	restCfg, err := cfg.ClientConfig()
	if err != nil {
		return nil, "", fmt.Errorf("kubeconfig not usable: %w", err)
	}
	raw, err := cfg.RawConfig()
	if err != nil {
		return nil, "", err
	}
	effective := contextName
	if effective == "" {
		effective = raw.CurrentContext
	}
	return restCfg, effective, nil
}

// inCluster reports whether the process runs inside a pod with a SA token.
func inCluster() bool {
	_, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token")
	return err == nil
}

func (r *Reader) Contexts() ([]string, string, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if r.kubeconfig != "" {
		loadingRules.ExplicitPath = r.kubeconfig
	}
	cfg, err := loadingRules.Load()
	if err != nil {
		return nil, "", err
	}
	names := make([]string, 0, len(cfg.Contexts))
	for name := range cfg.Contexts {
		names = append(names, name)
	}
	return names, cfg.CurrentContext, nil
}

// Snapshot lists the Kueue objects (names only) + operator CSV version.
func (r *Reader) Snapshot(ctx context.Context, contextName string) (*Snapshot, error) {
	restCfg, effective, err := r.restConfig(contextName)
	if err != nil {
		return nil, err
	}

	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}
	snap := &Snapshot{Context: effective}

	gvr := func(group, version, resource string) schema.GroupVersionResource {
		return schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	}

	// kueue resources (v1beta2)
	type listing struct {
		dst  *[]string
		gvr  schema.GroupVersionResource
		ns   string
		kind string
	}
	for _, l := range []listing{
		{&snap.Cohorts, gvr("kueue.x-k8s.io", "v1beta2", "cohorts"), "", "Cohort"},
		{&snap.ClusterQs, gvr("kueue.x-k8s.io", "v1beta2", "clusterqueues"), "", "ClusterQueue"},
		{&snap.Flavors, gvr("kueue.x-k8s.io", "v1beta2", "resourceflavors"), "", "ResourceFlavor"},
		{&snap.LocalQs, gvr("kueue.x-k8s.io", "v1beta2", "localqueues"), metav1.NamespaceAll, "LocalQueue"},
		{&snap.Classes, gvr("kueue.x-k8s.io", "v1beta2", "workloadpriorityclasses"), "", "WorkloadPriorityClass"},
	} {
		var list *unstructured.UnstructuredList
		var err error
		if l.ns != "" {
			list, err = dyn.Resource(l.gvr).Namespace(l.ns).List(ctx, metav1.ListOptions{})
		} else {
			list, err = dyn.Resource(l.gvr).List(ctx, metav1.ListOptions{})
		}
		if err != nil {
			// a cluster without Kueue CRDs: report which kind failed in the snapshot
			*l.dst = append(*l.dst, "ERR:"+err.Error())
			continue
		}
		for _, item := range list.Items {
			name := item.GetName()
			if l.ns != "" {
				name = item.GetNamespace() + "/" + name
			}
			*l.dst = append(*l.dst, name)
		}
	}
	snap.CSV = r.csvVersion(ctx, dyn)
	return snap, nil
}

// csvVersion finds the kueue-operator CSV, best effort (OLM may be absent).
// Lists NAMESPACED CSVs only — a cluster-wide list decodes every CSV on the
// cluster (RHOAI CSVs are huge and OOM'd a 1Gi pod). The kueue operator lives
// in openshift-kueue-operator (RHOAI convention) or openshift-operators.
func (r *Reader) csvVersion(ctx context.Context, dyn dynamic.Interface) string {
	csvGVR := schema.GroupVersionResource{
		Group: "operators.coreos.com", Version: "v1alpha1", Resource: "clusterserviceversions",
	}
	for _, ns := range []string{"openshift-kueue-operator", "openshift-operators"} {
		list, err := dyn.Resource(csvGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for _, item := range list.Items {
			name := item.GetName()
			if strings.HasPrefix(name, "kueue-operator.") {
				return name
			}
		}
	}
	return ""
}

// Import loads live Kueue objects and rebuilds the org chart via the same
// reverse-mapping pipeline as YAML import (convert unstructured → YAML → parse).
func (r *Reader) Import(ctx context.Context, version, contextName string) (*parse.Result, []model.Problem, error) {
	restCfg, _, err := r.restConfig(contextName)
	if err != nil {
		return nil, nil, err
	}
	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, nil, err
	}

	gvr := func(group, resource string) schema.GroupVersionResource {
		return schema.GroupVersionResource{Group: "kueue.x-k8s.io", Version: "v1beta2", Resource: resource}
	}

	var docs []string
	appendDocs := func(list *unstructured.UnstructuredList, err error, kind string) error {
		if err != nil {
			return fmt.Errorf("list %s: %w", kind, err)
		}
		for _, item := range list.Items {
			clean(&item)
			raw, err := json.Marshal(item.Object)
			if err != nil {
				continue
			}
			y, err := sigsyamlFromJSON(raw)
			if err != nil {
				continue
			}
			docs = append(docs, y)
		}
		return nil
	}

	listCohorts, err := dyn.Resource(gvr("kueue.x-k8s.io", "cohorts")).List(ctx, metav1.ListOptions{})
	if err := appendDocs(listCohorts, err, "Cohort"); err != nil {
		return nil, nil, err
	}
	listCQs, err := dyn.Resource(gvr("kueue.x-k8s.io", "clusterqueues")).List(ctx, metav1.ListOptions{})
	if err := appendDocs(listCQs, err, "ClusterQueue"); err != nil {
		return nil, nil, err
	}
	listRFs, err := dyn.Resource(gvr("kueue.x-k8s.io", "resourceflavors")).List(ctx, metav1.ListOptions{})
	if err := appendDocs(listRFs, err, "ResourceFlavor"); err != nil {
		return nil, nil, err
	}
	listWPCs, err := dyn.Resource(gvr("kueue.x-k8s.io", "workloadpriorityclasses")).List(ctx, metav1.ListOptions{})
	if err := appendDocs(listWPCs, err, "WorkloadPriorityClass"); err != nil {
		return nil, nil, err
	}
	// LocalQueues across namespaces — failures here are non-fatal (RBAC scope)
	listLQs, err := dyn.Resource(gvr("kueue.x-k8s.io", "localqueues")).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	_ = appendDocs(listLQs, err, "LocalQueue")

	if len(docs) == 0 {
		return nil, nil, fmt.Errorf("no Kueue objects found on context %q", contextName)
	}
	multi := strings.Join(docs, "\n---\n")
	return parse.ChartFromYAML(version, multi)
}

// clean strips server-side fields that don't round-trip into a planning doc.
func clean(u *unstructured.Unstructured) {
	u.SetUID("")
	u.SetResourceVersion("")
	u.SetGeneration(0)
	u.SetCreationTimestamp(metav1.Time{})
	u.SetManagedFields(nil)
	u.SetSelfLink("")
	ann := u.GetAnnotations()
	delete(ann, "kubectl.kubernetes.io/last-applied-configuration")
	delete(ann, "argocd.argoproj.io/tracking-id")
	u.SetAnnotations(ann)
	unstructured.RemoveNestedField(u.Object, "status")
}

func sigsyamlFromJSON(j []byte) (string, error) {
	out, err := sigsyaml.JSONToYAML(j)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
