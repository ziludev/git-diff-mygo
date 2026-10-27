import { useEffect, useState } from "react";

export type ThemeType = "light" | "dark";

/** Follows the system appearance, which the MyGo webview reports. */
export function useThemeType(): ThemeType {
  const [theme, setTheme] = useState<ThemeType>(() =>
    window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light",
  );

  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const update = () => setTheme(media.matches ? "dark" : "light");
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);

  useEffect(() => {
    document.documentElement.classList.toggle("dark", theme === "dark");
  }, [theme]);

  return theme;
}
