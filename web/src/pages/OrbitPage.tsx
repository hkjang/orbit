import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Slider,
  Typography,
} from "@mui/material";
import ArrowForwardRoundedIcon from "@mui/icons-material/ArrowForwardRounded";
import PersonAddAltRoundedIcon from "@mui/icons-material/PersonAddAltRounded";
import { useNavigate } from "react-router-dom";
import { api, formatDate } from "../api";
import { useAuth } from "../AuthContext";
import { OrbitCanvas } from "../components/OrbitCanvas";
import { EmptyView, ErrorView, LoadingView } from "../components/StateViews";
import { describeEclipse, findEclipses } from "../eclipse";
import { STATE_META, STATE_ORDER } from "../orbitGrammar";
import type { OrbitLink, OrbitNode } from "../types";

export function OrbitPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const [nodes, setNodes] = useState<OrbitNode[]>();
  const [contexts, setContexts] = useState<Record<string, number>>({});
  const [links, setLinks] = useState<OrbitLink[]>([]);
  const [categories, setCategories] = useState<string[]>([]);
  const [constellation, setConstellation] = useState("");
  const [eclipseFocus, setEclipseFocus] = useState<string[]>();
  const [forecast, setForecast] = useState(false);
  // 시간 여행. 비어 있으면 현재를 뜻한다.
  const [travelTo, setTravelTo] = useState<string>();
  const [earliest, setEarliest] = useState<string>();
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
        api<{
          nodes: OrbitNode[];
          contexts: Record<string, number>;
          links: OrbitLink[];
          categories: string[];
          earliest_at?: string;
        }>(travelTo ? `/orbit?at=${encodeURIComponent(travelTo)}` : "/orbit"),
        api<{ item: typeof rediscover }>("/rediscover"),
      ]);
      setNodes(orbit.nodes);
      setContexts(orbit.contexts);
      setLinks(orbit.links ?? []);
      setCategories(orbit.categories ?? []);
      // 구간의 시작점은 현재 화면을 부를 때만 온다. 과거로 간 뒤에도 유지한다.
      if (orbit.earliest_at) setEarliest(orbit.earliest_at);
      setRediscover(discovery.item);
    } catch (e) {
      setError(e instanceof Error ? e.message : "우주를 불러오지 못했습니다.");
    }
  }, [travelTo]);
  useEffect(() => {
    void load();
  }, [load]);
  const eclipses = useMemo(
    () => (nodes ? findEclipses(nodes, links) : []),
    [nodes, links],
  );
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
          {!travelTo && (
            <Chip
              label="90일 후 미리보기"
              variant={forecast ? "filled" : "outlined"}
              color={forecast ? "secondary" : "default"}
              clickable
              aria-pressed={forecast}
              title="이대로 두면 어디로 밀려나는지 유령 궤도로 겹쳐 봅니다."
              onClick={() => setForecast((on) => !on)}
            />
          )}
          {Object.entries(contexts)
            .sort((a, b) => b[1] - a[1])
            .slice(0, 4)
            .map(([name, count]) => (
              <Chip
                key={name}
                label={`${name} ${count}`}
                variant={constellation === name ? "filled" : "outlined"}
                color={constellation === name ? "primary" : "default"}
                clickable
                aria-pressed={constellation === name}
                title={`${name} 별자리로 잇기`}
                onClick={() =>
                  setConstellation((current) => (current === name ? "" : name))
                }
              />
            ))}
        </Box>
      </Box>
      {earliest && (
        <TimeTravel
          earliest={earliest}
          value={travelTo}
          onChange={setTravelTo}
        />
      )}
      {eclipses.length > 0 && nodes && (
        <EclipseCard
          group={eclipses[0]}
          nodes={nodes}
          focused={Boolean(eclipseFocus)}
          onToggle={() =>
            setEclipseFocus((current) =>
              current ? undefined : eclipses[0].members.map((m) => m.id),
            )
          }
          onOpen={(personId) => navigate(`/people/${personId}`)}
        />
      )}
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
                {formatDate(rediscover.occurred_at)} · 지도 위에 혜성으로
                다가오고 있어요
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
        nodes={nodes}
        centerName={user?.display_name ?? "나"}
        onSelect={(node) => navigate(`/people/${node.id}`)}
        constellation={constellation || undefined}
        rediscover={
          rediscover
            ? { person_id: rediscover.person_id, title: rediscover.title }
            : undefined
        }
        links={links}
        focus={eclipseFocus}
        forecastDays={forecast && !travelTo ? 90 : undefined}
        asOf={travelTo ? Date.parse(travelTo) : undefined}
        categoryOrder={categories}
      />
      <Alert
        severity="info"
        icon={false}
        sx={{ mt: 2, bgcolor: "rgba(120,183,241,.07)" }}
      >
        <Typography variant="body2" sx={{ mb: 1 }}>
          크기는 중요도, 거리는 친밀도, 화살표는 관계가 지금 움직이는
          방향입니다. 숫자로 관계를 평가하지 않습니다.
        </Typography>
        <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap" }}>
          {STATE_ORDER.map((state) => (
            <Box
              key={state}
              sx={{ display: "flex", alignItems: "center", gap: 0.8 }}
            >
              <Box
                sx={{
                  width: 9,
                  height: 9,
                  borderRadius: "50%",
                  bgcolor: STATE_META[state].tone,
                  flexShrink: 0,
                }}
              />
              <Typography variant="caption" color="text.secondary">
                <b>{STATE_META[state].label}</b> · {STATE_META[state].hint}
              </Typography>
            </Box>
          ))}
        </Box>
      </Alert>
    </Box>
  );
}

/**
 * 함께 조용해진 그룹을 한 장의 사건으로 보여줍니다.
 * 사람마다 빨간 배지를 다는 대신, 무슨 일이 있었는지 묻게 만드는 쪽을 택했습니다.
 */
function EclipseCard({
  group,
  nodes,
  focused,
  onToggle,
  onOpen,
}: {
  group: ReturnType<typeof findEclipses>[number];
  nodes: OrbitNode[];
  focused: boolean;
  onToggle: () => void;
  onOpen: (personId: string) => void;
}) {
  const copy = describeEclipse(group, nodes);
  return (
    <Card
      sx={{
        mb: 2.5,
        background:
          "linear-gradient(110deg,rgba(120,183,241,.10),rgba(169,155,248,.10))",
      }}
    >
      <CardContent sx={{ py: "18px!important" }}>
        <Typography
          variant="overline"
          color="primary.light"
          sx={{ letterSpacing: ".13em" }}
        >
          ECLIPSE
        </Typography>
        <Typography sx={{ fontWeight: 720 }}>{copy.headline}</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.3 }}>
          {copy.hint}
        </Typography>
        <Box sx={{ display: "flex", gap: 0.8, flexWrap: "wrap", mt: 1.5 }}>
          {group.members.map((member) => (
            <Chip
              key={member.id}
              size="small"
              clickable
              variant={
                group.faded.some((f) => f.id === member.id)
                  ? "outlined"
                  : "filled"
              }
              label={member.name}
              onClick={() => onOpen(member.id)}
            />
          ))}
          <Button size="small" onClick={onToggle}>
            {focused ? "전체 우주 보기" : "지도에서 보기"}
          </Button>
        </Box>
      </CardContent>
    </Card>
  );
}

/**
 * 과거의 우주로 옮겨 가는 슬라이더.
 *
 * 되살아나는 것은 교류가 만든 거리와 흐름이다. 중요도·소속·고정 여부는
 * 사용자가 직접 정하는 값이라 변경 이력이 없어 오늘의 값을 쓴다. 그 사실을
 * 화면에서도 숨기지 않는다.
 */
function TimeTravel({
  earliest,
  value,
  onChange,
}: {
  earliest: string;
  value?: string;
  onChange: (value?: string) => void;
}) {
  const start = new Date(earliest).getTime();
  const end = Date.now();
  const current = value ? new Date(value).getTime() : end;
  const days = Math.max(1, Math.round((end - start) / 86_400_000));
  const dayOf = (time: number) => Math.round((time - start) / 86_400_000);
  return (
    <Card sx={{ mb: 2.5 }}>
      <CardContent sx={{ py: "16px!important" }}>
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            gap: 2,
            flexWrap: "wrap",
          }}
        >
          <Box sx={{ minWidth: 160 }}>
            <Typography
              variant="overline"
              color="primary.light"
              sx={{ letterSpacing: ".13em" }}
            >
              TIME TRAVEL
            </Typography>
            <Typography sx={{ fontWeight: 720 }}>
              {value ? formatDate(value) : "지금의 우주"}
            </Typography>
          </Box>
          <Slider
            value={dayOf(current)}
            min={0}
            max={days}
            step={1}
            aria-label="돌아볼 시점"
            valueLabelDisplay="auto"
            valueLabelFormat={(day) =>
              formatDate(new Date(start + day * 86_400_000).toISOString())
            }
            onChange={(_, day) => {
              const at = new Date(start + (day as number) * 86_400_000);
              onChange(
                dayOf(at.getTime()) >= days ? undefined : at.toISOString(),
              );
            }}
            sx={{ flex: "1 1 240px", mx: 1 }}
          />
          {value && (
            <Button size="small" onClick={() => onChange(undefined)}>
              현재로
            </Button>
          )}
        </Box>
        {value && (
          <Typography variant="caption" color="text.secondary">
            그날의 거리와 흐름을 교류 기록에서 다시 계산했습니다.
            중요도·소속·고정은 변경 이력이 없어 오늘의 값을 씁니다.
          </Typography>
        )}
      </CardContent>
    </Card>
  );
}
