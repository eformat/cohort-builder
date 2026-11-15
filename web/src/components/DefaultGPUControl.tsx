// Header control for the page-level default GPU profile ("one 8× H200 server").
// Applied to diagram-imported teams and new nodes; never to cluster imports.
import type { Defaults, GPUProfile } from "../types";

export function DefaultGPUControl({
  value,
  profiles,
  onChange,
}: {
  value: Defaults;
  profiles: GPUProfile[];
  onChange: (resource: string, count: number, flavor: string) => void;
}) {
  const resource = value.resource ?? "nvidia.com/gpu";
  const count = value.count ?? 8;

  const onProfile = (key: string) => {
    if (key.startsWith("custom:")) {
      onChange(key.slice(7), count, value.flavor ?? "default-flavor");
      return;
    }
    const p = profiles.find((x) => x.key === key);
    if (p) onChange(p.resource, count, p.flavor);
  };

  const matched = profiles.find((p) => p.resource === resource);
  const customOptions = profiles.length
    ? [] // catalog covers the resources; custom entries arrive via chart usage
    : [];

  return (
    <div className="flex items-center gap-1 text-xs" title="Default GPU profile applied to diagram-imported teams and new nodes (cluster imports keep their real quota)">
      <span className="text-[#6a6e73]">default GPU</span>
      <select
        value={matched ? matched.key : `custom:${resource}`}
        onChange={(e) => onProfile(e.target.value)}
        className="rounded border border-[#c7c7c7] bg-white px-1.5 py-1 text-xs dark:border-[#4d4d4d] dark:bg-[#2a2a2a]"
      >
        {profiles.map((p) => (
          <option key={p.key} value={p.key}>
            {p.label}
          </option>
        ))}
        {!matched && resource && (
          <option value={`custom:${resource}`}>{resource}</option>
        )}
        {customOptions.length > 0 && null}
      </select>
      <span className="text-[#6a6e73]">×</span>
      <input
        type="number"
        min={1}
        value={count}
        onChange={(e) => onChange(resource, Math.max(1, parseInt(e.target.value, 10) || 1), value.flavor ?? matched?.flavor ?? "default-flavor")}
        className="w-14 rounded border border-[#c7c7c7] px-1.5 py-1 text-center font-mono text-xs dark:border-[#4d4d4d] dark:bg-[#2a2a2a]"
      />
    </div>
  );
}
