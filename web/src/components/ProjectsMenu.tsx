// Project switcher + creation (SQLite-backed).
import { useState } from "react";
import type { Project } from "../types";

export function ProjectsMenu({
  projects,
  active,
  onOpen,
  onCreate,
}: {
  projects: Project[];
  active: number | null;
  onOpen: (p: Project) => void;
  onCreate: (name: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const current = projects.find((p) => p.id === active);

  return (
    <div className="relative text-sm">
      <button
        onClick={() => setOpen(!open)}
        className="rounded border border-[#c7c7c7] px-3 py-1 hover:bg-[#f2f2f2] dark:border-[#4d4d4d] dark:hover:bg-[#2a2a2a]"
      >
        {current ? `📁 ${current.name}` : "Open project"} ▾
      </button>
      {open && (
        <div className="absolute left-0 top-full z-20 mt-1 w-72 rounded-md border border-[#e0e0e0] bg-white p-2 shadow-lg dark:border-[#383838] dark:bg-[#1c1c1c]">
          <div className="max-h-60 overflow-auto">
            {projects.map((p) => (
              <button
                key={p.id}
                onClick={() => {
                  onOpen(p);
                  setOpen(false);
                }}
                className={`block w-full rounded px-2 py-1 text-left hover:bg-[#f2f2f2] dark:hover:bg-[#2a2a2a] ${
                  p.id === active ? "font-semibold text-[#0066cc]" : ""
                }`}
              >
                {p.name}
              </button>
            ))}
            {projects.length === 0 && <p className="px-2 py-1 text-xs text-[#6a6e73]">No projects yet.</p>}
          </div>
          <div className="mt-2 flex gap-1 border-t border-[#e0e0e0] pt-2 dark:border-[#383838]">
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && name.trim()) {
                  onCreate(name.trim());
                  setName("");
                  setOpen(false);
                }
              }}
              placeholder="new project name"
              className="w-full rounded border border-[#c7c7c7] px-2 py-1 text-xs dark:border-[#4d4d4d] dark:bg-[#2a2a2a]"
            />
            <button
              onClick={() => {
                if (name.trim()) {
                  onCreate(name.trim());
                  setName("");
                  setOpen(false);
                }
              }}
              className="rounded bg-[#0066cc] px-2 py-1 text-xs text-white"
            >
              Create
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
