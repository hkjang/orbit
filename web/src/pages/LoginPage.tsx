import { useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  Divider,
  TextField,
  Typography,
} from "@mui/material";
import LoginRoundedIcon from "@mui/icons-material/LoginRounded";
import ShieldOutlinedIcon from "@mui/icons-material/ShieldOutlined";
import { useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../AuthContext";
import { Brand } from "../components/Brand";

export function LoginPage() {
  const { login, config } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError("");
    setBusy(true);
    try {
      await login(username, password);
      navigate((location.state as { from?: string })?.from ?? "/");
    } catch (e) {
      setError(e instanceof Error ? e.message : "로그인하지 못했습니다.");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Box
      sx={{
        minHeight: "100vh",
        display: "grid",
        gridTemplateColumns: { xs: "1fr", md: "minmax(420px,45%) 1fr" },
        bgcolor: "#090b18",
      }}
    >
      <Box
        sx={{
          p: { xs: 3, sm: 6, md: 8 },
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          minHeight: { xs: 260, md: "100vh" },
          background:
            "radial-gradient(circle at 20% 15%,rgba(162,140,255,.2),transparent 32%),radial-gradient(circle at 75% 72%,rgba(246,201,107,.12),transparent 27%)",
          position: "relative",
          overflow: "hidden",
        }}
      >
        <Brand />
        <Box sx={{ maxWidth: 580, my: { xs: 8, md: 0 } }}>
          <Typography
            variant="overline"
            color="primary.light"
            sx={{ letterSpacing: ".18em" }}
          >
            PERSONAL RELATIONSHIP UNIVERSE
          </Typography>
          <Typography
            component="h1"
            sx={{
              fontSize: { xs: "2.5rem", sm: "3.7rem" },
              fontWeight: 780,
              lineHeight: 1.08,
              letterSpacing: "-.05em",
              mt: 1.5,
            }}
          >
            Your relationships
            <br />
            <Box component="span" sx={{ color: "secondary.main" }}>
              have gravity.
            </Box>
          </Typography>
          <Typography
            color="text.secondary"
            sx={{ fontSize: "1.08rem", mt: 3, maxWidth: 470 }}
          >
            사람들이 내 삶에 들어오고, 가까워지고, 기억으로 남는 흐름을 하나의
            우주에서 만나보세요.
          </Typography>
        </Box>
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            gap: 1,
            color: "text.secondary",
          }}
        >
          <ShieldOutlinedIcon fontSize="small" />
          <Typography variant="body2">
            Your relationships belong to you.
          </Typography>
        </Box>
        {[
          ["12%", "18%"],
          ["78%", "14%"],
          ["85%", "62%"],
          ["24%", "78%"],
        ].map(([left, top], i) => (
          <Box
            key={left}
            sx={{
              position: "absolute",
              left,
              top,
              width: i === 1 ? 8 : 5,
              height: i === 1 ? 8 : 5,
              borderRadius: "50%",
              bgcolor: i === 2 ? "secondary.main" : "primary.light",
              boxShadow: "0 0 16px currentColor",
              opacity: 0.65,
            }}
          />
        ))}
      </Box>
      <Box
        sx={{ display: "grid", placeItems: "center", p: { xs: 2.5, sm: 6 } }}
      >
        <Card
          sx={{
            width: "100%",
            maxWidth: 460,
            bgcolor: "rgba(18,21,40,.92)",
            boxShadow: "0 28px 90px rgba(0,0,0,.38)",
          }}
        >
          <CardContent
            sx={{
              p: { xs: 3, sm: 4.5 },
              "&:last-child": { pb: { xs: 3, sm: 4.5 } },
            }}
          >
            <Typography variant="h2">다시 오신 것을 환영해요</Typography>
            <Typography color="text.secondary" sx={{ mt: 1, mb: 3 }}>
              나의 Orbit으로 안전하게 들어갑니다.
            </Typography>
            {error && (
              <Alert severity="error" sx={{ mb: 2 }}>
                {error}
              </Alert>
            )}
            <Box component="form" onSubmit={submit}>
              <TextField
                fullWidth
                label="사용자 ID"
                autoComplete="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                sx={{ mb: 2 }}
                slotProps={{ htmlInput: { style: { fontSize: 16 } } }}
              />
              <TextField
                fullWidth
                label="비밀번호"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                slotProps={{ htmlInput: { style: { fontSize: 16 } } }}
              />
              <Button
                fullWidth
                type="submit"
                variant="contained"
                size="large"
                disabled={busy || !username || !password}
                startIcon={
                  busy ? <CircularProgress size={18} /> : <LoginRoundedIcon />
                }
                sx={{ mt: 3 }}
              >
                Orbit 시작하기
              </Button>
            </Box>
            {config?.oidc.enabled && (
              <>
                <Divider sx={{ my: 3 }}>또는</Divider>
                <Button
                  fullWidth
                  variant="outlined"
                  size="large"
                  href="/api/v1/auth/oidc/start"
                >
                  {config.oidc.display_name || "Keycloak SSO"}로 계속
                </Button>
              </>
            )}
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ display: "block", textAlign: "center", mt: 3 }}
            >
              Orbit {config?.version ?? "dev"} · Offline-ready
            </Typography>
          </CardContent>
        </Card>
      </Box>
    </Box>
  );
}
