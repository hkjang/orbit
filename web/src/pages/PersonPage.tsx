import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  IconButton,
  MenuItem,
  TextField,
  Typography,
} from "@mui/material";
import ArrowBackRoundedIcon from "@mui/icons-material/ArrowBackRounded";
import EditRoundedIcon from "@mui/icons-material/EditRounded";
import AddCommentRoundedIcon from "@mui/icons-material/AddCommentRounded";
import AutoAwesomeRoundedIcon from "@mui/icons-material/AutoAwesomeRounded";
import { useNavigate, useParams } from "react-router-dom";
import { StateChip } from "../components/StateChip";
import { readGrammar } from "../orbitGrammar";
import { api, formatDate } from "../api";
import { ErrorView, LoadingView, EmptyView } from "../components/StateViews";
import { PersonFormDialog } from "../components/PersonFormDialog";
import type { Interaction, Memory, Person } from "../types";

export function PersonPage() {
  const { personId } = useParams();
  const navigate = useNavigate();
  const [data, setData] = useState<{
    person: Person;
    interactions: Interaction[];
    memories: Memory[];
  }>();
  const [error, setError] = useState("");
  const [edit, setEdit] = useState(false);
  const [interaction, setInteraction] = useState(false);
  const load = useCallback(async () => {
    if (!personId) return;
    setError("");
    try {
      setData(await api(`/people/${personId}`));
    } catch (e) {
      setError(e instanceof Error ? e.message : "관계를 불러오지 못했습니다.");
    }
  }, [personId]);
  useEffect(() => {
    void load();
  }, [load]);
  if (error) return <ErrorView message={error} retry={load} />;
  if (!data) return <LoadingView />;
  const { person, interactions, memories } = data;
  const years = person.first_met
    ? Math.max(
        1,
        new Date().getFullYear() - new Date(person.first_met).getFullYear(),
      )
    : undefined;
  const grammar = readGrammar(person);
  const trend = {
    approaching: "최근 함께하는 시간이 많아졌어요",
    stable: "편안한 궤도를 이어가고 있어요",
    drifting: "요즘은 조금 먼 궤도에 있어요",
    dormant: "한동안 서로의 소식이 닿지 않았어요",
  }[grammar.state];
  return (
    <>
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 3 }}>
        <IconButton onClick={() => navigate(-1)} aria-label="뒤로">
          <ArrowBackRoundedIcon />
        </IconButton>
        <Typography color="text.secondary">관계 이야기</Typography>
      </Box>
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr", lg: "minmax(0,1fr) 360px" },
          gap: 3,
        }}
      >
        <Box>
          <Card
            sx={{
              background:
                "radial-gradient(circle at 15% 0%,rgba(169,155,248,.18),transparent 40%),rgba(18,21,40,.9)",
            }}
          >
            <CardContent sx={{ p: { xs: 3, sm: 4 } }}>
              <Box
                sx={{
                  display: "flex",
                  alignItems: { xs: "flex-start", sm: "center" },
                  gap: 2.5,
                  flexDirection: { xs: "column", sm: "row" },
                }}
              >
                <Avatar
                  src={person.avatar_url}
                  sx={{
                    width: 82,
                    height: 82,
                    bgcolor: "primary.dark",
                    fontSize: 30,
                    boxShadow: "0 0 28px rgba(169,155,248,.3)",
                  }}
                >
                  {person.display_name[0]}
                </Avatar>
                <Box sx={{ flex: 1 }}>
                  <Typography variant="h1">{person.display_name}</Typography>
                  <Typography color="text.secondary" sx={{ mt: 0.5 }}>
                    {person.relationship_label || "우리 관계"}
                    {years && ` · ${years}년째`}
                  </Typography>
                  <Box
                    sx={{
                      display: "flex",
                      gap: 0.7,
                      mt: 1.5,
                      flexWrap: "wrap",
                    }}
                  >
                    {person.categories.map((c) => (
                      <Chip key={c} size="small" label={c} />
                    ))}
                  </Box>
                </Box>
                <Button
                  variant="outlined"
                  startIcon={<EditRoundedIcon />}
                  onClick={() => setEdit(true)}
                >
                  다듬기
                </Button>
              </Box>
              <Divider sx={{ my: 3 }} />
              <Box
                sx={{
                  display: "flex",
                  alignItems: "center",
                  gap: 1.2,
                  flexWrap: "wrap",
                }}
              >
                <Typography variant="overline" color="primary.light">
                  NOW IN ORBIT
                </Typography>
                <StateChip person={person} />
              </Box>
              <Typography variant="h3" sx={{ mt: 0.5 }}>
                {trend}
              </Typography>
              <Typography color="text.secondary" sx={{ mt: 0.8 }}>
                {grammar.hint}
              </Typography>
              <Typography color="text.secondary" sx={{ mt: 1 }}>
                마지막 교류는 {formatDate(person.last_interaction_at)}이에요.
                거리와 중요도는 서로 다르게 움직입니다.
              </Typography>
            </CardContent>
          </Card>
          <Box sx={{ mt: 3 }}>
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                mb: 1.5,
              }}
            >
              <Typography variant="h2">함께한 시간</Typography>
              <Button
                startIcon={<AddCommentRoundedIcon />}
                onClick={() => setInteraction(true)}
              >
                교류 기록
              </Button>
            </Box>
            {interactions.length === 0 && memories.length === 0 ? (
              <EmptyView
                title="아직 기록된 순간이 없어요"
                description="짧은 메모나 만남을 남기면 두 사람의 타임라인이 시작됩니다."
              />
            ) : (
              <Box
                sx={{
                  position: "relative",
                  pl: 3,
                  "&::before": {
                    content: '""',
                    position: "absolute",
                    left: 7,
                    top: 10,
                    bottom: 10,
                    width: 1,
                    bgcolor: "divider",
                  },
                }}
              >
                {[
                  ...memories.map((m) => ({
                    id: m.id,
                    date: m.occurred_at ?? m.created_at,
                    title: m.title,
                    body: m.content,
                    type: "기억",
                    status: m.status,
                  })),
                  ...interactions.map((i) => ({
                    id: i.id,
                    date: i.occurred_at,
                    title: kindLabel(i.kind),
                    body: i.summary,
                    type: "교류",
                    status: "approved",
                  })),
                ]
                  .sort((a, b) => +new Date(b.date) - +new Date(a.date))
                  .map((item) => (
                    <Box
                      key={`${item.type}-${item.id}`}
                      sx={{
                        position: "relative",
                        mb: 2.5,
                        "&::before": {
                          content: '""',
                          position: "absolute",
                          width: 11,
                          height: 11,
                          borderRadius: "50%",
                          bgcolor:
                            item.type === "기억"
                              ? "secondary.main"
                              : "primary.main",
                          left: -21.5,
                          top: 7,
                          boxShadow: "0 0 12px currentColor",
                        },
                      }}
                    >
                      <Typography variant="caption" color="text.secondary">
                        {formatDate(item.date)} · {item.type}
                      </Typography>
                      <Typography sx={{ mt: 0.3, fontWeight: 720 }}>
                        {item.title}{" "}
                        {item.status === "pending" && (
                          <Chip
                            label="검토 대기"
                            size="small"
                            color="warning"
                            sx={{ ml: 1 }}
                          />
                        )}
                      </Typography>
                      {item.body && (
                        <Typography
                          color="text.secondary"
                          sx={{ whiteSpace: "pre-wrap", mt: 0.4 }}
                        >
                          {item.body}
                        </Typography>
                      )}
                    </Box>
                  ))}
              </Box>
            )}
          </Box>
        </Box>
        <Box>
          <Card>
            <CardContent>
              <Typography variant="h3">관계 정보</Typography>
              <Info label="회사/소속" value={person.company} />
              <Info label="역할" value={person.role_title} />
              <Info label="처음 만난 날" value={formatDate(person.first_met)} />
              <Info label="이메일" value={person.email} />
              <Info label="전화번호" value={person.phone} />
              {person.note && (
                <>
                  <Divider sx={{ my: 2 }} />
                  <Typography variant="body2" color="text.secondary">
                    나만의 메모
                  </Typography>
                  <Typography sx={{ mt: 0.5, whiteSpace: "pre-wrap" }}>
                    {person.note}
                  </Typography>
                </>
              )}
            </CardContent>
          </Card>
          <Button
            fullWidth
            variant="contained"
            startIcon={<AutoAwesomeRoundedIcon />}
            sx={{ mt: 2 }}
            onClick={() => navigate(`/ai?person=${person.id}`)}
          >
            {person.display_name}님에 대해 AI에게 묻기
          </Button>
        </Box>
      </Box>
      <PersonFormDialog
        open={edit}
        person={person}
        onClose={() => setEdit(false)}
        onSaved={load}
      />
      <InteractionDialog
        open={interaction}
        personId={person.id}
        onClose={() => setInteraction(false)}
        onSaved={load}
      />
    </>
  );
}

function Info({ label, value }: { label: string; value?: string }) {
  return (
    <Box sx={{ mt: 2 }}>
      <Typography variant="body2" color="text.secondary">
        {label}
      </Typography>
      <Typography>{value || "기록 없음"}</Typography>
    </Box>
  );
}
function kindLabel(kind: string) {
  return (
    (
      {
        meeting: "함께 만남",
        call: "통화",
        message: "메시지",
        note: "메모",
        other: "교류",
      } as Record<string, string>
    )[kind] ?? kind
  );
}
function InteractionDialog({
  open,
  personId,
  onClose,
  onSaved,
}: {
  open: boolean;
  personId: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [kind, setKind] = useState("meeting");
  const [occurred, setOccurred] = useState(() =>
    new Date().toISOString().slice(0, 16),
  );
  const [summary, setSummary] = useState("");
  const [error, setError] = useState("");
  const save = async () => {
    try {
      await api(`/people/${personId}/interactions`, {
        method: "POST",
        body: JSON.stringify({
          kind,
          occurred_at: new Date(occurred).toISOString(),
          weight: 1,
          summary,
        }),
      });
      setSummary("");
      onSaved();
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : "기록하지 못했습니다.");
    }
  };
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="xs">
      <DialogTitle>새로운 교류 기록</DialogTitle>
      <DialogContent>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        <TextField
          select
          fullWidth
          label="교류 유형"
          value={kind}
          onChange={(e) => setKind(e.target.value)}
          sx={{ mt: 1 }}
        >
          {[
            ["meeting", "함께 만남"],
            ["call", "통화"],
            ["message", "메시지"],
            ["note", "메모"],
            ["other", "기타"],
          ].map(([v, l]) => (
            <MenuItem key={v} value={v}>
              {l}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          fullWidth
          type="datetime-local"
          label="언제"
          slotProps={{ inputLabel: { shrink: true } }}
          value={occurred}
          onChange={(e) => setOccurred(e.target.value)}
          sx={{ mt: 2 }}
        />
        <TextField
          fullWidth
          multiline
          minRows={3}
          label="어떤 시간을 보냈나요?"
          value={summary}
          onChange={(e) => setSummary(e.target.value)}
          sx={{ mt: 2 }}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>취소</Button>
        <Button variant="contained" onClick={save}>
          기록하기
        </Button>
      </DialogActions>
    </Dialog>
  );
}
