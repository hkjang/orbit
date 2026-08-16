import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Typography,
} from "@mui/material";
import ArrowForwardRoundedIcon from "@mui/icons-material/ArrowForwardRounded";
import PersonAddAltRoundedIcon from "@mui/icons-material/PersonAddAltRounded";
import { useNavigate } from "react-router-dom";
import { api, formatDate } from "../api";
import { useAuth } from "../AuthContext";
import { OrbitCanvas } from "../components/OrbitCanvas";
import { EmptyView, ErrorView, LoadingView } from "../components/StateViews";
import type { OrbitNode } from "../types";

export function OrbitPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const [nodes, setNodes] = useState<OrbitNode[]>();
  const [contexts, setContexts] = useState<Record<string, number>>({});
  const [selectedContext, setSelectedContext] = useState("");
  const [rediscover, setRediscover] = useState<{
    person_id: string;
    person_name: string;
    title: string;
    occurred_at?: string;
  } | null>();
  const [error, setError] = useState("");
  const load = useCallback(async () => {
    setError("");
    try {
      const [orbit, discovery] = await Promise.all([
        api<{ nodes: OrbitNode[]; contexts: Record<string, number> }>("/orbit"),
        api<{ item: typeof rediscover }>("/rediscover"),
      ]);
      setNodes(orbit.nodes);
      setContexts(orbit.contexts);
      setRediscover(discovery.item);
    } catch (e) {
      setError(e instanceof Error ? e.message : "우주를 불러오지 못했습니다.");
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);
  if (error) return <ErrorView message={error} retry={load} />;
  if (!nodes) return <LoadingView />;
  if (nodes.length === 0)
    return (
      <>
        <Box sx={{ mb: 4 }}>
          <Typography variant="h1">
            안녕하세요, {user?.display_name}님
          </Typography>
          <Typography color="text.secondary" sx={{ mt: 1 }}>
            당신의 관계가 머무를 첫 우주입니다.
          </Typography>
        </Box>
        <EmptyView
          title="당신의 우주를 만들어볼까요?"
          description="중요한 사람을 한 명씩 더하면 새로운 행성이 나타납니다. 먼저 떠오르는 사람부터 시작해 보세요."
          action={
            <Button
              variant="contained"
              startIcon={<PersonAddAltRoundedIcon />}
              onClick={() => navigate("/people?new=1")}
            >
              첫 사람 추가하기
            </Button>
          }
        />
      </>
    );
  return (
    <Box>
      <Box
        sx={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: { xs: "flex-start", sm: "center" },
          mb: 2.5,
          gap: 2,
          flexDirection: { xs: "column", sm: "row" },
        }}
      >
        <Box>
          <Typography variant="h1">나의 Orbit</Typography>
          <Typography color="text.secondary" sx={{ mt: 0.6 }}>
            관계는 평가가 아니라 움직임으로 보입니다.
          </Typography>
        </Box>
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
          {Object.entries(contexts)
            .sort((a, b) => b[1] - a[1])
            .slice(0, 4)
            .map(([name, count]) => (
              <Chip
                key={name}
                label={`${name} ${count}`}
                variant={selectedContext === name ? "filled" : "outlined"}
                color={selectedContext === name ? "primary" : "default"}
                clickable
                onClick={() =>
                  setSelectedContext((current) => (current === name ? "" : name))
                }
              />
            ))}
        </Box>
      </Box>
      {rediscover && (
        <Card
          sx={{
            mb: 2.5,
            background:
              "linear-gradient(110deg,rgba(244,201,107,.12),rgba(169,155,248,.10))",
          }}
        >
          <CardContent
            sx={{
              display: "flex",
              alignItems: { xs: "flex-start", sm: "center" },
              justifyContent: "space-between",
              gap: 2,
              py: "18px!important",
              flexDirection: { xs: "column", sm: "row" },
            }}
          >
            <Box>
              <Typography
                variant="overline"
                color="secondary.main"
                sx={{ letterSpacing: ".13em" }}
              >
                REDISCOVER
              </Typography>
              <Typography sx={{ fontWeight: 720 }}>
                {rediscover.person_name}님과의 기억 · {rediscover.title}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                {formatDate(rediscover.occurred_at)}
              </Typography>
            </Box>
            <Button
              endIcon={<ArrowForwardRoundedIcon />}
              onClick={() => navigate(`/people/${rediscover.person_id}`)}
            >
              기억 다시 보기
            </Button>
          </CardContent>
        </Card>
      )}
      <OrbitCanvas
        nodes={
          selectedContext
            ? nodes.filter((node) => node.categories.includes(selectedContext))
            : nodes
        }
        centerName={user?.display_name ?? "나"}
        onSelect={(node) => navigate(`/people/${node.id}`)}
      />
      <Alert
        severity="info"
        icon={false}
        sx={{ mt: 2, bgcolor: "rgba(120,183,241,.07)" }}
      >
        행성의 크기는 장기 중요도, 거리는 최근 관계 활성도, 초록 궤적은
        가까워지는 흐름을 나타냅니다. 숫자로 관계를 평가하지 않습니다.
      </Alert>
    </Box>
  );
}
