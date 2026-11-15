// Help page: how to use cohort-builder — canvas, quota defaults, imports,
// YAML round-trip, validation, versions. Reached from the header "Help" item;
// replaces the canvas row while open.
export function HelpPage({ onClose }: { onClose: () => void }) {
  return (
    <div className="h-full overflow-y-auto bg-[#f5f5f5] dark:bg-[#151515]">
      <div className="mx-auto max-w-3xl px-6 py-8">
        <div className="flex items-start justify-between gap-4">
          <div>
            <div className="text-xs font-medium uppercase tracking-wide text-[#ee0000]">Help</div>
            <h1 className="font-display mt-1 text-2xl font-bold">How to use cohort-builder</h1>
            <p className="mt-2 text-sm text-[#6a6e73]">
              Plan GPU capacity sharing for your organization with Kueue + Red Hat OpenShift AI:
              draw the org as a tree, attach quota and preemption policy, and emit version-accurate
              Kueue YAML — or start from a slide diagram or a live cluster.
            </p>
          </div>
          <button
            onClick={onClose}
            className="shrink-0 rounded border border-[#c7c7c7] px-3 py-1 text-sm hover:bg-[#f2f2f2] dark:border-[#4d4d4d] dark:hover:bg-[#2a2a2a]"
          >
            ← Back to chart
          </button>
        </div>

        <nav className="mt-5 flex flex-wrap gap-1.5 text-xs">
          {TOC.map((t) => (
            <a
              key={t.id}
              href={`#${t.id}`}
              className="rounded-full border border-[#c7c7c7] bg-white px-2.5 py-1 text-[#0066cc] hover:bg-[#f0f6fc] dark:border-[#4d4d4d] dark:bg-[#1c1c1c] dark:hover:bg-[#262626]"
            >
              {t.label}
            </a>
          ))}
        </nav>

        <div className="mt-6 space-y-4 pb-12">
          <Section id="about" title="What is cohort-builder?">
            <p>
              You describe <em>who</em> gets <em>what</em> share of the GPU fleet as an org chart —
              company → divisions → teams — and the app turns that picture into Kueue manifests
              (<Code>Cohort</Code>, <Code>ClusterQueue</Code>, <Code>LocalQueue</Code>,{" "}
              <Code>ResourceFlavor</Code>, <Code>WorkloadPriorityClass</Code>) that are accurate for
              a specific kueue-operator release. The YAML drawer and validation pane keep the canvas
              and the manifests in sync in both directions, so you can start from a whiteboard
              diagram, a running cluster, or raw YAML.
            </p>
          </Section>

          <Section id="workspace" title="Projects, pages & versions">
            <ul className="list-disc space-y-1.5 pl-5">
              <li>
                <strong>Projects</strong> (menu, top-left) group your work; each project holds{" "}
                <strong>pages</strong> — the scenario tabs in the header. Use one page per what-if
                scenario; everything autosaves to a local SQLite database.
              </li>
              <li>
                The <strong>version selector</strong> sets which kueue-operator release branch the
                page targets: <Code>1.4</Code> → Kueue v0.18.1, <Code>1.5</Code> → Kueue v0.19.0. It
                controls which typed API snapshot emits and validates the YAML. Imports default to
                the latest release.
              </li>
              <li>
                The <strong>default GPU control</strong> in the header defines what "one unit of
                hardware" means for this page (resource, count, flavor). Default: 8×{" "}
                <Code>nvidia.com/gpu</Code> on the <Code>h200</Code> flavor — one full H200 node.
                It feeds the import defaults described below.
              </li>
            </ul>
          </Section>

          <Section id="canvas" title="Drawing the org chart">
            <ul className="list-disc space-y-1.5 pl-5">
              <li>
                Click a card to edit it in the <strong>Node</strong> tab (right pane): namespaces,
                GPU profile, custom resources, fair-sharing weight, preemption preset, flavor.
              </li>
              <li>
                <strong>+ Add child team</strong> appends a team under the selected node — a
                pool-mode borrower by default. <strong>Delete</strong> removes a node (never the
                root).
              </li>
              <li>
                Drag an <strong>edge endpoint</strong> onto another card to re-parent that subtree;
                deleting an edge drops the node back under the root. Cards drag by their title bar;
                the tree layout recomputes from the IR on every change.
              </li>
              <li>
                Blue-bordered cards are <strong>cohorts</strong> (quota levels); leaf cards are{" "}
                <strong>teams</strong> with mode/preset chips. The MiniMap (bottom-right) shows the
                whole tree — drag it to navigate; the canvas also auto-recenters on imports and page
                switches.
              </li>
              <li>
                Importing a diagram with several roots wraps them in a synthetic{" "}
                <Code>org</Code> grouping node — it is UI-only and never emitted as a Kueue Cohort.
              </li>
            </ul>
          </Section>

          <Section id="defaults" title="Quota model & defaults (the normal GPUaaS route)">
            <p>A team gets capacity one of two ways:</p>
            <ul className="list-disc space-y-1.5 pl-5">
              <li>
                <strong>Pool (borrower)</strong> — the team borrows from its cohort:{" "}
                <Code>nominalQuota: "0"</Code> plus a borrowing declaration, sharing the cohort's
                hardware with siblings. Kueue arbitrates by fair-sharing share values (weights) and
                the preset's preemption rules.
              </li>
              <li>
                <strong>Guaranteed (reserved)</strong> — the team carries its own{" "}
                <Code>nominalQuota</Code> directly; nothing above it is touched.
              </li>
            </ul>
            <p className="mt-2">On diagram imports (and when adding teams) the app applies:</p>
            <ul className="list-disc space-y-1.5 pl-5">
              <li>
                the <strong>top cohort</strong> (no cohort above it) receives the page's default GPU
                profile as the <em>hardware pool</em> — the pool lives there, teams borrow from it;
              </li>
              <li>
                teams under a cohort become <strong>pool-mode borrowers</strong> with the{" "}
                <Code>fair-share</Code> preset — total demand stays bounded by the real hardware;
              </li>
              <li>
                a <strong>standalone team</strong> with no cohort anywhere carries the profile
                directly (guaranteed — nothing to borrow from);
              </li>
              <li>
                quota numbers already shown in the diagram are respected (a slide saying
                "AI Innovators: 300" stays a 300 sub-pool);
              </li>
              <li>
                <strong>cluster imports are never touched</strong> by these defaults — real cluster
                data wins.
              </li>
            </ul>
          </Section>

          <Section id="presets" title="Preemption presets">
            <p>
              Values mirror the live GPU Booking cluster and the KubeCon fair-sharing deck. Pick one
              per ClusterQueue in the Node tab.
            </p>
            <Table
              head={["Preset", "Behaviour"]}
              rows={[
                [
                  "Reserved team (GPU Booking)",
                  "booked-team — a protected reservation: reclaims it from any borrower (reclaimWithinCohort: Any); never preempts its own queue; never borrows beyond the reservation.",
                ],
                [
                  "Shared borrower",
                  "unreserved — pure borrower from the cohort pool; no preemption rights at all.",
                ],
                [
                  "Priority borrower",
                  "unreserved-priority — preempts lower-priority borrowing workloads only (LowerPriority + borrow priority threshold 100).",
                ],
                [
                  "Fair-share member",
                  "fair-share — withinClusterQueue: LowerPriority, reclaimWithinCohort: Any; pairs with fair-sharing weights; preempted by share-value ordering.",
                ],
                [
                  "Custom (API defaults)",
                  "custom — emits no preemption/fungibility block; the Kueue API server defaults apply.",
                ],
              ]}
            />
          </Section>

          <Section id="profiles" title="GPU profiles">
            <p>
              From the fake-gpu-booking catalog. Setting a profile on a node stamps its GPU quota and
              the matching flavor; custom resource rows (in the Node tab) add anything else — cpu,
              memory, MIG slices.
            </p>
            <Table
              head={["Profile", "Resource", "Flavor", "Equivalent", "Per node"]}
              rows={[
                ["NVIDIA H200 (full GPU)", "nvidia.com/gpu", "h200", "1.0", "8"],
                ["MIG 3g.71gb (half GPU)", "nvidia.com/mig-3g.71gb", "h200", "0.5", "8"],
                ["MIG 2g.35gb (quarter GPU)", "nvidia.com/mig-2g.35gb", "h200", "0.25", "8"],
                ["MIG 1g.18gb (eighth GPU)", "nvidia.com/mig-1g.18gb", "h200", "0.125", "16"],
              ]}
            />
          </Section>

          <Section id="imports" title="Importing">
            <ul className="list-disc space-y-2 pl-5">
              <li>
                <strong>Import diagram</strong> — drop a PNG/JPEG of an org-chart slide;{" "}
                <Code>glm-5.3-flash</Code> extracts the topology (names, hierarchy, quota numbers
                when shown) and the import defaults above are applied. One diagram per import;
                extraction takes a few seconds (up to ~15s cold) — the canvas and drawer update as
                soon as it resolves. MultiKueue and ProvisioningRequestConfig aren't modelled yet.
              </li>
              <li>
                <strong>Import cluster</strong> — a read-only snapshot of the live Kueue state:
                cohorts, clusterqueues, localqueues, resourceflavors, workloadpriorityclasses. The
                client is GET-only by construction. Locally it uses your kubeconfig
                (<Code>KUBECONFIG</Code>); when deployed in-cluster it reads with the app's
                ServiceAccount (cluster-scoped, read-only RBAC). The snapshot reports the operator
                version it found, so you can tell when the live cluster runs something older than
                your target release. Imported data is never modified by the page defaults.
              </li>
              <li>
                <strong>YAML</strong> — paste manifests into the drawer and hit Apply to rebuild the
                tree (parent wiring and cycle detection included).
              </li>
            </ul>
          </Section>

          <Section id="yaml" title="YAML drawer">
            <ul className="list-disc space-y-1.5 pl-5">
              <li>
                Emits live as you edit the canvas (debounced) and <strong>follows the chart until
                you hand-edit the text</strong> — after that it stays on your version until you
                Apply or clear the edit.
              </li>
              <li>
                Document order is kubectl-apply-friendly: Cohorts (parents before children), then
                ClusterQueues/LocalQueues, then ResourceFlavors and WorkloadPriorityClasses.
              </li>
              <li>
                <strong>Copy/Download give the full document</strong> — the editor virtualizes long
                files, so the visible portion is not the whole YAML.
              </li>
              <li>
                Emission uses typed Go snapshots of the kueue-operator release APIs (v1beta2 for
                both 1.4 and 1.5), so field names match the target release — including drift such as{" "}
                <Code>cohort</Code> → <Code>cohortName</Code>.
              </li>
            </ul>
          </Section>

          <Section id="validation" title="Validation pane">
            <p>
              The <strong>Validation (N)</strong> tab groups problems by severity (errors, warnings,
              infos). Emitted objects are checked structurally against the CRD schemas embedded for
              the selected release, plus semantic checks — e.g. teams without namespaces still emit
              as ClusterQueues but raise a warning, and LocalQueues only fan out to ≤8 explicit
              namespaces because wider selectors don't behave as the diagram implies.
            </p>
          </Section>

          <Section id="kueue" title="Kueue version accuracy">
            <ul className="list-disc space-y-1.5 pl-5">
              <li>
                <Code>1.4</Code> targets Kueue v0.18.1 (kueue-operator <Code>release-1.4</Code>);{" "}
                <Code>1.5</Code> targets Kueue v0.19.0 (<Code>release-1.5</Code>). The API/CRD
                snapshots are synced from those branches (<Code>hack/sync-kueue.sh</Code>).
              </li>
              <li>
                Both versions emit the <Code>kueue.x-k8s.io/v1beta2</Code> API (v1beta1 is
                deprecated upstream). Capability differences per release are tracked in{" "}
                <Code>docs/capabilities.md</Code>.
              </li>
            </ul>
          </Section>

          <Section id="persistence" title="Saving & deployment">
            <ul className="list-disc space-y-1.5 pl-5">
              <li>
                Charts autosave (debounced) to SQLite (WAL mode) under the app's data directory —
                one writer, safe for a single replica.
              </li>
              <li>
                On OpenShift the app sits behind an oauth-proxy: you sign in with your cluster
                credentials, and cluster import runs with the pod's read-only ServiceAccount.
              </li>
              <li>
                The <strong>Light / Dark / Auto</strong> switch in the header controls the theme;
                Auto follows your OS setting, and the choice is remembered per browser.
              </li>
            </ul>
          </Section>
        </div>
      </div>
    </div>
  );
}

const TOC = [
  { id: "about", label: "What is it" },
  { id: "workspace", label: "Projects, pages & versions" },
  { id: "canvas", label: "Org chart canvas" },
  { id: "defaults", label: "Quota model & defaults" },
  { id: "presets", label: "Preemption presets" },
  { id: "profiles", label: "GPU profiles" },
  { id: "imports", label: "Importing" },
  { id: "yaml", label: "YAML drawer" },
  { id: "validation", label: "Validation" },
  { id: "kueue", label: "Kueue versions" },
  { id: "persistence", label: "Saving & deployment" },
];

function Section({ id, title, children }: { id: string; title: string; children: React.ReactNode }) {
  return (
    <section
      id={id}
      className="scroll-mt-4 rounded-md border border-[#e0e0e0] bg-white p-4 dark:border-[#383838] dark:bg-[#1c1c1c]"
    >
      <h2 className="font-display border-l-2 border-[#ee0000] pl-2 text-base font-semibold">
        {title}
      </h2>
      <div className="mt-2 text-sm leading-relaxed">{children}</div>
    </section>
  );
}

function Code({ children }: { children: React.ReactNode }) {
  return (
    <code className="rounded bg-[#f5f5f5] px-1 py-0.5 font-mono text-xs dark:bg-[#262626]">
      {children}
    </code>
  );
}

function Table({ head, rows }: { head: string[]; rows: string[][] }) {
  return (
    <div className="mt-2 overflow-x-auto">
      <table className="w-full border-collapse text-left text-xs">
        <thead>
          <tr className="border-b border-[#e0e0e0] dark:border-[#383838]">
            {head.map((h) => (
              <th key={h} className="px-2 py-1.5 font-medium uppercase tracking-wide text-[#6a6e73]">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i} className="border-b border-[#f0f0f0] last:border-0 dark:border-[#262626]">
              {r.map((c, j) => (
                <td key={j} className="px-2 py-1.5 align-top">
                  {c}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
