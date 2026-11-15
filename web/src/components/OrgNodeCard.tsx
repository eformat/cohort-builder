import { Handle, Position, type NodeProps } from "@xyflow/react";
import type { Node as OrgNode } from "../types";
import { quotaSummary } from "../lib/chart";

const presetBadge: Record<string, { label: string; color: string }> = {
  "booked-team": { label: "reserved", color: "#0066cc" },
  unreserved: { label: "borrower", color: "#8a8d90" },
  "unreserved-priority": { label: "priority", color: "#ec835a" },
  "fair-share": { label: "fair-share", color: "#7d70af" },
  custom: { label: "custom", color: "#8a8d90" },
};

export function OrgNodeCard({ data }: NodeProps) {
  const org = (data as { org: OrgNode }).org;
  const badge = presetBadge[org.preset ?? ""];
  const quotas = quotaSummary(org);
  const isCohort = (org.children?.length ?? 0) > 0 && !org.synthetic;
  const isGrouping = !!org.synthetic;

  return (
    <div
      className={`w-[260px] rounded-md border bg-white shadow-sm dark:bg-[#1c1c1c] ${
        isCohort ? "border-[#0066cc]" : "border-[#c7c7c7] dark:border-[#4d4d4d]"
      }`}
    >
      {!isCohort && <Handle type="target" position={Position.Left} />}
      {!isCohort && <Handle type="source" position={Position.Right} />}
      {isCohort && (
        <>
          <Handle type="target" position={Position.Left} />
          <Handle type="source" position={Position.Right} />
        </>
      )}
      <div className="drag-handle cursor-grab rounded-t-md border-b border-[#e0e0e0] bg-[#f5f5f5] px-3 py-1.5 dark:border-[#383838] dark:bg-[#262626]">
        <div className="flex items-center gap-2">
          <span className="font-semibold">{org.name}</span>
          {isCohort && (
            <span className="rounded bg-[#0066cc]/10 px-1.5 text-[10px] font-medium uppercase tracking-wide text-[#0066cc]">
              cohort
            </span>
          )}
          {isGrouping && (
            <span
              className="rounded bg-[#8a8d90]/15 px-1.5 text-[10px] font-medium uppercase tracking-wide text-[#6a6e73]"
              title="UI grouping only — not emitted as a Cohort"
            >
              grouping
            </span>
          )}
          {badge && (
            <span
              className="ml-auto rounded px-1.5 text-[10px] font-medium"
              style={{ color: badge.color, background: `${badge.color}1a` }}
            >
              {badge.label}
            </span>
          )}
        </div>
        {org.fairSharingWeight && (
          <span className="text-[10px] text-[#6a6e73]">weight {org.fairSharingWeight}</span>
        )}
      </div>
      <div className="px-3 py-1.5 text-xs">
        {quotas.length > 0 ? (
          <div className="flex flex-wrap gap-1">
            {quotas.map((q) => (
              <span key={q} className="rounded bg-[#f5f5f5] px-1.5 py-0.5 font-mono text-[10px] dark:bg-[#262626]">
                {q}
              </span>
            ))}
          </div>
        ) : (
          <span className="text-[#6a6e73]">{isCohort ? "pool (no own quota)" : "no quota"}</span>
        )}
        <div className="mt-1 flex flex-wrap items-center gap-1 text-[10px] text-[#6a6e73]">
          {org.mode && <span className="rounded bg-[#f2f2f2] px-1 dark:bg-[#2a2a2a]">{org.mode}</span>}
          {org.namespaces && org.namespaces.length > 0 && (
            <span className="font-mono">
              ns: {org.namespaces.length === 1 && org.namespaces[0] === "*" ? "*" : `${org.namespaces.length} ns`}
            </span>
          )}
          {org.flavor && <span className="font-mono">{org.flavor}</span>}
        </div>
      </div>
    </div>
  );
}
