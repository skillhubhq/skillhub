import { useEffect, useState, useSyncExternalStore } from "react";
import { API, isWeb } from "./platform";
export type Theme = "system" | "light" | "dark";
const key = "skillhub.theme";
let theme: Theme = "system";
try {
  const v = localStorage.getItem(key);
  if (v === "light" || v === "dark") theme = v;
} catch {}
const media = window.matchMedia("(prefers-color-scheme: dark)");
const listeners = new Set<() => void>();
function apply() {
  document.documentElement.dataset.theme =
    theme === "system" ? (media.matches ? "dark" : "light") : theme;
  document.documentElement.style.colorScheme =
    document.documentElement.dataset.theme!;
  listeners.forEach((fn) => fn());
}
export function setTheme(value: Theme) {
  theme = value;
  try {
    localStorage.setItem(key, value);
  } catch {}
  apply();
}
export function useTheme() {
  return useSyncExternalStore(
    (fn) => {
      listeners.add(fn);
      return () => {
        listeners.delete(fn);
      };
    },
    () => theme,
  );
}
media.addEventListener("change", apply);
apply();

window.addEventListener("storage", (event) => {
  if (event.key === key) {
    theme =
      event.newValue === "light" || event.newValue === "dark"
        ? event.newValue
        : "system";
    apply();
  }
});

export function useNativeAppearance() {
  const theme = useTheme();
  const dark = useSyncExternalStore(
    (fn) => {
      listeners.add(fn);
      return () => {
        listeners.delete(fn);
      };
    },
    () => document.documentElement.dataset.theme === "dark",
  );
  const [error, setError] = useState("");
  useEffect(() => {
    if (isWeb) return;
    let current = true;
    void API.ApplyAppearance(theme, dark)
      .then(() => {
        if (current) setError("");
      })
      .catch(() => {
        if (current) setError("Window appearance could not be updated.");
      });
    return () => {
      current = false;
    };
  }, [theme, dark]);
  return error;
}
