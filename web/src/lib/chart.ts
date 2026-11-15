// Org-chart tree helpers shared by the canvas and the editor pane.
import type { Node, OrgChart, ResourceQuota } from "../types";

export function walk(n: Node, fn: (n: Node, parents: Node[]) => void, parents: Node[] = []): void {
  fn(n, parents);
  const kids: Node[] = n.children ?? [];
  for (const c of kids) walk(c, fn, [...parents, n]);
}

export function findNode(root: Node | null, name: string): Node | null {
  if (!root) return null;
  let hit: Node | null = null;
  walk(root, (n) => {
    if (n.name === name) hit = n;
  });
  return hit;
}

export function removeNode(root: Node, name: string): boolean {
  if (!root.children) return false;
  const idx = root.children.findIndex((c) => c.name === name);
  if (idx >= 0) {
    root.children.splice(idx, 1);
    return true;
  }
  for (const c of root.children) {
    if (removeNode(c, name)) return true;
  }
  return false;
}

// reparent moves `name` under `newParent` (cycle-safe).
export function reparent(root: Node, name: string, newParent: string): boolean {
  if (name === newParent) return false;
  // refuse cycles: newParent must not be a descendant of name
  let cyclic = false;
  walk(findNode(root, name) ?? root, (n) => {
    if (n.name === newParent) cyclic = true;
  });
  if (cyclic) return false;
  const node = findNode(root, name);
  const parent = findNode(root, newParent);
  if (!node || !parent) return false;
  removeNode(root, name);
  parent.children = parent.children ?? [];
  parent.children.push(node);
  return true;
}

export function quotaSummary(n: Node): string[] {
  const out: string[] = [];
  for (const [res, q] of Object.entries(n.quota ?? {})) {
    const q2 = (q ?? {}) as ResourceQuota;
    const short = res.includes("/") ? res.split("/")[1] : res;
    out.push(`${short}: ${q2.nominalQuota ?? "0"}`);
  }
  return out.slice(0, 4);
}

export function defaultChart(version: string): OrgChart {
  return {
    version,
    root: {
      name: "ai-inc",
      kind: "org",
      fairSharingWeight: "1",
      quota: { "nvidia.com/gpu": { nominalQuota: "500" } },
      children: [
        {
          name: "team-one",
          kind: "team",
          mode: "pool",
          preset: "booked-team",
          namespaces: ["team-one-lab"],
        },
      ],
    },
    resourceFlavors: [],
    priorityClasses: [],
  };
}

export function emptyChart(version: string): OrgChart {
  return {
    version,
    root: { name: "org", kind: "org" },
    resourceFlavors: [],
    priorityClasses: [],
  };
}

/**
 * Apply the page's default GPU profile the "normal GPUaaS" way:
 *  - the TOP cohort (no cohort above it) receives the hardware pool
 *    (e.g. one 8× H200 server) when the diagram/import shows no quota there
 *  - teams (leaves) under a cohort become pool-mode borrowers
 *    (nominalQuota 0 + fair-share preset) — they share the pool
 *  - a standalone team with no cohort anywhere carries the profile directly
 *    (guaranteed — there is nothing to borrow from)
 * Cluster imports never call this: real cluster data wins.
 */
export function applyDefaultGPU(chart: OrgChart, d: { resource?: string; count?: number; flavor?: string }): void {
  if (!chart.root || !d.resource) return;
  const resource: string = d.resource;
  walk(chart.root, (n, parents) => {
    const hasGPU = Object.keys(n.quota ?? {}).some((r) => r.includes("gpu") || r.includes("mig"));
    const isLeaf = !n.children || n.children.length === 0;
    const cohortAncestor = [...parents].reverse().find((p) => !p.synthetic && (p.children?.length ?? 0) > 0);

    if (isLeaf && !hasGPU) {
      if (cohortAncestor) {
        // normal route: borrow from the cohort pool, share it fairly
        n.mode = "pool";
        if (!n.preset) n.preset = "fair-share";
      } else {
        // nothing to borrow from: direct guaranteed hardware
        n.mode = "guaranteed";
        n.quota = { ...(n.quota ?? {}), [resource]: { nominalQuota: String(d.count || 1) } };
        n.flavor = n.flavor ?? d.flavor;
      }
      return;
    }

    // top cohort = a cohort with no non-synthetic cohort above it
    const isTopCohort = !isLeaf && !n.synthetic && !parents.some((p) => !p.synthetic && (p.children?.length ?? 0) > 0);
    if (isTopCohort && !hasGPU) {
      // the hardware pool lives here; teams below borrow from it
      n.quota = { ...(n.quota ?? {}), [resource]: { nominalQuota: String(d.count || 1) } };
      n.flavor = n.flavor ?? d.flavor;
    }
  });
}
