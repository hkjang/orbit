import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  MenuItem,
  TextField,
  Typography,
} from "@mui/material";
import AddRoundedIcon from "@mui/icons-material/AddRounded";
import AutoStoriesRoundedIcon from "@mui/icons-material/AutoStoriesRounded";
import { api, formatDate } from "../api";
import { PageHeader } from "../components/PageHeader";
import { EmptyView, ErrorView, LoadingView } from "../components/StateViews";
import type { Memory, Person } from "../types";

export function MemoriesPage() {
  const [memories, setMemories] = useState<Memory[]>();
  const [people, setPeople] = useState<Person[]>([]);
  const [error, setError] = useState("");
  const [open, setOpen] = useState(false);
  const load = useCallback(async () => {
    setError("");
    try {
      const [m, p] = await Promise.all([
        api<{ memories: Memory[] }>("/memories/"),
        api<{ people: Person[] }>("/people/"),
      ]);
      setMemories(m.memories);
      setPeople(p.people);
    } catch (e) {
      setError(e instanceof Error ? e.message : "기억을 불러오지 못했습니다.");
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);
  return (
    <>
      <PageHeader
        title="Relationship Memory"
        description="사람과 함께한 순간을 나만의 맥락으로 기억합니다."
        action={
          <Button
            variant="contained"
            startIcon={<AddRoundedIcon />}
            onClick={() => setOpen(true)}
          >
            기억 남기기
          </Button>
        }
      />
      {error ? (
        <ErrorView message={error} retry={load} />
      ) : !memories ? (
        <LoadingView />
      ) : memories.length === 0 ? (
        <EmptyView
          title="아직 꺼내볼 기억이 없어요"
          description="짧은 문장 하나로 시작해도 충분합니다. Orbit이 시간과 사람을 이어줍니다."
          action={
            <Button variant="contained" onClick={() => setOpen(true)}>
              첫 기억 남기기
            </Button>
          }
        />
      ) : (
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill,minmax(300px,1fr))",
            gap: 2,
          }}
        >
          {memories.map((memory) => (
            <Card
              key={memory.id}
              sx={{ opacity: memory.status === "rejected" ? 0.68 : 1 }}
            >
              <CardContent sx={{ p: 2.7 }}>
                <Box
                  sx={{
                    display: "flex",
                    justifyContent: "space-between",
                    gap: 1,
                    alignItems: "flex-start",
                  }}
                >
                  <Box>
                    <Typography variant="overline" color="primary.light">
                      {memory.person_name || "나의 기록"}
                    </Typography>
                    <Typography variant="h3">{memory.title}</Typography>
                  </Box>
                  {memory.status !== "approved" && (
                    <Chip
                      size="small"
                      color={
                        memory.status === "pending" ? "warning" : "default"
                      }
                      label={
                        memory.status === "pending"
                          ? "검토 대기"
                          : memory.status === "rejected"
                            ? "반려됨"
                            : memory.status
                      }
                    />
                  )}
                </Box>
                <Typography
                  color="text.secondary"
                  sx={{
                    mt: 1.5,
                    whiteSpace: "pre-wrap",
                    display: "-webkit-box",
                    WebkitLineClamp: 5,
                    WebkitBoxOrient: "vertical",
                    overflow: "hidden",
                  }}
                >
                  {memory.content}
                </Typography>
                <Box
                  sx={{ display: "flex", gap: 0.7, mt: 2, flexWrap: "wrap" }}
                >
                  {memory.topics.map((topic) => (
                    <Chip
                      key={topic}
                      size="small"
                      variant="outlined"
                      label={topic}
                    />
                  ))}
                </Box>
                <Typography
                  variant="body2"
                  color="text.secondary"
                  sx={{ mt: 2 }}
                >
                  {formatDate(memory.occurred_at ?? memory.created_at)} ·{" "}
                  {sourceLabel(memory.source_type)}
                </Typography>
              </CardContent>
            </Card>
          ))}
        </Box>
      )}
      <MemoryDialog
        open={open}
        people={people}
        onClose={() => setOpen(false)}
        onSaved={load}
      />
    </>
  );
}

function sourceLabel(v: string) {
  return (
    (
      { manual: "직접 기록", mcp: "MCP", ai: "AI 정리" } as Record<
        string,
        string
      >
    )[v] ?? v
  );
}
function MemoryDialog({
  open,
  people,
  onClose,
  onSaved,
}: {
  open: boolean;
  people: Person[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [form, setForm] = useState({
    person_id: "",
    title: "",
    content: "",
    occurred_at: "",
    topics: "",
    request_note: "",
  });
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const save = async () => {
    setBusy(true);
    setError("");
    try {
      await api("/memories/", {
        method: "POST",
        body: JSON.stringify({
          ...form,
          occurred_at: form.occurred_at
            ? new Date(form.occurred_at).toISOString()
            : null,
          topics: form.topics
            .split(",")
            .map((v) => v.trim())
            .filter(Boolean),
        }),
      });
      setForm({
        person_id: "",
        title: "",
        content: "",
        occurred_at: "",
        topics: "",
        request_note: "",
      });
      onSaved();
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : "기억을 남기지 못했습니다.");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>
        <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
          <AutoStoriesRoundedIcon color="primary" />
          기억 남기기
        </Box>
      </DialogTitle>
      <DialogContent dividers>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        <TextField
          select
          fullWidth
          label="함께한 사람"
          value={form.person_id}
          onChange={(e) => setForm({ ...form, person_id: e.target.value })}
          sx={{ mt: 0.5 }}
        >
          <MenuItem value="">사람을 지정하지 않음</MenuItem>
          {people.map((p) => (
            <MenuItem key={p.id} value={p.id}>
              {p.display_name}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          fullWidth
          required
          label="기억의 제목"
          value={form.title}
          onChange={(e) => setForm({ ...form, title: e.target.value })}
          sx={{ mt: 2 }}
        />
        <TextField
          fullWidth
          required
          multiline
          minRows={5}
          label="어떤 순간이었나요?"
          value={form.content}
          onChange={(e) => setForm({ ...form, content: e.target.value })}
          sx={{ mt: 2 }}
        />
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" },
            gap: 2,
            mt: 2,
          }}
        >
          <TextField
            type="datetime-local"
            label="언제"
            slotProps={{ inputLabel: { shrink: true } }}
            value={form.occurred_at}
            onChange={(e) => setForm({ ...form, occurred_at: e.target.value })}
          />
          <TextField
            label="주제"
            placeholder="여행, 창업, 러닝"
            value={form.topics}
            onChange={(e) => setForm({ ...form, topics: e.target.value })}
          />
        </Box>
        <TextField
          fullWidth
          label="검토 요청 메모 (선택)"
          helperText="승인 프로세스가 활성화된 경우 팀장에게 전달됩니다."
          value={form.request_note}
          onChange={(e) => setForm({ ...form, request_note: e.target.value })}
          sx={{ mt: 2 }}
        />
      </DialogContent>
      <DialogActions sx={{ p: 2 }}>
        <Button onClick={onClose}>취소</Button>
        <Button
          variant="contained"
          disabled={busy || !form.title || !form.content}
          onClick={save}
        >
          {busy ? "저장 중…" : "기억하기"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
