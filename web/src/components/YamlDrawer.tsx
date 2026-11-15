// YAML drawer: live emitted output + paste-to-load (Direction A).
import { useEffect, useState } from "react";
import CodeMirror from "@uiw/react-codemirror";
import { yaml as yamlLang } from "@codemirror/lang-yaml";
import { oneDark } from "@codemirror/theme-one-dark";
import { api } from "../api";
import type { OrgChart, Problem } from "../types";

export function YamlDrawer({
  yaml,
  version,
  onApply,
}: {
  yaml: string;
  version: string;
  onApply: (chart: OrgChart) => void;
}) {
  const [editing, setEditing] = useState<string | null>(null);
  const [dirty, setDirty] = useState(false); // user edited — stop following live yaml
  const [problems, setProblems] = useState<Problem[]>([]);
  const [error, setError] = useState<string>("");

  useEffect(() => {
    if (!dirty) setEditing(yaml);
  }, [yaml, dirty]);

  const apply = async () => {
    if (editing === null) return;
    setError("");
    try {
      const res = await api.parse(version, editing);
      const errs = (res.problems ?? []).filter((p) => p.severity === "error");
      if (!res.chart) {
        setError(errs[0]?.message ?? "parse failed");
        setProblems(res.problems ?? []);
        return;
      }
      onApply(res.chart);
      setProblems(res.problems ?? []);
    } catch (e) {
      setError(String(e));
    }
  };

  return (
    <aside className="flex w-[46rem] flex-col border-l border-[#e0e0e0] bg-white dark:border-[#383838] dark:bg-[#1c1c1c]">
      <div className="flex items-center gap-2 border-b border-[#e0e0e0] px-3 py-2 dark:border-[#383838]">
        <span className="text-sm font-semibold">Kueue YAML ({version})</span>
        <div className="flex-1" />
        <button
          onClick={() => {
            setEditing(yaml);
            setDirty(false);
          }}
          className="rounded border border-[#c7c7c7] px-2 py-0.5 text-xs hover:bg-[#f2f2f2] dark:border-[#4d4d4d] dark:hover:bg-[#2a2a2a]"
          title="Reload from the emitted output"
        >
          Reset
        </button>
        <button
          onClick={apply}
          className="rounded bg-[#0066cc] px-2 py-0.5 text-xs font-medium text-white hover:bg-[#004d99]"
          title="Parse this YAML back onto the canvas"
        >
          Apply to canvas
        </button>
        <button
          onClick={() => navigator.clipboard.writeText(editing ?? yaml)}
          className="rounded border border-[#c7c7c7] px-2 py-0.5 text-xs hover:bg-[#f2f2f2] dark:border-[#4d4d4d] dark:hover:bg-[#2a2a2a]"
        >
          Copy
        </button>
        <button
          onClick={() => {
            const blob = new Blob([editing ?? yaml], { type: "application/yaml" });
            const a = document.createElement("a");
            a.href = URL.createObjectURL(blob);
            a.download = "cohort-builder.yaml";
            a.click();
          }}
          className="rounded border border-[#c7c7c7] px-2 py-0.5 text-xs hover:bg-[#f2f2f2] dark:border-[#4d4d4d] dark:hover:bg-[#2a2a2a]"
        >
          Download
        </button>
      </div>
      {error && <div className="bg-[#d03b3b]/10 px-3 py-1 text-xs text-[#be0000]">{error}</div>}
      <div className="min-h-0 flex-1 overflow-auto text-xs">
        <CodeMirror
          value={editing ?? yaml}
          extensions={[yamlLang()]}
          theme={oneDark}
          height="100%"
          onChange={(v) => {
            setEditing(v);
            setDirty(true);
          }}
        />
      </div>
    </aside>
  );
}
