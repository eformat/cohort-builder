// Theme state: Light / Dark / Auto (OS scheme). The choice persists in
// localStorage; the resolved mode is applied as the `.dark` class on <html>
// (Tailwind's dark: variant is class-based — see index.css) and feeds React
// Flow's colorMode so canvas chrome follows the same source of truth.
import { useCallback, useEffect, useState } from "react";

export type Theme = "light" | "dark" | "system";
export type ResolvedTheme = "light" | "dark";

const KEY = "cohort-theme";

function stored(): Theme {
  try {
    const t = localStorage.getItem(KEY);
    return t === "light" || t === "dark" || t === "system" ? t : "system";
  } catch {
    return "system";
  }
}

function systemDark(): boolean {
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

function resolve(theme: Theme): ResolvedTheme {
  if (theme !== "system") return theme;
  return systemDark() ? "dark" : "light";
}

export function useTheme() {
  const [theme, setThemeState] = useState<Theme>(stored);
  const [resolved, setResolved] = useState<ResolvedTheme>(() => resolve(theme));

  // apply on change; while in Auto, follow OS scheme changes live
  useEffect(() => {
    document.documentElement.classList.toggle("dark", resolve(theme) === "dark");
    setResolved(resolve(theme));
    if (theme !== "system") return;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => {
      document.documentElement.classList.toggle("dark", mq.matches);
      setResolved(mq.matches ? "dark" : "light");
    };
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, [theme]);

  const setTheme = useCallback((t: Theme) => {
    setThemeState(t);
    try {
      localStorage.setItem(KEY, t);
    } catch {
      // private mode etc. — theme still applies for this session
    }
  }, []);

  return { theme, resolved, setTheme };
}
