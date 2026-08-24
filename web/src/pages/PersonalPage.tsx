import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  MenuItem,
  Slider,
  Tab,
  Tabs,
  TextField,
  Typography,
  useColorScheme,
} from "@mui/material";
import DownloadRoundedIcon from "@mui/icons-material/DownloadRounded";
import AutorenewRoundedIcon from "@mui/icons-material/AutorenewRounded";
import KeyRoundedIcon from "@mui/icons-material/KeyRounded";
import ContentCopyRoundedIcon from "@mui/icons-material/ContentCopyRounded";
import DeleteOutlineRoundedIcon from "@mui/icons-material/DeleteOutlineRounded";
import { api, formatDate } from "../api";
import { PageHeader } from "../components/PageHeader";
import { LoadingView } from "../components/StateViews";

interface Preferences {
  theme: "dark" | "light" | "system";
  locale: string;
  font_scale: number;
  reduce_motion: boolean;
  rediscover_frequency: "off" | "daily" | "weekly";
  updated_at: string;
}
interface DataKey {
  id: string;
  version: number;
  status: string;
  created_at: string;
  retired_at?: string;
}
interface APIKey {
  id: string;
  name: string;
  prefix: string;
  scopes: string[];
  expires_at?: string;
  last_used_at?: string;
  revoked_at?: string;
  created_at: string;
}
const scopes = [
  ["people:read", "관계 조회"],
  ["people:write", "관계 변경"],
  ["memories:read", "기억 조회"],
  ["memories:write", "기억 생성"],
  ["orbit:read", "Orbit 조회"],
  ["ai:invoke", "AI 호출"],
  ["mcp:use", "MCP 사용"],
];
export function PersonalPage() {
  const [tab, setTab] = useState(0);
  return (
    <>
      <PageHeader
        title="개인화"
        description="나만의 Orbit 경험과 개인 데이터 접근 키를 관리합니다."
      />
      <Tabs
        value={tab}
        onChange={(_, v) => setTab(v)}
        sx={{ mb: 3, borderBottom: "1px solid", borderColor: "divider" }}
      >
        <Tab label="화면과 경험" />
        <Tab label="암호화 키" />
        <Tab label="API · MCP 키" />
      </Tabs>
      {tab === 0 ? (
        <PreferencePanel />
      ) : tab === 1 ? (
        <EncryptionPanel />
      ) : (
        <APIKeyPanel />
      )}
    </>
  );
}

function PreferencePanel() {
  const { setMode } = useColorScheme();
  const [value, setValue] = useState<Preferences>();
  const [message, setMessage] = useState("");
  useEffect(() => {
    api<{ preferences: Preferences }>("/personal/preferences").then((v) =>
      setValue(v.preferences),
    );
  }, []);
  if (!value) return <LoadingView />;
  const save = async () => {
    await api("/personal/preferences", {
      method: "PUT",
      body: JSON.stringify(value),
    });
    setMode(value.theme);
    document.documentElement.style.fontSize = `${16 * value.font_scale}px`;
    setMessage("개인화 설정을 저장했습니다.");
  };
  return (
    <Card>
      <CardContent sx={{ p: { xs: 3, sm: 4 }, maxWidth: 760 }}>
        {message && (
          <Alert severity="success" sx={{ mb: 2 }}>
            {message}
          </Alert>
        )}
        <Typography variant="h2">화면과 경험</Typography>
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" },
            gap: 2,
            mt: 3,
          }}
        >
          <TextField
            select
            label="화면 테마"
            value={value.theme}
            onChange={(e) =>
              setValue({
                ...value,
                theme: e.target.value as Preferences["theme"],
              })
            }
          >
            <MenuItem value="dark">어두운 우주</MenuItem>
            <MenuItem value="light">밝은 우주</MenuItem>
            <MenuItem value="system">시스템 설정</MenuItem>
          </TextField>
          <TextField
            select
            label="Rediscover"
            value={value.rediscover_frequency}
            onChange={(e) =>
              setValue({
                ...value,
                rediscover_frequency: e.target
                  .value as Preferences["rediscover_frequency"],
              })
            }
          >
            <MenuItem value="off">사용 안 함</MenuItem>
            <MenuItem value="daily">매일 한 번</MenuItem>
            <MenuItem value="weekly">일주일에 한 번</MenuItem>
          </TextField>
          <Box sx={{ gridColumn: "1/-1", mt: 1 }}>
            <Typography gutterBottom>
              글자 크기 · {Math.round(value.font_scale * 100)}%
            </Typography>
            <Slider
              min={0.9}
              max={1.4}
              step={0.05}
              marks={[
                { value: 0.9, label: "90%" },
                { value: 1.15, label: "115%" },
                { value: 1.4, label: "140%" },
              ]}
              value={value.font_scale}
              onChange={(_, v) =>
                setValue({ ...value, font_scale: v as number })
              }
            />
          </Box>
          <FormControlLabel
            sx={{ gridColumn: "1/-1" }}
            control={
              <Checkbox
                checked={value.reduce_motion}
                onChange={(e) =>
                  setValue({ ...value, reduce_motion: e.target.checked })
                }
              />
            }
            label="움직임 줄이기 (Orbit 애니메이션 최소화)"
          />
        </Box>
        <Button variant="contained" onClick={save} sx={{ mt: 3 }}>
          개인화 저장
        </Button>
      </CardContent>
    </Card>
  );
}

function EncryptionPanel() {
  const [data, setData] = useState<{
    keys: DataKey[];
    policy: { rotation_days: number; allow_user_rotation: boolean };
  }>();
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const load = useCallback(
    () => api<typeof data>("/personal/keys").then(setData),
    [],
  );
  useEffect(() => {
    void load();
  }, [load]);
  if (!data) return <LoadingView />;
  const rotate = async () => {
    setBusy(true);
    try {
      const result = await api<{
        version: number;
        reencrypted: Record<string, number>;
      }>("/personal/keys/rotate", { method: "POST", body: "{}" });
      setMessage(
        `키 v${result.version}으로 회전하고 기존 데이터를 다시 암호화했습니다.`,
      );
      setConfirm(false);
      await load();
    } finally {
      setBusy(false);
    }
  };
  return (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: { xs: "1fr", lg: "minmax(0,1fr) 330px" },
        gap: 3,
      }}
    >
      <Card>
        <CardContent sx={{ p: 3 }}>
          {message && (
            <Alert severity="success" sx={{ mb: 2 }}>
              {message}
            </Alert>
          )}
          <Box
            sx={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: { xs: "flex-start", sm: "center" },
              gap: 2,
              flexDirection: { xs: "column", sm: "row" },
              mb: 3,
              pb: 3,
              borderBottom: "1px solid",
              borderColor: "divider",
            }}
          >
            <Box>
              <Typography variant="h2">내 기록 내보내기</Typography>
              <Typography color="text.secondary" sx={{ mt: 0.5 }}>
                사람, 교류, 기억, 연결을 읽을 수 있는 JSON 한 파일로 내려받습니다.
                암호화된 내용도 복호화해 담기므로 안전한 곳에 보관하세요.
              </Typography>
            </Box>
            {/* 서버가 파일로 내려주므로 브라우저에 맡긴다. 같은 출처라 세션이 그대로 실린다. */}
            <Button
              component="a"
              href="/api/v1/personal/export"
              variant="outlined"
              startIcon={<DownloadRoundedIcon />}
              sx={{ flexShrink: 0 }}
            >
              내보내기
            </Button>
          </Box>
          <Box
            sx={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
              gap: 2,
            }}
          >
            <Box>
              <Typography variant="h2">개인 데이터 키</Typography>
              <Typography color="text.secondary" sx={{ mt: 0.5 }}>
                사람·연락처·기억은 사용자별 키로 암호화됩니다.
              </Typography>
            </Box>
            <Button
              variant="contained"
              startIcon={<AutorenewRoundedIcon />}
              disabled={!data.policy.allow_user_rotation}
              onClick={() => setConfirm(true)}
            >
              키 회전
            </Button>
          </Box>
          <Box sx={{ mt: 3, display: "grid", gap: 1.2 }}>
            {data.keys.map((key) => (
              <Box
                key={key.id}
                sx={{
                  display: "flex",
                  alignItems: "center",
                  gap: 2,
                  p: 2,
                  border: "1px solid",
                  borderColor: "divider",
                  borderRadius: 3,
                }}
              >
                <KeyRoundedIcon
                  color={key.status === "active" ? "primary" : "disabled"}
                />
                <Box sx={{ flex: 1 }}>
                  <Typography sx={{ fontWeight: 720 }}>
                    암호화 키 v{key.version}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    생성 {formatDate(key.created_at, true)}
                    {key.retired_at &&
                      ` · 종료 ${formatDate(key.retired_at, true)}`}
                  </Typography>
                </Box>
                <Chip
                  size="small"
                  color={key.status === "active" ? "success" : "default"}
                  label={key.status === "active" ? "사용 중" : "보관됨"}
                />
              </Box>
            ))}
          </Box>
        </CardContent>
      </Card>
      <Alert severity="info">
        Orbit은 회전 시 현재 데이터 전체를 새 키로 트랜잭션 안에서
        재암호화합니다. 실패하면 이전 상태로 완전히 되돌아갑니다.
        <br />
        <br />
        관리자가 설정한 권장 회전 주기: {data.policy.rotation_days}일
      </Alert>
      <Dialog open={confirm} onClose={() => setConfirm(false)}>
        <DialogTitle>개인 암호화 키를 회전할까요?</DialogTitle>
        <DialogContent>
          <Typography color="text.secondary">
            새 키를 만들고 현재 사람, 교류, 기억 데이터를 모두 재암호화합니다.
            데이터 양에 따라 시간이 걸릴 수 있습니다.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirm(false)}>취소</Button>
          <Button variant="contained" disabled={busy} onClick={rotate}>
            {busy ? "재암호화 중…" : "회전 시작"}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}

function APIKeyPanel() {
  const [keys, setKeys] = useState<APIKey[]>();
  const [open, setOpen] = useState(false);
  const [created, setCreated] = useState("");
  const load = useCallback(
    () =>
      api<{ api_keys: APIKey[] }>("/personal/api-keys").then((v) =>
        setKeys(v.api_keys),
      ),
    [],
  );
  useEffect(() => {
    void load();
  }, [load]);
  if (!keys) return <LoadingView />;
  return (
    <>
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr", lg: "minmax(0,1fr) 340px" },
          gap: 3,
        }}
      >
        <Card>
          <CardContent sx={{ p: 3 }}>
            <Box
              sx={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                gap: 2,
              }}
            >
              <Box>
                <Typography variant="h2">개인 API 키</Typography>
                <Typography color="text.secondary" sx={{ mt: 0.5 }}>
                  REST API와 MCP 접근 권한을 최소 범위로 발급합니다.
                </Typography>
              </Box>
              <Button
                variant="contained"
                startIcon={<KeyRoundedIcon />}
                onClick={() => setOpen(true)}
              >
                새 키
              </Button>
            </Box>
            <Box sx={{ mt: 3, display: "grid", gap: 1.2 }}>
              {keys.length === 0 ? (
                <Alert severity="info">아직 발급한 키가 없습니다.</Alert>
              ) : (
                keys.map((key) => (
                  <Box
                    key={key.id}
                    sx={{
                      p: 2,
                      border: "1px solid",
                      borderColor: "divider",
                      borderRadius: 3,
                      opacity: key.revoked_at ? 0.7 : 1,
                    }}
                  >
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Typography sx={{ flex: 1, fontWeight: 720 }}>
                        {key.name}
                      </Typography>
                      <Typography
                        variant="body2"
                        sx={{ fontFamily: "monospace" }}
                      >
                        {key.prefix}…
                      </Typography>
                      {!key.revoked_at && (
                        <Button
                          color="warning"
                          size="small"
                          startIcon={<DeleteOutlineRoundedIcon />}
                          onClick={async () => {
                            await api(`/personal/api-keys/${key.id}`, {
                              method: "DELETE",
                            });
                            await load();
                          }}
                        >
                          폐기
                        </Button>
                      )}
                    </Box>
                    <Box
                      sx={{
                        display: "flex",
                        gap: 0.6,
                        flexWrap: "wrap",
                        mt: 1.3,
                      }}
                    >
                      {key.scopes.map((v) => (
                        <Chip
                          key={v}
                          size="small"
                          variant="outlined"
                          label={v}
                        />
                      ))}
                    </Box>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ display: "block", mt: 1.2 }}
                    >
                      생성 {formatDate(key.created_at, true)} · 최근 사용{" "}
                      {formatDate(key.last_used_at, true)}
                    </Typography>
                  </Box>
                ))
              )}
            </Box>
          </CardContent>
        </Card>
        <Card>
          <CardContent>
            <Typography variant="h3">MCP 연결</Typography>
            <Typography color="text.secondary" sx={{ mt: 1 }}>
              Streamable HTTP 엔드포인트
            </Typography>
            <Box
              component="code"
              sx={{
                display: "block",
                p: 1.5,
                bgcolor: "rgba(0,0,0,.25)",
                borderRadius: 2,
                mt: 1.5,
                wordBreak: "break-all",
              }}
            >
              {location.origin}/mcp
            </Box>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
              Authorization 헤더에 `mcp:use` 권한이 있는 API 키를 Bearer
              토큰으로 전달하세요.
            </Typography>
            <Button
              component="a"
              href="/openapi.json"
              target="_blank"
              variant="outlined"
              fullWidth
              sx={{ mt: 2 }}
            >
              OpenAPI 문서 열기
            </Button>
          </CardContent>
        </Card>
      </Box>
      <NewKeyDialog
        open={open}
        close={() => setOpen(false)}
        created={(token) => {
          setCreated(token);
          void load();
          setOpen(false);
        }}
      />
      {created && (
        <Dialog open onClose={() => setCreated("")} fullWidth maxWidth="sm">
          <DialogTitle>API 키가 생성되었습니다</DialogTitle>
          <DialogContent>
            <Alert severity="warning">
              이 키는 지금 한 번만 표시됩니다. 안전한 곳에 보관하세요.
            </Alert>
            <Box
              sx={{
                display: "flex",
                gap: 1,
                alignItems: "center",
                p: 2,
                bgcolor: "rgba(0,0,0,.25)",
                borderRadius: 2,
                mt: 2,
              }}
            >
              <Typography
                component="code"
                sx={{
                  fontFamily: "monospace",
                  wordBreak: "break-all",
                  flex: 1,
                }}
              >
                {created}
              </Typography>
              <Button
                startIcon={<ContentCopyRoundedIcon />}
                onClick={() => navigator.clipboard.writeText(created)}
              >
                복사
              </Button>
            </Box>
          </DialogContent>
          <DialogActions>
            <Button variant="contained" onClick={() => setCreated("")}>
              확인
            </Button>
          </DialogActions>
        </Dialog>
      )}
    </>
  );
}

function NewKeyDialog({
  open,
  close,
  created,
}: {
  open: boolean;
  close: () => void;
  created: (token: string) => void;
}) {
  const [name, setName] = useState("");
  const [selected, setSelected] = useState<string[]>([
    "people:read",
    "memories:read",
    "orbit:read",
  ]);
  const [error, setError] = useState("");
  const save = async () => {
    try {
      const result = await api<{ token: string }>("/personal/api-keys", {
        method: "POST",
        body: JSON.stringify({ name, scopes: selected, expires_at: null }),
      });
      created(result.token);
      setName("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "키를 만들지 못했습니다.");
    }
  };
  return (
    <Dialog open={open} onClose={close} fullWidth maxWidth="sm">
      <DialogTitle>새 API · MCP 키</DialogTitle>
      <DialogContent>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        <TextField
          fullWidth
          label="키 이름"
          placeholder="예: Claude Desktop"
          value={name}
          onChange={(e) => setName(e.target.value)}
          sx={{ mt: 1 }}
        />
        <Typography sx={{ mt: 3, mb: 1, fontWeight: 700 }}>
          허용할 권한
        </Typography>
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" },
          }}
        >
          {scopes.map(([value, label]) => (
            <FormControlLabel
              key={value}
              control={
                <Checkbox
                  checked={selected.includes(value)}
                  onChange={(e) =>
                    setSelected(
                      e.target.checked
                        ? [...selected, value]
                        : selected.filter((v) => v !== value),
                    )
                  }
                />
              }
              label={`${label} (${value})`}
            />
          ))}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={close}>취소</Button>
        <Button
          variant="contained"
          disabled={!name || selected.length === 0}
          onClick={save}
        >
          키 발급
        </Button>
      </DialogActions>
    </Dialog>
  );
}
