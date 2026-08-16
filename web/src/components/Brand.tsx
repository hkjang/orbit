import { Box, Typography } from "@mui/material";

export function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1.2 }}>
      <Box
        component="svg"
        viewBox="0 0 128 128"
        aria-hidden="true"
        sx={{
          width: 36,
          height: 36,
          flexShrink: 0,
          filter: "drop-shadow(0 0 14px rgba(162,140,255,.45))",
          borderRadius: "50%",
        }}
      >
        <defs>
          <radialGradient id="brandBg" cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor="#151a38" />
            <stop offset="85%" stopColor="#090b18" />
            <stop offset="100%" stopColor="#060710" />
          </radialGradient>
          <radialGradient id="brandSun" cx="35%" cy="35%" r="65%">
            <stop offset="0%" stopColor="#fff4cc" />
            <stop offset="45%" stopColor="#f6c96b" />
            <stop offset="100%" stopColor="#e09b2d" />
          </radialGradient>
          <radialGradient id="brandPurple" cx="35%" cy="35%" r="65%">
            <stop offset="0%" stopColor="#e4dcff" />
            <stop offset="60%" stopColor="#a28cff" />
            <stop offset="100%" stopColor="#6d4aff" />
          </radialGradient>
          <radialGradient id="brandGreen" cx="35%" cy="35%" r="65%">
            <stop offset="0%" stopColor="#a7f3d0" />
            <stop offset="60%" stopColor="#34d399" />
            <stop offset="100%" stopColor="#059669" />
          </radialGradient>
          <linearGradient id="brandRing" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#a28cff" stopOpacity="0.85" />
            <stop offset="50%" stopColor="#6366f1" stopOpacity="0.3" />
            <stop offset="100%" stopColor="#f6c96b" stopOpacity="0.75" />
          </linearGradient>
        </defs>
        <circle cx="64" cy="64" r="60" fill="url(#brandBg)" stroke="#a28cff" strokeWidth="2.5" strokeOpacity="0.45" />
        <circle cx="64" cy="64" r="48" stroke="#6366f1" strokeWidth="1.2" strokeOpacity="0.25" strokeDasharray="6 4" />
        <ellipse cx="64" cy="64" rx="42" ry="20" transform="rotate(-28 64 64)" stroke="url(#brandRing)" strokeWidth="2.2" />
        <ellipse cx="64" cy="64" rx="36" ry="16" transform="rotate(42 64 64)" stroke="#a28cff" strokeWidth="1" strokeOpacity="0.35" strokeDasharray="3 4" />
        <circle cx="64" cy="64" r="13" fill="url(#brandSun)" />
        <g transform="rotate(-28 64 64)">
          <circle cx="106" cy="64" r="7" fill="url(#brandPurple)" />
          <circle cx="106" cy="64" r="9" stroke="#a28cff" strokeWidth="1" strokeOpacity="0.6" fill="none" />
          <circle cx="22" cy="64" r="5" fill="url(#brandGreen)" />
          <circle cx="22" cy="64" r="7" stroke="#34d399" strokeWidth="0.8" strokeOpacity="0.5" fill="none" />
        </g>
      </Box>
      {!compact && (
        <Box>
          <Typography
            sx={{
              fontWeight: 800,
              fontSize: "1.15rem",
              lineHeight: 1,
              letterSpacing: ".02em",
            }}
          >
            ORBIT
          </Typography>
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ letterSpacing: ".12em" }}
          >
            RELATIONSHIPS
          </Typography>
        </Box>
      )}
    </Box>
  );
}
