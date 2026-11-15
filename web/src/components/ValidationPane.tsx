// Right-hand pane: node editor (fully populated from the selected node) +
// validation results. GPU profiles come from the backend catalog (fake-gpu-booking
// shapes) plus resources already present in the chart.
import { useMemo, useState } from "react";
import type { Node as OrgNode, Problem, Preset, QuotaMode } from "../types";
import type { GPUProfile } from "../types";

export type PaneTab = "node" | "problems";

const sevStyle: Record<string, string> = {
  error: "border-[#d03b3b] bg-[#d03b3b]/10 text-[#be0000] dark:text-[#ff8a8a]",
  warning: "border-[#fab219] bg-[#fab219]/10 text-[#8a6d00] dark:text-[#ffdf8a]",
  info: "border-[#8a8d90] bg-[#8a8d90]/10 text-[#4d4d4d] dark:text-[#c7c7c7]",
};

export function ValidationPane({
  problems,
  selectedName,
  selectedNode,
  presets,
  gpuProfiles,
  chartResources,
  tab,
  setTab,
  onUpdateNode,
  onAddChild,
  onDeleteNode,
}: {
  problems: Problem[];
  selectedName: string;
  selectedNode: OrgNode | null;
  presets: Preset[];
  gpuProfiles: GPUProfile[];
  chartResources: string[];
  tab: PaneTab;
  setTab: (t: PaneTab) => void;
  onUpdateNode: (name: string, patch: Partial<OrgNode>) => void;
  onAddChild: (parent: string) => void;
  onDeleteNode: (name: string) => void;
}) {
  const errors = problems.filter((p) => p.severity === "error");
  const warnings = problems.filter((p) => p.severity === "warning");
  const infos = problems.filter((p) => p.severity === "info");

  return (
    <aside className="flex w-[26rem] shrink-0 flex-col border-l border-[#e0e0e0] bg-white dark:border-[#383838] dark:bg-[#1c1c1c]">
      <div className="flex border-b border-[#e0e0e0] dark:border-[#383838]">
        {(["node", "problems"] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`px-3 py-2 text-sm capitalize ${
              tab === t
                ? "border-b-2 border-[#ee0000] font-semibold"
                : "text-[#6a6e73] hover:text-[#151515] dark:hover:text-[#f0f0f0]"
            }`}
          >
            {t === "node" ? "Node" : `Validation (${errors.length})`}
          </button>
        ))}
      </div>

      {tab === "problems" && (
        <div className="min-h-0 flex-1 overflow-auto p-2 text-xs">
          {problems.length === 0 && (
            <p className="p-2 text-[#6a6e73]">No problems — the chart validates clean.</p>
          )}
          {[...errors, ...warnings, ...infos].map((p, i) => (
            <div key={i} className={`mb-1.5 rounded border px-2 py-1.5 ${sevStyle[p.severity]}`}>
              <span className="font-mono text-[10px] opacity-70">{p.path}</span>
              <p className="mt-0.5">{p.message}</p>
            </div>
          ))}
        </div>
      )}

      {tab === "node" && (
        <div className="min-h-0 flex-1 overflow-auto p-3 text-sm">
          {!selectedNode && (
            <p className="text-[#6a6e73]">Click a node on the canvas to edit it.</p>
          )}
          {selectedNode && (
            <NodeForm
              key={selectedNode.name}
              node={selectedNode}
              presets={presets}
              gpuProfiles={gpuProfiles}
              chartResources={chartResources}
              onUpdateNode={onUpdateNode}
              onAddChild={onAddChild}
              onDeleteNode={onDeleteNode}
            />
          )}
        </div>
      )}
    </aside>
  );
}

function NodeForm({
  node,
  presets,
  gpuProfiles,
  chartResources,
  onUpdateNode,
  onAddChild,
  onDeleteNode,
}: {
  node: OrgNode;
  presets: Preset[];
  gpuProfiles: GPUProfile[];
  chartResources: string[];
  onUpdateNode: (name: string, patch: Partial<OrgNode>) => void;
  onAddChild: (parent: string) => void;
  onDeleteNode: (name: string) => void;
}) {
  const [nsText, setNsText] = useState((node.namespaces ?? []).join(", "));
  const [profileKey, setProfileKey] = useState("");
  const [count, setCount] = useState("1");
  const [manualRes, setManualRes] = useState("");
  const [manualVal, setManualVal] = useState("");

  const quota = useMemo(() => node.quota ?? {}, [node]);
  const isRootOnly = node.kind === "org";

  // custom entries: resources already used somewhere in the chart
  const customOptions = chartResources.filter(
    (r) => !gpuProfiles.some((p) => p.resource === r),
  );

  const applyProfile = () => {
    const p = gpuProfiles.find((x) => x.key === profileKey);
    if (!p) return;
    const n = Math.max(1, parseInt(count, 10) || 1);
    onUpdateNode(node.name, {
      quota: { ...quota, [p.resource]: { nominalQuota: String(n) } },
      flavor: p.flavor,
    });
  };

  const applyCustom = () => {
    if (!manualRes) return;
    onUpdateNode(node.name, {
      quota: { ...quota, [manualRes]: { nominalQuota: manualVal || "0" } },
    });
    setManualRes("");
    setManualVal("");
  };

  const setQuotaValue = (res: string, value: string) => {
    onUpdateNode(node.name, {
      quota: { ...quota, [res]: { ...quota[res], nominalQuota: value } },
    });
  };

  const removeQuota = (res: string) => {
    const next = { ...quota };
    delete next[res];
    onUpdateNode(node.name, { quota: next });
  };

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <span className="font-mono text-sm font-semibold">{node.name}</span>
        {node.kind && (
          <span className="rounded bg-[#f2f2f2] px-1.5 text-[10px] uppercase dark:bg-[#2a2a2a]">
            {node.kind}
          </span>
        )}
        <div className="flex-1" />
        {!isRootOnly && (
          <button
            onClick={() => onDeleteNode(node.name)}
            className="rounded border border-[#d03b3b] px-2 py-0.5 text-[10px] text-[#be0000] hover:bg-[#d03b3b]/10"
            title="Remove this node from the chart"
          >
            Delete
          </button>
        )}
      </div>

      {/* Common GPU profiles */}
      <div className="rounded border border-[#e0e0e0] p-2 dark:border-[#383838]">
        <p className="text-xs font-semibold text-[#0066cc]">Common GPU profiles</p>
        <div className="mt-1 flex gap-1">
          <select
            value={profileKey}
            onChange={(e) => setProfileKey(e.target.value)}
            className="w-full rounded border border-[#c7c7c7] bg-white px-2 py-1 text-xs dark:border-[#4d4d4d] dark:bg-[#2a2a2a]"
          >
            <option value="">pick a GPU profile…</option>
            <optgroup label="Catalog (fake-gpu-booking cluster)">
              {gpuProfiles.map((p) => (
                <option key={p.key} value={p.key}>
                  {p.label}
                </option>
              ))}
            </optgroup>
            {customOptions.length > 0 && (
              <optgroup label="Already in this chart">
                {customOptions.map((r) => (
                  <option key={r} value={`custom:${r}`}>
                    {r}
                  </option>
                ))}
              </optgroup>
            )}
          </select>
          <input
            value={count}
            onChange={(e) => setCount(e.target.value)}
            className="w-14 rounded border border-[#c7c7c7] px-2 py-1 text-center font-mono text-xs dark:border-[#4d4d4d] dark:bg-[#2a2a2a]"
            title="count"
          />
          <button
            onClick={applyProfile}
            disabled={!profileKey}
            className="rounded bg-[#0066cc] px-2 py-1 text-xs text-white disabled:opacity-40"
          >
            Apply
          </button>
        </div>
        <p className="mt-1 text-[10px] text-[#6a6e73]">
          Sets the GPU quota on this node and the matching flavor. For the H200 node the
          fake cluster exposes 8 full GPUs / 8+8+16 MIG slices.
        </p>
      </div>

      {/* Quota table */}
      <div>
        <p className="text-xs font-semibold">Quota</p>
        {Object.keys(quota).length === 0 && (
          <p className="mt-1 text-xs text-[#6a6e73]">No quota set — pick a profile above.</p>
        )}
        <div className="mt-1 space-y-1">
          {Object.entries(quota).map(([res, q]) => (
            <div key={res} className="flex items-center gap-1">
              <span className="w-40 truncate font-mono text-[10px]" title={res}>
                {res}
              </span>
              <input
                defaultValue={(q ?? {}).nominalQuota ?? "0"}
                onBlur={(e) => setQuotaValue(res, e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") (e.target as HTMLInputElement).blur();
                }}
                className="w-24 rounded border border-[#c7c7c7] px-2 py-0.5 font-mono text-xs dark:border-[#4d4d4d] dark:bg-[#2a2a2a]"
              />
              <button
                onClick={() => removeQuota(res)}
                className="px-1 text-xs text-[#6a6e73] hover:text-[#be0000]"
                title={`remove ${res}`}
              >
                ×
              </button>
            </div>
          ))}
        </div>
        <div className="mt-1 flex gap-1">
          <input
            value={manualRes}
            onChange={(e) => setManualRes(e.target.value)}
            placeholder="resource (cpu, memory…)"
            className="w-40 rounded border border-[#c7c7c7] px-2 py-0.5 font-mono text-[10px] dark:border-[#4d4d4d] dark:bg-[#2a2a2a]"
          />
          <input
            value={manualVal}
            onChange={(e) => setManualVal(e.target.value)}
            placeholder="qty"
            className="w-20 rounded border border-[#c7c7c7] px-2 py-0.5 font-mono text-[10px] dark:border-[#4d4d4d] dark:bg-[#2a2a2a]"
          />
          <button
            onClick={applyCustom}
            disabled={!manualRes}
            className="rounded border border-[#c7c7c7] px-2 text-[10px] hover:bg-[#f2f2f2] disabled:opacity-40 dark:border-[#4d4d4d] dark:hover:bg-[#2a2a2a]"
          >
            Add
          </button>
        </div>
      </div>

      {/* Scheduling settings */}
      <label className="block text-xs">
        <span className="text-[#6a6e73]">Fair-sharing weight</span>
        <input
          key={`${node.name}-w`}
          defaultValue={node.fairSharingWeight ?? ""}
          onBlur={(e) => onUpdateNode(node.name, { fairSharingWeight: e.target.value || undefined })}
          onKeyDown={(e) => {
            if (e.key === "Enter") (e.target as HTMLInputElement).blur();
          }}
          placeholder="e.g. 3"
          className="mt-1 w-full rounded border border-[#c7c7c7] px-2 py-1 font-mono text-xs dark:border-[#4d4d4d] dark:bg-[#2a2a2a]"
        />
      </label>

      <label className="block text-xs">
        <span className="text-[#6a6e73]">Preemption preset</span>
        <select
          value={node.preset ?? ""}
          onChange={(e) => onUpdateNode(node.name, { preset: e.target.value })}
          className="mt-1 w-full rounded border border-[#c7c7c7] bg-white px-2 py-1 text-xs dark:border-[#4d4d4d] dark:bg-[#2a2a2a]"
        >
          <option value="">custom (API defaults)</option>
          {presets.map((p) => (
            <option key={p.key} value={p.key}>
              {p.label}
            </option>
          ))}
        </select>
      </label>

      <label className="block text-xs">
        <span className="text-[#6a6e73]">Mode</span>
        <select
          value={node.mode ?? "pool"}
          onChange={(e) => onUpdateNode(node.name, { mode: e.target.value as QuotaMode })}
          className="mt-1 w-full rounded border border-[#c7c7c7] bg-white px-2 py-1 text-xs dark:border-[#4d4d4d] dark:bg-[#2a2a2a]"
        >
          <option value="pool">pool (borrow from cohort)</option>
          <option value="guaranteed">guaranteed (own reservation)</option>
        </select>
      </label>

      <label className="block text-xs">
        <span className="text-[#6a6e73]">Flavor</span>
        <input
          key={`${node.name}-f`}
          defaultValue={node.flavor ?? ""}
          onBlur={(e) => onUpdateNode(node.name, { flavor: e.target.value || undefined })}
          onKeyDown={(e) => {
            if (e.key === "Enter") (e.target as HTMLInputElement).blur();
          }}
          placeholder="default-flavor"
          className="mt-1 w-full rounded border border-[#c7c7c7] px-2 py-1 font-mono text-xs dark:border-[#4d4d4d] dark:bg-[#2a2a2a]"
        />
      </label>

      <label className="block text-xs">
        <span className="text-[#6a6e73]">Namespaces (comma-separated, * = all)</span>
        <div className="mt-1 flex gap-1">
          <input
            value={nsText}
            onChange={(e) => setNsText(e.target.value)}
            placeholder="team-lab, team-dev"
            className="w-full rounded border border-[#c7c7c7] px-2 py-1 font-mono text-xs dark:border-[#4d4d4d] dark:bg-[#2a2a2a]"
          />
          <button
            onClick={() => {
              const list = nsText
                .split(",")
                .map((s) => s.trim())
                .filter(Boolean);
              onUpdateNode(node.name, { namespaces: list });
            }}
            className="rounded bg-[#0066cc] px-2 text-xs text-white"
          >
            Set
          </button>
        </div>
      </label>

      <button
        onClick={() => onAddChild(node.name)}
        className="w-full rounded border border-[#c7c7c7] px-2 py-1 text-xs hover:bg-[#f2f2f2] dark:border-[#4d4d4d] dark:hover:bg-[#2a2a2a]"
      >
        + Add child team
      </button>
    </div>
  );
}
