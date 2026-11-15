// Import menu: diagram image (glm-5.3-flash) and read-only live cluster import.
// Version defaults to the latest release; importing switches the page version.
import { useEffect, useRef, useState } from "react";
import { api } from "../api";
import type { OrgChart, Problem, VersionInfo } from "../types";

export function ImportMenu({
  version,
  onVersionChange,
  onImported,
}: {
  version: string;
  onVersionChange: (v: string) => void;
  onImported: (chart: OrgChart, version: string) => void;
}) {
  const [versions, setVersions] = useState<VersionInfo[]>([]);
  const [importVersion, setImportVersion] = useState(version);
  const [busy, setBusy] = useState<string>("");
  const [problems, setProblems] = useState<Problem[]>([]);
  const [error, setError] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    api.versions().then((vs) => {
      setVersions(vs);
      // default the import version to the latest release
      if (vs.length > 0) setImportVersion(vs[vs.length - 1].key);
    }).catch(() => {});
  }, []);

  const doImport = async (fn: (v: string) => Promise<{ chart: OrgChart; problems: Problem[] }>, label: string) => {
    setBusy(label);
    setError("");
    setProblems([]);
    try {
      const res = await fn(importVersion);
      onImported(res.chart, importVersion);
      setProblems(res.problems ?? []);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy("");
    }
  };

  const importImage = (file: File) =>
    doImport(async (v) => {
      const dataUrl = await new Promise<string>((resolve, reject) => {
        const r = new FileReader();
        r.onload = () => resolve(String(r.result));
        r.onerror = reject;
        r.readAsDataURL(file);
      });
      return api.importImage(v, dataUrl);
    }, "Extracting org chart with glm…");

  const importCluster = () => doImport((v) => api.clusterImport(v, ""), "Reading cluster (read-only)…");

  return (
    <div className="flex items-center gap-2 text-sm">
      <input
        ref={fileRef}
        type="file"
        accept="image/png,image/jpeg"
        className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) void importImage(f);
          e.target.value = "";
        }}
      />
      <select
        value={importVersion}
        onChange={(e) => {
          setImportVersion(e.target.value);
          onVersionChange(e.target.value);
        }}
        className="rounded border border-[#c7c7c7] bg-white px-2 py-1 text-xs dark:border-[#4d4d4d] dark:bg-[#2a2a2a]"
        title="Version used for imports"
      >
        {versions.map((v) => (
          <option key={v.key} value={v.key}>
            import as {v.key}
          </option>
        ))}
      </select>
      <button
        onClick={() => fileRef.current?.click()}
        disabled={!!busy}
        className="rounded border border-[#c7c7c7] px-3 py-1 hover:bg-[#f2f2f2] disabled:opacity-50 dark:border-[#4d4d4d] dark:hover:bg-[#2a2a2a]"
        title="Drop a PNG/JPEG org diagram — glm-5.3-flash extracts the chart"
      >
        Import diagram
      </button>
      <button
        onClick={importCluster}
        disabled={!!busy}
        className="rounded border border-[#c7c7c7] px-3 py-1 hover:bg-[#f2f2f2] disabled:opacity-50 dark:border-[#4d4d4d] dark:hover:bg-[#2a2a2a]"
        title="Read-only import of live Kueue resources"
      >
        Import cluster
      </button>
      {busy && <span className="text-xs text-[#0066cc]">{busy}</span>}
      {error && <span className="max-w-[24rem] truncate text-xs text-[#be0000]">{error}</span>}
      {problems.length > 0 && (
        <span className="text-xs text-[#8a6d00]">
          {problems.filter((p) => p.severity === "error").length} errors, {problems.filter((p) => p.severity === "warning").length} warnings — check the validation pane
        </span>
      )}
    </div>
  );
}
