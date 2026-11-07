import { useSyncExternalStore } from "react";
import zh from "./locales/zh.json";
export type Language = "en" | "zh";
const key = "skillhub.language";
let language: Language = "en";
try {
  if (localStorage.getItem(key) === "zh") language = "zh";
} catch {}
const listeners = new Set<() => void>();
export function setLanguage(value: Language) {
  language = value === "zh" ? "zh" : "en";
  try {
    localStorage.setItem(key, language);
    document.cookie = `skillhub-language=${language}; Path=/; SameSite=Strict`;
  } catch {}
  document.documentElement.lang = language === "zh" ? "zh-CN" : "en";
  listeners.forEach((listener) => listener());
}
export function useLanguage() {
  return useSyncExternalStore(
    (listener) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    () => language,
  );
}
export function t(english: string, chinese?: string): string {
  return language === "zh"
    ? (chinese ?? (zh as Record<string, string>)[english] ?? english)
    : english;
}
export function dateLabel(value: string) {
  return new Date(value).toLocaleDateString(
    language === "zh" ? "zh-CN" : "en",
    { month: "short", day: "numeric", year: "numeric" },
  );
}
document.documentElement.lang = language === "zh" ? "zh-CN" : "en";

window.addEventListener("storage", (event) => {
  if (event.key === key) setLanguage(event.newValue === "zh" ? "zh" : "en");
});
