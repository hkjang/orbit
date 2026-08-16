import { createTheme } from "@mui/material/styles";

export const theme = createTheme({
  colorSchemes: {
    dark: {
      palette: {
        primary: { main: "#a99bf8", light: "#c9c0ff", dark: "#7b6bcf" },
        secondary: { main: "#f4c96b" },
        background: { default: "#090b18", paper: "#121528" },
        text: { primary: "#f4f3fb", secondary: "#b7b6c8" },
        divider: "rgba(255,255,255,.09)",
        success: { main: "#77d69b" },
        warning: { main: "#f4c96b" },
      },
    },
    light: {
      palette: {
        primary: { main: "#6654c7" },
        secondary: { main: "#a66a00" },
        background: { default: "#f7f6fb", paper: "#ffffff" },
      },
    },
  },
  typography: {
    fontFamily:
      'Pretendard, "Noto Sans KR", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
    fontSize: 16,
    h1: { fontSize: "2rem", fontWeight: 750, letterSpacing: "-0.035em" },
    h2: { fontSize: "1.55rem", fontWeight: 720, letterSpacing: "-0.025em" },
    h3: { fontSize: "1.25rem", fontWeight: 700 },
    body1: { fontSize: "1rem", lineHeight: 1.65 },
    body2: { fontSize: ".925rem", lineHeight: 1.6 },
    button: { fontSize: ".95rem", fontWeight: 700, textTransform: "none" },
  },
  shape: { borderRadius: 14 },
  components: {
    MuiButton: {
      styleOverrides: { root: { minHeight: 44, borderRadius: 12 } },
    },
    MuiTextField: { defaultProps: { size: "small" } },
    MuiCard: {
      styleOverrides: {
        root: {
          border: "1px solid",
          borderColor: "rgba(255,255,255,.08)",
          backgroundImage: "none",
        },
      },
    },
    MuiCssBaseline: {
      styleOverrides: {
        "html, body, #root": { minHeight: "100%" },
        body: { margin: 0 },
        "*": { scrollbarWidth: "thin", scrollbarColor: "#6f659f transparent" },
        "*::-webkit-scrollbar": { width: 9, height: 9 },
        "*::-webkit-scrollbar-track": { background: "transparent" },
        "*::-webkit-scrollbar-thumb": {
          background: "linear-gradient(#8176bb,#4f496f)",
          borderRadius: 12,
          border: "2px solid transparent",
          backgroundClip: "padding-box",
        },
        "*::-webkit-scrollbar-thumb:hover": {
          background: "#9488d0",
          backgroundClip: "padding-box",
        },
      },
    },
  },
});
