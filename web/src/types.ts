// IR types mirrored from internal/model — keep in sync (single source: the Go IR).
export type QuotaMode = "pool" | "guaranteed";

export interface ResourceQuota {
  nominalQuota?: string;
  borrowingLimit?: string;
  lendingLimit?: string;
}

export interface Taint {
  key: string;
  value?: string;
  effect?: string;
}

export interface Node {
  name: string;
  kind?: "org" | "division" | "team";
  description?: string;
  quota?: Record<string, ResourceQuota>;
  fairSharingWeight?: string;
  namespaces?: string[];
  mode?: QuotaMode;
  preset?: string;
  queueingStrategy?: string;
  flavor?: string;
  localQueueName?: string;
  /** UI-only grouping node (multi-root import) — never emitted as a Cohort */
  synthetic?: boolean;
  children?: Node[];
}

export interface ResourceFlavor {
  name: string;
  nodeLabels?: Record<string, string>;
  taints?: Taint[];
}

export interface PriorityClass {
  name: string;
  value: number;
  description?: string;
}

export interface Defaults {
  resource?: string;
  count?: number;
  flavor?: string;
}

export interface OrgChart {
  version: string;
  root: Node | null;
  defaults?: Defaults;
  resourceFlavors?: ResourceFlavor[];
  priorityClasses?: PriorityClass[];
}

export interface Problem {
  severity: "error" | "warning" | "info";
  path: string;
  message: string;
}

export interface VersionInfo {
  key: string;
  label: string;
  operand: string;
}

export interface Preset {
  key: string;
  label: string;
  description: string;
  queueingStrategy?: string;
}

export interface GPUProfile {
  key: string;
  label: string;
  resource: string;
  flavor: string;
  equivalent: number;
  clusterCount: number;
}

export interface Page {
  id: number;
  projectId: number;
  name: string;
  position: number;
  version: string;
  state: string;
  updated?: string;
}

export interface Project {
  id: number;
  name: string;
  created?: string;
  updated?: string;
}
