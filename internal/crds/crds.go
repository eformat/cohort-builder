// Package crds embeds the operator-shipped Kueue CRDs per release (synced by
// hack/sync-kueue.sh) for structural validation.
package crds

import "embed"

//go:embed all:v0_18_1 all:v0_19_0
var FS embed.FS

// DirFor maps a release selector ("1.4"/"1.5") to the embedded CRD directory.
func DirFor(version string) string {
	switch version {
	case "1.4":
		return "v0_18_1"
	case "1.5":
		return "v0_19_0"
	default:
		return ""
	}
}
