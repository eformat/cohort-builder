// Typed API client for the cohort-builder backend.
import type { OrgChart, Problem, Preset, Project, Page, VersionInfo, GPUProfile } from "./types";

async function j<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let msg = `${res.status}`;
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  return res.json() as Promise<T>;
}

export const api = {
  versions: () => fetch("/api/versions").then((r) => j<VersionInfo[]>(r)),

  presets: () => fetch("/api/presets").then((r) => j<Preset[]>(r)),

  gpuProfiles: () => fetch("/api/gpu-profiles").then((r) => j<GPUProfile[]>(r)),

  parse: (version: string, yaml: string) =>
    fetch("/api/parse", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ version, yaml }),
    }).then((r) =>
      j<{ chart: OrgChart | null; objects?: string[]; problems: Problem[] }>(r),
    ),

  emit: (version: string, chart: OrgChart) =>
    fetch("/api/emit", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ version, chart }),
    }).then((r) =>
      j<{ yaml: string; objects: { kind: string; name: string; namespace?: string }[]; problems: Problem[] }>(r),
    ),

  importImage: (version: string, imageDataUrl: string) =>
    fetch("/api/import/image", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ version, image: imageDataUrl }),
    }).then((r) => j<{ chart: OrgChart; problems: Problem[] }>(r)),

  clusterContexts: () =>
    fetch("/api/cluster/contexts").then((r) => j<{ contexts: string[]; current: string }>(r)),

  clusterImport: (version: string, context: string) =>
    fetch("/api/cluster/import", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ version, context }),
    }).then((r) => j<{ chart: OrgChart; problems: Problem[] }>(r)),

  projects: () => fetch("/api/projects").then((r) => j<Project[]>(r)),

  createProject: (name: string) =>
    fetch("/api/projects", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    }).then((r) => j<Project>(r)),

  deleteProject: (id: number) =>
    fetch(`/api/projects/${id}`, { method: "DELETE" }).then((r) => j<unknown>(r)),

  pages: (projectId: number) =>
    fetch(`/api/projects/${projectId}/pages`).then((r) => j<Page[]>(r)),

  createPage: (projectId: number, page: Partial<Page>) =>
    fetch(`/api/projects/${projectId}/pages`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(page),
    }).then((r) => j<Page>(r)),

  savePage: (id: number, page: Partial<Page>) =>
    fetch(`/api/pages/${id}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(page),
    }).then((r) => j<Page>(r)),

  deletePage: (id: number) => fetch(`/api/pages/${id}`, { method: "DELETE" }).then((r) => j<unknown>(r)),
};
