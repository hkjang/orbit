import { Box, Typography } from "@mui/material";
import type { ReactNode } from "react";

export function PageHeader({
  title,
  description,
  action,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <Box
      sx={{
        display: "flex",
        alignItems: { xs: "flex-start", sm: "center" },
        justifyContent: "space-between",
        gap: 2,
        mb: 3,
        flexDirection: { xs: "column", sm: "row" },
      }}
    >
      <Box>
        <Typography variant="h1">{title}</Typography>
        {description && (
          <Typography color="text.secondary" sx={{ mt: 0.6 }}>
            {description}
          </Typography>
        )}
      </Box>
      {action}
    </Box>
  );
}
