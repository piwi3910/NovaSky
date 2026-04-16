/**
 * NovaSky Design System — Mantine Theme Configuration
 * Generated from .ui-design/design-system.json
 *
 * Usage: import { novaskyTheme } from '.ui-design/tokens/theme';
 *        <MantineProvider theme={novaskyTheme}>
 */
import { createTheme, MantineColorsTuple } from "@mantine/core";

// Primary: Indigo #6366F1
const primary: MantineColorsTuple = [
  "#EEF2FF", // 50
  "#E0E7FF", // 100
  "#C7D2FE", // 200
  "#A5B4FC", // 300
  "#818CF8", // 400
  "#6366F1", // 500 — brand primary
  "#4F46E5", // 600
  "#4338CA", // 700
  "#3730A3", // 800
  "#312E81", // 900
];

// Secondary: Teal #14B8A6
const secondary: MantineColorsTuple = [
  "#F0FDFA",
  "#CCFBF1",
  "#99F6E4",
  "#5EEAD4",
  "#2DD4BF",
  "#14B8A6",
  "#0D9488",
  "#0F766E",
  "#115E59",
  "#134E4A",
];

// Neutral: Slate
const neutral: MantineColorsTuple = [
  "#F8FAFC",
  "#F1F5F9",
  "#E2E8F0",
  "#CBD5E1",
  "#94A3B8",
  "#64748B",
  "#475569",
  "#334155",
  "#1E293B",
  "#0F172A",
];

export const novaskyTheme = createTheme({
  primaryColor: "indigo",
  defaultRadius: "sm",

  colors: {
    indigo: primary,
    teal: secondary,
    slate: neutral,
  },

  fontFamily: "Inter, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
  fontFamilyMonospace: "ui-monospace, 'JetBrains Mono', 'Fira Code', monospace",

  headings: {
    fontFamily: "Inter, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
    fontWeight: "600",
    sizes: {
      h1: { fontSize: "2.25rem", lineHeight: "1.25" },
      h2: { fontSize: "1.5rem", lineHeight: "1.3" },
      h3: { fontSize: "1.25rem", lineHeight: "1.4" },
      h4: { fontSize: "1.125rem", lineHeight: "1.4" },
    },
  },

  spacing: {
    xs: "0.25rem",
    sm: "0.5rem",
    md: "1rem",
    lg: "1.5rem",
    xl: "2rem",
  },

  radius: {
    xs: "2px",
    sm: "4px",
    md: "6px",
    lg: "8px",
    xl: "12px",
  },

  shadows: {
    xs: "0 1px 2px 0 rgb(0 0 0 / 0.05)",
    sm: "0 1px 3px 0 rgb(0 0 0 / 0.1), 0 1px 2px -1px rgb(0 0 0 / 0.1)",
    md: "0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1)",
    lg: "0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1)",
    xl: "0 20px 25px -5px rgb(0 0 0 / 0.1), 0 8px 10px -6px rgb(0 0 0 / 0.1)",
  },

  breakpoints: {
    xs: "576px",
    sm: "768px",
    md: "992px",
    lg: "1200px",
    xl: "1408px",
  },

  other: {
    // Observatory-specific semantic tokens
    colors: {
      safe: "#22C55E",
      unsafe: "#EF4444",
      unknown: "#F59E0B",
      day: "#FBBF24",
      night: "#6366F1",
      twilight: "#A78BFA",
      starOverlay: "#00FF64",
      constellationLine: "#6496FF",
      planetLabel: "#FFC832",
      cloudCover: "#94A3B8",
    },
    animation: {
      fast: "100ms",
      normal: "200ms",
      slow: "400ms",
      ease: "cubic-bezier(0.4, 0, 0.2, 1)",
    },
  },
});

// Export semantic color accessors for use in components
export const observatory = {
  safe: "#22C55E",
  unsafe: "#EF4444",
  unknown: "#F59E0B",
  day: "#FBBF24",
  night: "#6366F1",
  twilight: "#A78BFA",
  starOverlay: "rgba(0, 255, 100, 0.6)",
  constellationLine: "rgba(100, 150, 255, 0.5)",
  planetLabel: "rgba(255, 200, 50, 0.9)",
} as const;
