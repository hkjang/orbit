import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Typography,
} from "@mui/material";
import AutoAwesomeRoundedIcon from "@mui/icons-material/AutoAwesomeRounded";

export function LoadingView({
  label = "우주를 불러오는 중…",
}: {
  label?: string;
}) {
  return (
    <Box sx={{ minHeight: 280, display: "grid", placeItems: "center" }}>
      <Box sx={{ textAlign: "center" }}>
        <CircularProgress size={28} />
        <Typography color="text.secondary" sx={{ mt: 2 }}>
          {label}
        </Typography>
      </Box>
    </Box>
  );
}
export function ErrorView({
  message,
  retry,
}: {
  message: string;
  retry?: () => void;
}) {
  return (
    <Alert
      severity="error"
      action={
        retry && (
          <Button color="inherit" onClick={retry}>
            다시 시도
          </Button>
        )
      }
    >
      {message}
    </Alert>
  );
}
export function EmptyView({
  title,
  description,
  action,
}: {
  title: string;
  description: string;
  action?: React.ReactNode;
}) {
  return (
    <Box
      sx={{
        py: 8,
        px: 3,
        textAlign: "center",
        border: "1px dashed",
        borderColor: "divider",
        borderRadius: 4,
        background:
          "radial-gradient(circle at 50% 25%,rgba(124,108,242,.13),transparent 45%)",
      }}
    >
      <AutoAwesomeRoundedIcon color="primary" sx={{ fontSize: 38, mb: 1.5 }} />
      <Typography variant="h3">{title}</Typography>
      <Typography
        color="text.secondary"
        sx={{ maxWidth: 480, mx: "auto", mt: 1, mb: action ? 2.5 : 0 }}
      >
        {description}
      </Typography>
      {action}
    </Box>
  );
}
