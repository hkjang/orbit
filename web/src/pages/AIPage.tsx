import { useEffect, useRef, useState } from "react";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  MenuItem,
  TextField,
  Typography,
} from "@mui/material";
import AutoAwesomeRoundedIcon from "@mui/icons-material/AutoAwesomeRounded";
import StopCircleOutlinedIcon from "@mui/icons-material/StopCircleOutlined";
import SendRoundedIcon from "@mui/icons-material/SendRounded";
import { useSearchParams } from "react-router-dom";
import { api, streamAI } from "../api";
import { PageHeader } from "../components/PageHeader";
import type { Person } from "../types";

interface Message {
  role: "user" | "assistant";
  content: string;
}
const suggestions = [
  "요즘 내가 자주 만나는 사람은?",
  "예전에 AI 이야기를 했던 사람들을 알려줘",
  "최근 가까워진 관계를 부드럽게 설명해줘",
];
export function AIPage() {
  const [params] = useSearchParams();
  const [people, setPeople] = useState<Person[]>([]);
  const [personId, setPersonId] = useState(params.get("person") ?? "");
  const [prompt, setPrompt] = useState("");
  const [messages, setMessages] = useState<Message[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const abort = useRef<AbortController | undefined>(undefined);
  useEffect(() => {
    api<{ people: Person[] }>("/people/")
      .then((v) => setPeople(v.people))
      .catch(() => undefined);
  }, []);
  const ask = async (value = prompt) => {
    if (!value.trim() || busy) return;
    setError("");
    setMessages((m) => [
      ...m,
      { role: "user", content: value },
      { role: "assistant", content: "" },
    ]);
    setPrompt("");
    setBusy(true);
    abort.current = new AbortController();
    try {
      await streamAI(
        { prompt: value, person_id: personId || undefined },
        (delta) =>
          setMessages((m) => {
            const copy = [...m];
            copy[copy.length - 1] = {
              ...copy[copy.length - 1],
              content: copy[copy.length - 1].content + delta,
            };
            return copy;
          }),
        abort.current.signal,
      );
    } catch (e) {
      if ((e as Error).name !== "AbortError")
        setError(e instanceof Error ? e.message : "AI 응답을 받지 못했습니다.");
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <PageHeader
        title="Orbit AI"
        description="관계를 판단하지 않고, 내가 남긴 기록 안에서 기억을 찾아드립니다."
      />
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr", lg: "minmax(0,1fr) 310px" },
          gap: 3,
        }}
      >
        <Card
          sx={{
            minHeight: "calc(100vh - 190px)",
            display: "flex",
            flexDirection: "column",
          }}
        >
          <CardContent
            sx={{
              flex: 1,
              p: { xs: 2, sm: 3 },
              display: "flex",
              flexDirection: "column",
            }}
          >
            {error && (
              <Alert severity="error" sx={{ mb: 2 }}>
                {error}
              </Alert>
            )}
            <Box
              sx={{
                flex: 1,
                overflowY: "auto",
                maxHeight: "calc(100vh - 390px)",
                minHeight: 320,
                pr: 1,
              }}
            >
              {messages.length === 0 ? (
                <Box
                  sx={{
                    height: "100%",
                    display: "grid",
                    placeItems: "center",
                    textAlign: "center",
                    py: 7,
                  }}
                >
                  <Box>
                    <AutoAwesomeRoundedIcon
                      color="primary"
                      sx={{
                        fontSize: 48,
                        filter: "drop-shadow(0 0 16px rgba(169,155,248,.45))",
                      }}
                    />
                    <Typography variant="h2" sx={{ mt: 2 }}>
                      무엇을 다시 기억해볼까요?
                    </Typography>
                    <Typography
                      color="text.secondary"
                      sx={{ mt: 1, maxWidth: 500 }}
                    >
                      Orbit AI는 저장된 관계와 기억을 근거로 답하고, 기록이
                      부족하면 솔직하게 모른다고 말합니다.
                    </Typography>
                    <Box
                      sx={{
                        display: "flex",
                        gap: 1,
                        justifyContent: "center",
                        flexWrap: "wrap",
                        mt: 3,
                      }}
                    >
                      {suggestions.map((v) => (
                        <Chip
                          key={v}
                          clickable
                          label={v}
                          onClick={() => void ask(v)}
                        />
                      ))}
                    </Box>
                  </Box>
                </Box>
              ) : (
                messages.map((message, index) => (
                  <Box
                    key={index}
                    sx={{
                      display: "flex",
                      justifyContent:
                        message.role === "user" ? "flex-end" : "flex-start",
                      gap: 1.2,
                      mb: 2,
                    }}
                  >
                    {message.role === "assistant" && (
                      <Avatar
                        sx={{ width: 34, height: 34, bgcolor: "primary.dark" }}
                      >
                        <AutoAwesomeRoundedIcon fontSize="small" />
                      </Avatar>
                    )}
                    <Box
                      sx={{
                        maxWidth: "82%",
                        px: 2,
                        py: 1.3,
                        borderRadius: 3,
                        bgcolor:
                          message.role === "user"
                            ? "primary.dark"
                            : "rgba(255,255,255,.055)",
                        whiteSpace: "pre-wrap",
                      }}
                    >
                      {message.content || (
                        <Box
                          sx={{ display: "flex", alignItems: "center", gap: 1 }}
                        >
                          <CircularProgress size={16} />
                          기억을 찾는 중…
                        </Box>
                      )}
                    </Box>
                  </Box>
                ))
              )}
            </Box>
            <Box
              sx={{ display: "flex", gap: 1, mt: 2, alignItems: "flex-end" }}
            >
              <TextField
                fullWidth
                multiline
                maxRows={5}
                placeholder="관계와 기억에 대해 물어보세요"
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && !e.shiftKey) {
                    e.preventDefault();
                    void ask();
                  }
                }}
              />
              {busy ? (
                <Button
                  variant="outlined"
                  color="warning"
                  onClick={() => abort.current?.abort()}
                  startIcon={<StopCircleOutlinedIcon />}
                >
                  중지
                </Button>
              ) : (
                <Button
                  variant="contained"
                  disabled={!prompt.trim()}
                  onClick={() => void ask()}
                  startIcon={<SendRoundedIcon />}
                >
                  질문
                </Button>
              )}
            </Box>
          </CardContent>
        </Card>
        <Box>
          <Card>
            <CardContent>
              <Typography variant="h3">질문 범위</Typography>
              <TextField
                select
                fullWidth
                label="관계 인물"
                value={personId}
                onChange={(e) => setPersonId(e.target.value)}
                sx={{ mt: 2 }}
              >
                <MenuItem value="">전체 Orbit</MenuItem>
                {people.map((p) => (
                  <MenuItem key={p.id} value={p.id}>
                    {p.display_name}
                  </MenuItem>
                ))}
              </TextField>
              <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
                특정 인물을 선택하면 그 사람의 승인된 기억을 중심으로 답합니다.
              </Typography>
            </CardContent>
          </Card>
          <Alert severity="info" sx={{ mt: 2 }}>
            응답은 기본적으로 실시간 스트리밍됩니다. 최대 출력 토큰은 관리자가
            262,144까지 설정할 수 있습니다.
          </Alert>
        </Box>
      </Box>
    </>
  );
}
