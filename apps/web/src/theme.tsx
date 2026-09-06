import * as React from "react";
import { api } from "./api";

export const baseColors = ["neutral", "zinc", "stone", "mauve", "olive", "mist", "taupe"] as const;
export const accentColors = ["zinc", "red", "orange", "amber", "green", "teal", "blue", "violet", "rose"] as const;
export type BaseColor = (typeof baseColors)[number];
export type AccentColor = (typeof accentColors)[number];
export type ColorMode = "light" | "dark" | "system";
export type Radius = "none" | "small" | "medium" | "large";

export interface Appearance {
  baseColor: BaseColor;
  accentColor: AccentColor;
  mode: ColorMode;
  radius: Radius;
}

const defaultAppearance: Appearance = { baseColor: "zinc", accentColor: "zinc", mode: "light", radius: "medium" };

function validAppearance(value: unknown): Appearance {
  const item = typeof value === "object" && value !== null ? value as Partial<Appearance> : {};
  return {
    baseColor: baseColors.includes(item.baseColor as BaseColor) ? item.baseColor as BaseColor : defaultAppearance.baseColor,
    accentColor: accentColors.includes(item.accentColor as AccentColor) ? item.accentColor as AccentColor : defaultAppearance.accentColor,
    mode: ["light", "dark", "system"].includes(item.mode ?? "") ? item.mode as ColorMode : defaultAppearance.mode,
    radius: ["none", "small", "medium", "large"].includes(item.radius ?? "") ? item.radius as Radius : defaultAppearance.radius,
  };
}

function applyToDocument(appearance: Appearance) {
  const root = document.documentElement;
  const dark = appearance.mode === "dark" || (appearance.mode === "system" && matchMedia("(prefers-color-scheme: dark)").matches);
  root.dataset.baseColor = appearance.baseColor;
  root.dataset.accentColor = appearance.accentColor;
  root.dataset.mode = dark ? "dark" : "light";
  root.dataset.radius = appearance.radius;
  root.style.colorScheme = dark ? "dark" : "light";
}

interface ThemeValue {
  appearance: Appearance;
  apply(value: Appearance): void;
  refresh(): Promise<void>;
}

const ThemeContext = React.createContext<ThemeValue | null>(null);

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [appearance, setAppearance] = React.useState(defaultAppearance);
  const apply = React.useCallback((value: Appearance) => {
    const safe = validAppearance(value);
    setAppearance(safe);
    applyToDocument(safe);
  }, []);
  const refresh = React.useCallback(async () => {
    const data = await api.get<{ settings: Record<string, unknown> }>("/settings");
    apply(validAppearance(data.settings.appearance));
  }, [apply]);

  React.useEffect(() => {
    applyToDocument(appearance);
    if (appearance.mode !== "system") return;
    const query = matchMedia("(prefers-color-scheme: dark)");
    const update = () => applyToDocument(appearance);
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, [appearance]);

  return <ThemeContext.Provider value={{ appearance, apply, refresh }}>{children}</ThemeContext.Provider>;
}

export function useTheme() {
  const value = React.useContext(ThemeContext);
  if (!value) throw new Error("useTheme must be used inside ThemeProvider");
  return value;
}
