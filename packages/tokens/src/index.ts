/**
 * Design tokens shared by the mobile app and the website. Plain values only, so any
 * platform can consume them (React Native styles, CSS custom properties via `toCssVars`).
 */

export type ThemeName = "light" | "dark";

export interface ColorTokens {
  bg: string;
  surface: string;
  surfaceAlt: string;
  ink: string;
  muted: string;
  line: string;
  accent: string;
  accentInk: string;
  accentSoft: string;
  marquee: string;
  marqueeSoft: string;
  danger: string;
  ok: string;
  okSoft: string;
}

export const colors: Record<ThemeName, ColorTokens> = {
  light: {
    bg: "#f6f6f9",
    surface: "#ffffff",
    surfaceAlt: "#eeedf4",
    ink: "#1b1a2e",
    muted: "#5d5b75",
    line: "#dcdbe8",
    accent: "#3b2fb3",
    accentInk: "#ffffff",
    accentSoft: "#ebe9fb",
    marquee: "#b7791f",
    marqueeSoft: "#fbf1df",
    danger: "#b42318",
    ok: "#1f7a4d",
    okSoft: "#e3f3ea",
  },
  dark: {
    bg: "#12111c",
    surface: "#1b1a29",
    surfaceAlt: "#25233a",
    ink: "#ecebf5",
    muted: "#a3a1bb",
    line: "#2f2d44",
    accent: "#a89dff",
    accentInk: "#12111c",
    accentSoft: "#26224a",
    marquee: "#e9b35c",
    marqueeSoft: "#2f2615",
    danger: "#ff8a80",
    ok: "#6fd3a0",
    okSoft: "#173327",
  },
};

/** Category color keys (stored on categories) mapped to a shade per theme. */
export const categoryColors: Record<ThemeName, Record<string, string>> = {
  light: {
    violet: "#6d4fd8",
    blue: "#2563c9",
    teal: "#0f8a8a",
    green: "#2f8a3e",
    amber: "#b7791f",
    orange: "#c4561b",
    red: "#c0362c",
    pink: "#c23a7a",
    slate: "#5b6577",
  },
  dark: {
    violet: "#b9a6ff",
    blue: "#8ab4ff",
    teal: "#6fd6d2",
    green: "#8ed99b",
    amber: "#f0c477",
    orange: "#ffaa77",
    red: "#ff8f86",
    pink: "#ff9cc8",
    slate: "#b3bccb",
  },
};

export const space = { xs: 4, sm: 8, md: 12, lg: 16, xl: 24, xxl: 32 } as const;
export const radius = { sm: 6, md: 10, lg: 14, pill: 999 } as const;

export const type = {
  display: { size: 28, weight: "700", lineHeight: 34 },
  title: { size: 20, weight: "700", lineHeight: 26 },
  heading: { size: 17, weight: "600", lineHeight: 22 },
  body: { size: 15, weight: "400", lineHeight: 22 },
  small: { size: 13, weight: "400", lineHeight: 18 },
  label: { size: 11, weight: "600", lineHeight: 14, letterSpacing: 0.6 },
} as const;

/** Poster aspect ratio used everywhere (2:3, the film poster standard). */
export const POSTER_RATIO = 2 / 3;

/** CSS custom properties for the website, e.g. `--color-accent`. */
export function toCssVars(theme: ThemeName): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(colors[theme])) out[`--color-${k}`] = v;
  for (const [k, v] of Object.entries(categoryColors[theme])) out[`--category-${k}`] = v;
  for (const [k, v] of Object.entries(space)) out[`--space-${k}`] = `${v}px`;
  for (const [k, v] of Object.entries(radius)) out[`--radius-${k}`] = `${v}px`;
  return out;
}
