# cohort-builder — GPUaaS quota planning with Kueue + RHOAI

Plan GPU quota configuration using Kueue Cohorts, ClusterQueues and fair sharing,
for the kueue-operator releases RHOAI ships. Two directions, one canvas:

- **Org chart → YAML**: build the org hierarchy on an infinite canvas (or import a
  diagram PNG/JPEG — glm-5.3-flash extracts the chart), emit version-accurate Kueue YAML
- **YAML → Org chart**: paste live or hand-written Kueue YAML, get the rendered
  org tree plus validation against the exact CRDs the operator ships

Version selector matches the kueue-operator release branches:

| Selector | Branch | Kueue operand |
|---|---|---|
| 1.4 | `upstream/release-1.4` | sigs.k8s.io/kueue v0.18.1 |
| 1.5 | `upstream/release-1.5` | sigs.k8s.io/kueue v0.19.0 |

## No templates, ever

YAML is emitted from **typed Go objects** — exact snapshots of the Kueue API types
vendored in each kueue-operator branch, synced by `hack/sync-kueue.sh` into
`internal/kueueapi/v0_18_1` and `v0_19_0`. Field drift between versions fails the
build, not a cluster. Serialization is typed-struct → JSON (field order preserved)
→ `JSONToYAML` (key order preserved → kubectl-style output). Validation runs each
emitted object against the CRDs the operator itself installs (`internal/crds/`).
The capability matrix (`docs/capabilities.md`) is generated from the real type diff.

## Presets (the GPU Booking point of view)

`internal/presets` encodes the preemption model proven on the live GPU Booking
cluster: reserved teams (`reclaimWithinCohort: Any`, `borrowWithinCohort: Never`),
pure borrowers, priority borrowers (threshold 100), and hierarchical fair sharing
(KubeCon slides 32–48). Cluster import recognizes these presets in live objects.

## Quick start (local-first)

```bash
make dev          # vite dev server (:5173, proxies /api) + Go API (:8080)
make test         # go test ./...
make web-build    # build the UI into internal/server/static
make build        # single binary: cmd/server (+ embedded UI)
make sync-kueue   # re-sync API types + CRDs from kueue-operator branches
```

Then open http://localhost:5173 (dev) or http://localhost:8080 (built).

### LLM diagram import (env only — never flags/argv)

```bash
export OPENAI_BASE_URL=https://<maas-endpoint>/v1
export OPENAI_API_KEY=...
export COHORT_LLM_MODEL=publishers/prelude-maas/models/glm-53-flash  # default
```

### Read-only cluster import

```bash
export KUBECONFIG=~/.kube/config.ibm-tmm   # or --kubeconfig <path>
```

The importer only ever lists/gets (GET-only client by construction); the kubeconfig
path comes from the flag/env via standard client-go rules — nothing is searched,
nothing credential-bearing is stored.

## Storage

SQLite (WAL) at `~/.local/share/cohort-builder/cohort.db` — projects and scenario
pages, autosaved. Multi-tab sync is client-side (BroadcastChannel pattern).

## Layout

```
cmd/server/            HTTP API + embedded UI
internal/model/        org-chart IR (version-agnostic)
internal/kueueapi/     synced typed Kueue API snapshots + capability matrix
internal/crds/         synced operator CRDs (structural validation)
internal/emit/         IR → typed objects → YAML (per version, no templates)
internal/parse/        YAML → IR (reverse mapping + preset recognition)
internal/validate/     CRD structural + semantic invariants
internal/presets/      GPU Booking / fair-sharing preemption presets
internal/llm/          glm-5.3-flash diagram import (image → IR only)
internal/cluster/      read-only kubeconfig client (live import)
internal/store/        SQLite projects/pages
web/                   React 19 + Vite + Tailwind + React Flow canvas
hack/                  sync-kueue.sh + capabilities.py (generators)
docs/capabilities.md   generated field/enum diff between 1.4 and 1.5
```

The UI shell follows the PrettyFish workspace model (canvas, editor cards,
multi-page projects, themes); Red Hat palette from the j8fns postmortem.

## Status / later phases

- Cluster **write-back / dry-run apply**: later phase — the app is plan-only today
- ProvisioningRequestConfig + MultiKueue emission: later phase (greyed out in the
  capability matrix)
- Container image: `make container` (multi-stage, node build → go build)

Attribution: UI patterns adapted from eformat/prettyfish (Apache-2.0).
