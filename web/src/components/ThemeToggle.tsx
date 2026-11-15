// Header segmented control for Light / Dark / Auto theme.
import type { Theme } from "../lib/theme";

const OPTIONS: { value: Theme; label: string; title: string }[] = [
  { value: "light", label: "Light", title: "Light theme" },
  { value: "dark", label: "Dark", title: "Dark theme" },
  { value: "system", label: "Auto", title: "Follow the OS setting" },
];

export function ThemeToggle({ value, onChange }: { value: Theme; onChange: (t: Theme) => void }) {
  return (
    <div
      className="flex overflow-hidden rounded border border-[#c7c7c7] text-xs dark:border-[#4d4d4d]"
      role="group"
      aria-label="Theme"
    >
      {OPTIONS.map((o) => (
        <button
          key={o.value}
          onClick={() => onChange(o.value)}
          title={o.title}
          aria-pressed={value === o.value}
          className={`px-2 py-1 ${
            value === o.value
              ? "bg-[#151515] text-white dark:bg-[#f0f0f0] dark:text-[#151515]"
              : "hover:bg-[#f2f2f2] dark:hover:bg-[#2a2a2a]"
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
