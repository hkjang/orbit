import { Box, Typography } from "@mui/material";

export function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1.2 }}>
      <Box
        aria-hidden
        sx={{
          width: 35,
          height: 35,
          borderRadius: "50%",
          border: "1.5px solid rgba(169,155,248,.7)",
          position: "relative",
          boxShadow: "0 0 22px rgba(140,120,255,.28)",
          "&::before": {
            content: '""',
            position: "absolute",
            inset: 8,
            borderRadius: "50%",
            background: "linear-gradient(135deg,#f6db8d,#9c87ff)",
            boxShadow: "0 0 18px rgba(246,219,141,.5)",
          },
          "&::after": {
            content: '""',
            position: "absolute",
            width: 5,
            height: 5,
            borderRadius: "50%",
            bgcolor: "#b7a8ff",
            left: -2,
            top: 13,
          },
        }}
      />
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
