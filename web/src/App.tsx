// App shell: header (project, page tabs, version selector, imports), React Flow
// canvas for the org chart, YAML drawer, and the validation pane.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  type ReactFlowInstance,
  type Edge,
  type Node as RFNode,
  type NodeChange,
  type EdgeChange,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { api } from "./api";
import type { Node as OrgNode, OrgChart, Problem, Project, Page, VersionInfo, Preset, GPUProfile } from "./types";
import { applyDefaultGPU, defaultChart, emptyChart, findNode, removeNode, reparent, walk } from "./lib/chart";
import { OrgNodeCard } from "./components/OrgNodeCard";
import { YamlDrawer } from "./components/YamlDrawer";
import { ValidationPane } from "./components/ValidationPane";
import { ImportMenu } from "./components/ImportMenu";
import { DefaultGPUControl } from "./components/DefaultGPUControl";
import { ProjectsMenu } from "./components/ProjectsMenu";
import { HelpPage } from "./components/HelpPage";
import { ThemeToggle } from "./components/ThemeToggle";
import { useTheme } from "./lib/theme";

export default function App() {
  const [versions, setVersions] = useState<VersionInfo[]>([]);
  const [presets, setPresets] = useState<Preset[]>([]);
  const [gpuProfiles, setGpuProfiles] = useState<GPUProfile[]>([]);
  const [paneTab, setPaneTab] = useState<"node" | "problems">("problems");
  const [projects, setProjects] = useState<Project[]>([]);
  const [pages, setPages] = useState<Page[]>([]);
  const [activePage, setActivePage] = useState<Page | null>(null);
  const [chart, setChart] = useState<OrgChart>(emptyChart("1.5"));
  const [problems, setProblems] = useState<Problem[]>([]);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);
  // light/dark/auto theme; resolved feeds both the <html> class and React Flow
  const { theme, resolved: themeResolved, setTheme } = useTheme();
  // page-level default GPU profile: applied to nodes lacking GPU settings
  // (diagram import, new teams). Cluster imports never receive it.
  const defaultGPU = chart.defaults ?? { resource: "nvidia.com/gpu", count: 8, flavor: "h200" };
  const saveTimer = useRef<number | null>(null);
  const emitTimer = useRef<number | null>(null);
  const emitSeq = useRef(0); // only the newest emit response may land (stale ones race)
  const rfRef = useRef<ReactFlowInstance | null>(null);
  const [fitSignal, setFitSignal] = useState(0); // bump → recenter the canvas
  // measured card sizes per node id: RF reports them as "dimensions" changes;
  // we intentionally ignore most changes, but the MiniMap only draws nodes
  // whose user node object carries dimensions — so keep them and re-attach.
  const [measured, setMeasured] = useState<Record<string, { width: number; height: number }>>({});



  const loadPage = useCallback((page: Page) => {
    setActivePage(page);
    setFitSignal((v) => v + 1);
    setHelpOpen(false); // page actions are visible again
    const ver = page.version || "1.5";
    try {
      const parsed = JSON.parse(page.state) as OrgChart;
      if (parsed?.root) {
        setChart({ ...parsed, version: parsed.version || ver });
      } else {
        setChart(defaultChart(ver));
      }
    } catch {
      setChart(defaultChart(ver));
    }
  }, []);

  const openProject = useCallback((project: Project) => {
    api.pages(project.id).then((ps) => {
      setPages(ps);
      if (ps.length > 0) loadPage(ps[0]);
      else
        api
          .createPage(project.id, { name: "main", position: 0, version: "1.5", state: JSON.stringify(defaultChart("1.5")) })
          .then((p) => {
            setPages([p]);
            loadPage(p);
          });
    });
  }, [loadPage]);

  const createProject = useCallback(
    (name: string) => {
      api.createProject(name).then((p) => {
        setProjects((prev) => [p, ...prev]);
        openProject(p);
      });
    },
    [openProject],
  );

  const mutateChart = useCallback(
    (next: OrgChart) => {
      setChart({ ...next, version: activePage?.version ?? chart.version });
      if (saveTimer.current) window.clearTimeout(saveTimer.current);
      if (activePage) {
        saveTimer.current = window.setTimeout(() => {
          api
            .savePage(activePage.id, {
              state: JSON.stringify({ ...next, version: activePage.version }),
              version: activePage.version,
            })
            .catch(() => {});
        }, 600);
      }
    },
    [activePage, chart.version],
  );

  // live YAML emission (debounced)
  const [yaml, setYaml] = useState("");
  useEffect(() => {
    if (emitTimer.current) window.clearTimeout(emitTimer.current);
    emitTimer.current = window.setTimeout(() => {
      if (!chart.version || !/^(1\.4|1\.5)$/.test(chart.version)) return;
      const seq = ++emitSeq.current;
      api
        .emit(chart.version, chart)
        .then((res) => {
          if (seq !== emitSeq.current) return; // a newer emit superseded this one
          setYaml(res.yaml);
          setProblems(res.problems ?? []);
        })
        .catch(() => {});
    }, 400);
    return () => {
      if (emitTimer.current) window.clearTimeout(emitTimer.current);
    };
  }, [chart]);

  useEffect(() => {
    api.versions().then(setVersions).catch(() => {});
    api.presets().then(setPresets).catch(() => {});
    api.gpuProfiles().then(setGpuProfiles).catch(() => {});
    api.projects().then((ps) => {
      setProjects(ps);
      if (ps.length > 0) {
        openProject(ps[0]);
      } else {
        // first run: seed a default project + page with the slides-style org
        api
          .createProject("default")
          .then((p) => {
            setProjects([p]);
            return api.createPage(p.id, {
              name: "main",
              position: 0,
              version: "1.5",
              state: JSON.stringify(defaultChart("1.5")),
            });
          })
          .then((page) => {
            if (page) {
              setPages([page]);
              loadPage(page);
            }
          })
          .catch(() => {});
      }
    }).catch(() => {});
  }, [openProject, loadPage]);

  // recenter when a new chart arrives (imports, page switches)
  useEffect(() => {
    if (fitSignal === 0) return;
    const t = window.setTimeout(() => {
      rfRef.current?.fitView({ padding: 0.15, duration: 500, maxZoom: 1.5 });
    }, 150);
    return () => window.clearTimeout(t);
  }, [fitSignal]);

  // React Flow layout: simple tree layout, roots left→right by depth
  const { rfNodes, rfEdges } = useMemo(() => {
    const nodes: RFNode[] = [];
    const edges: Edge[] = [];
    const levelW = 300;
    const levelH = 170;
    let leafCursor = 0;
    const place = (n: OrgNode, depth: number): { top: number; height: number } => {
      const children = n.children ?? [];
      let top: number;
      let height: number;
      if (children.length === 0) {
        top = leafCursor * levelH;
        leafCursor += 1;
        height = levelH;
      } else {
        const boxes = children.map((c) => place(c, depth + 1));
        top = Math.min(...boxes.map((b) => b.top));
        const bottom = Math.max(...boxes.map((b) => b.top + b.height));
        height = bottom - top;
        top = top + height / 2 - levelH / 2;
      }
      nodes.push({
        id: n.name,
        type: "org",
        position: { x: depth * levelW, y: Math.max(0, top) },
        data: { org: n },
        dragHandle: ".drag-handle",
        // re-attach the DOM-measured size so the MiniMap draws this node
        // (RF only renders minimap rects for nodes carrying dimensions)
        measured: measured[n.name],
      });
      for (const c of children) {
        edges.push({
          id: `${n.name}-${c.name}`,
          source: n.name,
          target: c.name,
          animated: false,
          style: { stroke: "#4d4d4d" },
        });
      }
      return { top, height };
    };
    if (chart.root) place(chart.root, 0);
    return { rfNodes: nodes, rfEdges: edges };
  }, [chart, measured]);

  const nodeTypes = useMemo(() => ({ org: OrgNodeCard }), []);

  const onNodesChange = useCallback(
    (changes: NodeChange[]) => {
      const dims: Record<string, { width: number; height: number }> = {};
      for (const ch of changes) {
        if (ch.type === "dimensions" && ch.dimensions?.width && ch.dimensions?.height) {
          dims[ch.id] = { width: ch.dimensions.width, height: ch.dimensions.height };
          continue;
        }
        if (ch.type === "remove") {
          const next = structuredClone(chart);
          if (next.root && next.root.name === ch.id) continue; // don't delete root via canvas
          const clone = defaultChart(next.version);
          // manual removal preserving everything else
          const removeRec = (n: OrgNode): boolean => {
            if (!n.children) return false;
            const i = n.children.findIndex((c) => c.name === ch.id);
            if (i >= 0) {
              n.children.splice(i, 1);
              return true;
            }
            return n.children.some(removeRec);
          };
          if (next.root) removeRec(next.root);
          void clone;
          mutateChart(next);
        }
      }
      if (Object.keys(dims).length > 0) setMeasured((m) => ({ ...m, ...dims }));
    },
    [chart, mutateChart],
  );

  const onEdgesChange = useCallback(
    (changes: EdgeChange[]) => {
      for (const ch of changes) {
        if (ch.type === "remove" && chart.root) {
          // dropping an edge = re-parent the target under the root
          const target = (ch as { target?: string }).target ?? "";
          const next = structuredClone(chart);
          if (target && next.root && reparent(next.root, target, next.root.name)) {
            mutateChart(next);
          }
        }
      }
    },
    [chart, mutateChart],
  );

  const onConnect = useCallback(
    (conn: { source: string; target: string }) => {
      if (!chart.root) return;
      const next = structuredClone(chart);
      if (next.root && reparent(next.root, conn.target, conn.source)) mutateChart(next);
    },
    [chart, mutateChart],
  );

  const updateNode = useCallback(
    (name: string, patch: Partial<OrgNode>) => {
      const next = structuredClone(chart);
      if (!next.root) return;
      walk(next.root, (n) => {
        if (n.name === name) Object.assign(n, patch);
      });
      mutateChart(next);
    },
    [chart, mutateChart],
  );

  const addChild = useCallback(
    (parent: string) => {
      const next = structuredClone(chart);
      if (!next.root) return;
      walk(next.root, (n) => {
        if (n.name === parent) {
          n.children = n.children ?? [];
          const fresh: OrgNode = {
            name: `team-${n.children.length + 1}-${Date.now().toString(36).slice(-3)}`,
            kind: "team",
            mode: "pool",
            preset: "fair-share",
            namespaces: [],
          };
          n.children.push(fresh);
        }
      });
      mutateChart(next);
    },
    [chart, mutateChart],
  );

  const setDefaultGPU = useCallback(
    (resource: string, count: number, flavor: string) => {
      const next = structuredClone(chart);
      next.defaults = { resource, count, flavor };
      setChart(next);
      if (activePage) {
        api
          .savePage(activePage.id, {
            state: JSON.stringify(next),
            version: activePage.version,
          })
          .catch(() => {});
      }
    },
    [chart, activePage],
  );

  const deleteNode = useCallback(
    (name: string) => {
      const next = structuredClone(chart);
      if (!next.root || next.root.name === name) return; // never delete the root here
      if (removeNode(next.root, name)) {
        setSelectedName("");
        mutateChart(next);
      }
    },
    [chart, mutateChart],
  );

  const changeVersion = useCallback(
    (version: string) => {
      const next = { ...structuredClone(chart), version };
      setChart(next);
      if (activePage) {
        api.savePage(activePage.id, { version, state: JSON.stringify(next) }).catch(() => {});
      }
    },
    [chart, activePage],
  );

  const deletePage = useCallback(
    (id: number) => {
      if (!activePage) return;
      const wasActive = activePage.id === id;
      const projectId = activePage.projectId;
      api.deletePage(id).then(() => {
        api.pages(projectId).then((ps) => {
          setPages(ps);
          if (wasActive && ps.length > 0) loadPage(ps[0]);
        });
      });
    },
    [activePage, loadPage],
  );

  const addPage = useCallback(() => {
    if (!activePage) return;
    api
      .createPage(activePage.projectId, {
        name: `scenario-${pages.length + 1}`,
        position: pages.length,
        version: activePage.version,
        state: JSON.stringify(emptyChart(activePage.version)),
      })
      .then((p) => {
        setPages((prev) => [...prev, p]);
        loadPage(p);
      });
  }, [activePage, pages.length, loadPage]);

  const [selectedName, setSelectedName] = useState<string>("");
  const selectedNode = useMemo(() => findNode(chart.root, selectedName), [chart, selectedName]);

  // resources already used in the chart (for custom profile entries)
  const chartResources = useMemo(() => {
    const set = new Set<string>();
    if (chart.root) walk(chart.root, (n) => Object.keys(n.quota ?? {}).forEach((r) => set.add(r)));
    return [...set].sort();
  }, [chart]);

  return (
    <div className="flex h-screen flex-col bg-[#f5f5f5] text-[#151515] dark:bg-[#151515] dark:text-[#f0f0f0]">
      <header className="flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-[#e0e0e0] bg-white px-4 py-2 dark:border-[#383838] dark:bg-[#1c1c1c]">
        <div className="flex items-baseline gap-2">
          <span className="text-lg font-bold tracking-tight text-[#ee0000]">cohort-builder</span>
          <span className="hidden text-xs text-[#6a6e73] 2xl:inline">GPUaaS quota planning with Kueue + RHOAI</span>
        </div>
        <div className="mx-2 h-6 w-px bg-[#e0e0e0] dark:bg-[#383838]" />
        <ProjectsMenu
          projects={projects}
          active={activePage?.projectId ?? null}
          onOpen={openProject}
          onCreate={createProject}
        />
        <div className="flex items-center gap-1">
          {pages.map((p) => (
            <span key={p.id} className="flex items-center">
              <button
                onClick={() => loadPage(p)}
                className={`rounded-l px-2 py-1 text-sm ${activePage?.id === p.id ? "bg-[#151515] text-white dark:bg-[#f0f0f0] dark:text-[#151515]" : "hover:bg-[#f2f2f2] dark:hover:bg-[#2a2a2a]"}`}
              >
                {p.name}
              </button>
              <button
                onClick={() => {
                  if (window.confirm(`Delete page "${p.name}"?`)) deletePage(p.id);
                }}
                title={`Delete page ${p.name}`}
                className={`rounded-r px-1.5 py-1 text-xs ${activePage?.id === p.id ? "bg-[#151515] text-[#ff8a8a] dark:bg-[#f0f0f0] dark:text-[#be0000]" : "text-[#6a6e73] hover:bg-[#f2f2f2] hover:text-[#be0000] dark:hover:bg-[#2a2a2a]"}`}
              >
                ×
              </button>
            </span>
          ))}
          {activePage && (
            <button onClick={addPage} className="rounded px-2 py-1 text-sm text-[#0066cc] hover:underline" title="New scenario page">
              + page
            </button>
          )}
        </div>
        <div className="mx-2 h-6 w-px bg-[#e0e0e0] dark:bg-[#383838]" />
        <select
          value={chart.version}
          onChange={(e) => changeVersion(e.target.value)}
          className="rounded border border-[#c7c7c7] bg-white px-2 py-1 text-sm dark:border-[#4d4d4d] dark:bg-[#2a2a2a]"
          title="kueue-operator release branch"
        >
          {versions.map((v) => (
            <option key={v.key} value={v.key}>
              {v.key} · {v.operand}
            </option>
          ))}
        </select>
        <div className="flex-1" />
        <DefaultGPUControl
          value={defaultGPU}
          profiles={gpuProfiles}
          onChange={setDefaultGPU}
        />
        <ImportMenu
          version={chart.version}
          onVersionChange={changeVersion}
          onImported={(c, v) => {
            const next = { ...structuredClone(c), version: v ?? chart.version };
            // saner defaults for diagram imports (the "normal GPUaaS route"):
            // hardware pool on the top cohort, teams borrow with fair sharing.
            // Cluster imports never reach this — real cluster data wins.
            applyDefaultGPU(next, defaultGPU);
            if (v && v !== chart.version) {
              setChart(next);
              changeVersion(v);
            } else {
              mutateChart(next);
            }
            setFitSignal((x) => x + 1);
            setHelpOpen(false); // show the result
          }}
        />
        <ThemeToggle value={theme} onChange={setTheme} />
        <button
          onClick={() => setDrawerOpen(!drawerOpen)}
          className="rounded border border-[#c7c7c7] px-3 py-1 text-sm hover:bg-[#f2f2f2] dark:border-[#4d4d4d] dark:hover:bg-[#2a2a2a]"
        >
          {drawerOpen ? "Hide YAML" : "YAML"}
        </button>
        <button
          onClick={() => setHelpOpen(!helpOpen)}
          className={`rounded border px-3 py-1 text-sm ${
            helpOpen
              ? "border-[#151515] bg-[#151515] text-white dark:border-[#f0f0f0] dark:bg-[#f0f0f0] dark:text-[#151515]"
              : "border-[#c7c7c7] hover:bg-[#f2f2f2] dark:border-[#4d4d4d] dark:hover:bg-[#2a2a2a]"
          }`}
          title="How to use cohort-builder"
        >
          Help
        </button>
      </header>

      {helpOpen ? (
        <HelpPage onClose={() => setHelpOpen(false)} />
      ) : (
        <div className="flex min-h-0 flex-1">
          <div className="relative min-w-0 flex-1">
          <ReactFlow
            nodes={rfNodes}
            edges={rfEdges}
            nodeTypes={nodeTypes}
            nodeClickDistance={2}
            // RF12 canvas chrome follows the theme toggle's resolved mode —
            // its dark variable set (minimap, controls, background dots) only
            // applies when colorMode selects it
            colorMode={themeResolved}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            onInit={(inst) => {
              rfRef.current = inst;
            }}
            onNodeClick={(_, n) => {
              setSelectedName(n.id);
              setPaneTab("node");
            }}
            fitView
          >
            <Background gap={24} />
            <Controls />
            {/* mini diagram: one rectangle per node, cohorts blue like their
                cards, teams neutral; the lighter rectangle is the viewport */}
            <MiniMap
              pannable
              zoomable
              aria-label="Org chart minimap"
              nodeColor={(n) => {
                const org = (n.data as { org?: OrgNode })?.org;
                if (org && org.kind !== "team") return "#0066cc"; // cohort
                return "#8a8d90"; // team (neutral, like the cards)
              }}
              style={{ borderRadius: 8 }}
            />
          </ReactFlow>
        </div>

        {drawerOpen && (
          <YamlDrawer
            yaml={yaml}
            version={chart.version}
            onApply={(parsed) => mutateChart(parsed)}
          />
        )}

        <ValidationPane
          problems={problems}
          selectedName={selectedName}
          selectedNode={selectedNode}
          presets={presets}
          gpuProfiles={gpuProfiles}
          chartResources={chartResources}
          tab={paneTab}
          setTab={setPaneTab}
          onUpdateNode={updateNode}
          onAddChild={addChild}
          onDeleteNode={deleteNode}
        />
        </div>
      )}
    </div>
  );
}
