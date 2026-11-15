// Package gpuprofiles holds the common GPU profile catalog for the Node pane.
// Source of truth: the fake-gpu-booking gpuConfig (H200 node + MIG slices),
// so the builder's presets match what the fake cluster actually exposes.
package gpuprofiles

// Profile is one selectable GPU resource preset.
type Profile struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Resource string  `json:"resource"`      // quota resource name
	Flavor   string  `json:"flavor"`        // suggested ResourceFlavor name
	Equivalent float64 `json:"equivalent"`  // GPU-equivalent weight (booking semantics)
	ClusterCount int   `json:"clusterCount"`  // reference count on the fake cluster
}

// All lists the catalog in picker order.
var All = []Profile{
	{
		Key: "h200", Label: "NVIDIA H200 (full GPU)", Resource: "nvidia.com/gpu",
		Flavor: "h200", Equivalent: 1.0, ClusterCount: 8,
	},
	{
		Key: "mig-3g-71gb", Label: "MIG 3g.71gb (half GPU)", Resource: "nvidia.com/mig-3g.71gb",
		Flavor: "h200", Equivalent: 0.5, ClusterCount: 8,
	},
	{
		Key: "mig-2g-35gb", Label: "MIG 2g.35gb (quarter GPU)", Resource: "nvidia.com/mig-2g.35gb",
		Flavor: "h200", Equivalent: 0.25, ClusterCount: 8,
	},
	{
		Key: "mig-1g-18gb", Label: "MIG 1g.18gb (eighth GPU)", Resource: "nvidia.com/mig-1g.18gb",
		Flavor: "h200", Equivalent: 0.125, ClusterCount: 16,
	},
}

// Get returns a profile by key.
func Get(key string) (Profile, bool) {
	for _, p := range All {
		if p.Key == key {
			return p, true
		}
	}
	return Profile{}, false
}
