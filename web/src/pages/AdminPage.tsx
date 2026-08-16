import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardActionArea,
  CardContent,
  Checkbox,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  MenuItem,
  Switch,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@mui/material";
import SaveRoundedIcon from "@mui/icons-material/SaveRounded";
import AddRoundedIcon from "@mui/icons-material/AddRounded";
import AdminPanelSettingsRoundedIcon from "@mui/icons-material/AdminPanelSettingsRounded";
import { api, formatDate } from "../api";
import { useAuth } from "../AuthContext";
import { PageHeader } from "../components/PageHeader";
import { ErrorView, LoadingView } from "../components/StateViews";
import type { User } from "../types";

interface AdminSettings {
  system: { service_name: string; public_url: string; session_hours: number };
  auth: {
    oidc: {
      enabled: boolean;
      issuer_url: string;
      client_id: string;
      client_secret?: string;
      clear_client_secret?: boolean;
      display_name: string;
      auto_provision: boolean;
      default_role: string;
    };
    has_client_secret: boolean;
  };
  ai: {
    provider: {
      enabled: boolean;
      provider: string;
      base_url: string;
      api_key?: string;
      clear_api_key?: boolean;
      model: string;
      max_output_tokens: number;
      request_timeout_seconds: number;
      system_prompt: string;
    };
    has_api_key: boolean;
  };
  workflow: {
    enabled: boolean;
    resource_types: string[];
    reviewer_role: string;
  };
  security: {
    rotation_days: number;
    allow_user_rotation: boolean;
    default_scopes: string[];
  };
}
const tabs = [
  "일반",
  "Keycloak SSO",
  "AI",
  "승인 프로세스",
  "사용자",
  "키 권한",
  "감사 로그",
];
export function AdminPage() {
  const { user } = useAuth();
  const [tab, setTab] = useState(0);
  const [settings, setSettings] = useState<AdminSettings>();
  const [error, setError] = useState("");
  const load = useCallback(async () => {
    try {
      const data = await api<{ settings: AdminSettings }>("/admin/settings");
      setSettings(data.settings);
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "관리자 설정을 불러오지 못했습니다.",
      );
    }
  }, []);
  useEffect(() => {
    if (user?.role === "admin") void load();
  }, [load, user]);
  if (user?.role !== "admin")
    return <Alert severity="error">서비스 관리자만 접근할 수 있습니다.</Alert>;
  if (error) return <ErrorView message={error} retry={load} />;
  if (!settings) return <LoadingView label="관리 설정을 불러오는 중…" />;
  return (
    <>
      <PageHeader
        title="서비스 관리"
        description="Orbit 전체의 인증, AI, 보안과 운영 정책을 한곳에서 관리합니다."
        action={
          <Chip
            icon={<AdminPanelSettingsRoundedIcon />}
            label="Service Admin"
            color="primary"
            variant="outlined"
          />
        }
      />
      <Box
        sx={{
          borderBottom: "1px solid",
          borderColor: "divider",
          mb: 3,
          overflowX: "auto",
        }}
      >
        <Tabs
          value={tab}
          onChange={(_, v) => setTab(v)}
          variant="scrollable"
          scrollButtons="auto"
        >
          {tabs.map((label) => (
            <Tab key={label} label={label} />
          ))}
        </Tabs>
      </Box>
      {tab === 0 ? (
        <GeneralSettings
          value={settings.system}
          changed={(v) => setSettings({ ...settings, system: v })}
        />
      ) : tab === 1 ? (
        <OIDCSettings
          value={settings.auth}
          changed={(v) => setSettings({ ...settings, auth: v })}
        />
      ) : tab === 2 ? (
        <AISettingsPanel
          value={settings.ai}
          changed={(v) => setSettings({ ...settings, ai: v })}
        />
      ) : tab === 3 ? (
        <WorkflowSettings
          value={settings.workflow}
          changed={(v) => setSettings({ ...settings, workflow: v })}
        />
      ) : tab === 4 ? (
        <UsersPanel />
      ) : tab === 5 ? (
        <SecurityPanel
          value={settings.security}
          changed={(v) => setSettings({ ...settings, security: v })}
        />
      ) : (
        <AuditPanel />
      )}
    </>
  );
}

function Panel({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <Card>
      <CardContent sx={{ p: { xs: 3, sm: 4 }, maxWidth: 900 }}>
        <Typography variant="h2">{title}</Typography>
        <Typography color="text.secondary" sx={{ mt: 0.7, mb: 3 }}>
          {description}
        </Typography>
        {children}
      </CardContent>
    </Card>
  );
}
function SaveNotice({ message }: { message: string }) {
  return message ? (
    <Alert severity="success" sx={{ mb: 2 }}>
      {message}
    </Alert>
  ) : null;
}

function GeneralSettings({
  value,
  changed,
}: {
  value: AdminSettings["system"];
  changed: (v: AdminSettings["system"]) => void;
}) {
  const [message, setMessage] = useState("");
  const save = async () => {
    await api("/admin/settings/system", {
      method: "PUT",
      body: JSON.stringify(value),
    });
    setMessage("일반 설정을 저장했습니다.");
  };
  return (
    <Panel
      title="일반 설정"
      description="서비스 이름, 외부 URL과 세션 수명을 관리합니다. 런타임 환경변수 없이 즉시 적용됩니다."
    >
      <SaveNotice message={message} />
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" },
          gap: 2,
        }}
      >
        <TextField
          label="서비스 이름"
          value={value.service_name}
          onChange={(e) => changed({ ...value, service_name: e.target.value })}
        />
        <TextField
          label="세션 유지 시간"
          type="number"
          value={value.session_hours}
          onChange={(e) =>
            changed({ ...value, session_hours: +e.target.value })
          }
          helperText="1~720시간"
        />
        <TextField
          sx={{ gridColumn: "1/-1" }}
          label="서비스 공개 URL"
          value={value.public_url}
          onChange={(e) => changed({ ...value, public_url: e.target.value })}
          helperText="OIDC Callback URL과 보안 쿠키 판단에 사용됩니다."
        />
      </Box>
      <Button
        variant="contained"
        startIcon={<SaveRoundedIcon />}
        onClick={save}
        sx={{ mt: 3 }}
      >
        설정 저장
      </Button>
    </Panel>
  );
}

function OIDCSettings({
  value,
  changed,
}: {
  value: AdminSettings["auth"];
  changed: (v: AdminSettings["auth"]) => void;
}) {
  const v = value.oidc;
  const [message, setMessage] = useState("");
  const update = (patch: Partial<typeof v>) =>
    changed({ ...value, oidc: { ...v, ...patch } });
  const save = async () => {
    await api("/admin/settings/auth", {
      method: "PUT",
      body: JSON.stringify(v),
    });
    setMessage("Keycloak OIDC 설정을 저장했습니다.");
  };
  return (
    <Panel
      title="Keycloak SSO · OIDC"
      description="Issuer URL, Client ID, Client Secret만 입력하면 Discovery와 토큰 검증이 자동 구성됩니다."
    >
      <SaveNotice message={message} />
      <FormControlLabel
        control={
          <Switch
            checked={v.enabled}
            onChange={(e) => update({ enabled: e.target.checked })}
          />
        }
        label="Keycloak SSO 사용"
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
          sx={{ gridColumn: "1/-1" }}
          label="Issuer URL"
          placeholder="https://keycloak.internal/realms/orbit"
          value={v.issuer_url}
          onChange={(e) => update({ issuer_url: e.target.value })}
        />
        <TextField
          label="Client ID"
          value={v.client_id}
          onChange={(e) => update({ client_id: e.target.value })}
        />
        <TextField
          type="password"
          label="Client Secret"
          value={v.client_secret ?? ""}
          onChange={(e) => update({ client_secret: e.target.value })}
          placeholder={
            value.has_client_secret ? "저장된 Secret 유지" : "Secret 입력"
          }
          helperText={
            value.has_client_secret ? "비워두면 기존 값이 유지됩니다." : ""
          }
        />
        {value.has_client_secret && (
          <FormControlLabel
            sx={{ gridColumn: "1/-1" }}
            control={
              <Checkbox
                checked={Boolean(v.clear_client_secret)}
                onChange={(e) =>
                  update({ clear_client_secret: e.target.checked })
                }
              />
            }
            label="저장된 Client Secret 제거"
          />
        )}
        <TextField
          label="로그인 버튼 이름"
          value={v.display_name}
          onChange={(e) => update({ display_name: e.target.value })}
        />
        <TextField
          select
          label="신규 사용자 기본 역할"
          value={v.default_role}
          onChange={(e) => update({ default_role: e.target.value })}
        >
          <MenuItem value="member">멤버</MenuItem>
          <MenuItem value="team_lead">팀장</MenuItem>
        </TextField>
        <FormControlLabel
          sx={{ gridColumn: "1/-1" }}
          control={
            <Checkbox
              checked={v.auto_provision}
              onChange={(e) => update({ auto_provision: e.target.checked })}
            />
          }
          label="첫 SSO 로그인 시 사용자를 자동 등록"
        />
      </Box>
      <Alert severity="info" sx={{ mt: 2 }}>
        Keycloak Valid Redirect URI:{" "}
        <strong>{location.origin}/api/v1/auth/oidc/callback</strong>
      </Alert>
      <Button
        variant="contained"
        startIcon={<SaveRoundedIcon />}
        onClick={save}
        sx={{ mt: 3 }}
      >
        OIDC 저장
      </Button>
    </Panel>
  );
}

function AISettingsPanel({
  value,
  changed,
}: {
  value: AdminSettings["ai"];
  changed: (v: AdminSettings["ai"]) => void;
}) {
  const v = value.provider;
  const [message, setMessage] = useState("");
  const update = (patch: Partial<typeof v>) =>
    changed({ ...value, provider: { ...v, ...patch } });
  const save = async () => {
    await api("/admin/settings/ai", { method: "PUT", body: JSON.stringify(v) });
    setMessage("AI 설정을 저장했습니다.");
  };
  return (
    <Panel
      title="AI Gateway"
      description="OpenAI Responses API 호환 엔드포인트 또는 오프라인망의 로컬 모델을 스트리밍 방식으로 연결합니다."
    >
      <SaveNotice message={message} />
      <FormControlLabel
        control={
          <Switch
            checked={v.enabled}
            onChange={(e) => update({ enabled: e.target.checked })}
          />
        }
        label="Orbit AI 사용"
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
          select
          label="Provider"
          value={v.provider}
          onChange={(e) => update({ provider: e.target.value })}
        >
          <MenuItem value="openai-compatible">OpenAI Responses 호환</MenuItem>
          <MenuItem value="local">Local LLM (Responses 호환)</MenuItem>
        </TextField>
        <TextField
          label="모델"
          placeholder="gpt-5 또는 로컬 모델 ID"
          value={v.model}
          onChange={(e) => update({ model: e.target.value })}
        />
        <TextField
          sx={{ gridColumn: "1/-1" }}
          label="Base URL"
          placeholder="https://api.openai.com 또는 http://llm.internal:8000"
          value={v.base_url}
          onChange={(e) => update({ base_url: e.target.value })}
          helperText="/v1/responses 경로는 자동으로 붙습니다."
        />
        <TextField
          type="password"
          label="API Key"
          value={v.api_key ?? ""}
          onChange={(e) => update({ api_key: e.target.value })}
          placeholder={
            value.has_api_key ? "저장된 키 유지" : "API 키 (로컬 모델은 선택)"
          }
          helperText={value.has_api_key ? "비워두면 기존 값을 유지합니다." : ""}
        />
        {value.has_api_key && (
          <FormControlLabel
            control={
              <Checkbox
                checked={Boolean(v.clear_api_key)}
                onChange={(e) => update({ clear_api_key: e.target.checked })}
              />
            }
            label="저장된 API Key 제거"
          />
        )}
        <TextField
          label="요청 제한 시간(초)"
          type="number"
          value={v.request_timeout_seconds}
          onChange={(e) => update({ request_timeout_seconds: +e.target.value })}
        />
        <TextField
          label="최대 출력 토큰"
          type="number"
          value={v.max_output_tokens}
          onChange={(e) => update({ max_output_tokens: +e.target.value })}
          helperText="최대 262,144 (256k)"
        />
        <TextField
          sx={{ gridColumn: "1/-1" }}
          multiline
          minRows={4}
          label="시스템 지침"
          value={v.system_prompt}
          onChange={(e) => update({ system_prompt: e.target.value })}
        />
      </Box>
      <Alert severity="warning" sx={{ mt: 2 }}>
        관계 데이터가 지정한 AI Provider로 전달될 수 있습니다. 오프라인망에서는
        내부 Responses 호환 모델 주소를 사용하세요.
      </Alert>
      <Button
        variant="contained"
        startIcon={<SaveRoundedIcon />}
        onClick={save}
        sx={{ mt: 3 }}
      >
        AI 설정 저장
      </Button>
    </Panel>
  );
}

function WorkflowSettings({
  value,
  changed,
}: {
  value: AdminSettings["workflow"];
  changed: (v: AdminSettings["workflow"]) => void;
}) {
  const [message, setMessage] = useState("");
  const save = async () => {
    await api("/admin/settings/workflow", {
      method: "PUT",
      body: JSON.stringify(value),
    });
    setMessage(
      value.enabled
        ? "승인 프로세스를 활성화했습니다."
        : "승인 프로세스를 제외했습니다.",
    );
  };
  return (
    <Panel
      title="팀장 검토 및 승인"
      description="필요한 경우에만 켭니다. 끄면 검토·승인·반려 단계 없이 기록이 즉시 반영됩니다."
    >
      <SaveNotice message={message} />
      <FormControlLabel
        control={
          <Switch
            checked={value.enabled}
            onChange={(e) => changed({ ...value, enabled: e.target.checked })}
          />
        }
        label="검토 및 승인 프로세스 사용"
      />
      <Divider sx={{ my: 2 }} />
      <FormControlLabel
        control={
          <Checkbox
            disabled={!value.enabled}
            checked={value.resource_types.includes("memory")}
            onChange={(e) =>
              changed({
                ...value,
                resource_types: e.target.checked ? ["memory"] : [],
              })
            }
          />
        }
        label="관계 기억 생성 시 검토"
      />
      <TextField
        select
        fullWidth
        disabled={!value.enabled}
        label="검토자 역할"
        value={value.reviewer_role}
        onChange={(e) => changed({ ...value, reviewer_role: e.target.value })}
        sx={{ maxWidth: 380, display: "block", mt: 2 }}
      >
        <MenuItem value="team_lead">팀장 및 관리자</MenuItem>
        <MenuItem value="admin">관리자</MenuItem>
      </TextField>
      <Alert severity={value.enabled ? "info" : "success"} sx={{ mt: 2 }}>
        {value.enabled
          ? "새 기억은 검토 대기 상태가 되고 팀장/관리자가 승인 또는 반려합니다."
          : "워크플로가 비활성 상태이므로 관련 단계가 서비스 흐름에서 제외됩니다."}
      </Alert>
      <Button
        variant="contained"
        startIcon={<SaveRoundedIcon />}
        onClick={save}
        sx={{ mt: 3 }}
      >
        프로세스 저장
      </Button>
    </Panel>
  );
}

function UsersPanel() {
  const [users, setUsers] = useState<User[]>();
  const [editing, setEditing] = useState<User | null>();
  const [creating, setCreating] = useState(false);
  const load = useCallback(
    () => api<{ users: User[] }>("/admin/users").then((v) => setUsers(v.users)),
    [],
  );
  useEffect(() => {
    void load();
  }, [load]);
  if (!users) return <LoadingView />;
  return (
    <Card>
      <CardContent sx={{ p: { xs: 2, sm: 3 } }}>
        <Box
          sx={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
            px: 1,
            mb: 2,
          }}
        >
          <Box>
            <Typography variant="h2">사용자</Typography>
            <Typography color="text.secondary">
              역할과 계정 상태를 관리합니다.
            </Typography>
          </Box>
          <Button
            variant="contained"
            startIcon={<AddRoundedIcon />}
            onClick={() => setCreating(true)}
          >
            사용자 추가
          </Button>
        </Box>
        <Box
          aria-label="관리자 사용자 메뉴"
          sx={{
            maxHeight: "58vh",
            overflowY: "auto",
            pr: 1,
            display: "grid",
            gap: 1,
            "&::-webkit-scrollbar": { width: 10 },
            "&::-webkit-scrollbar-thumb": {
              background: "linear-gradient(#9588d6,#564e78)",
              border: "2px solid #15172a",
              borderRadius: 10,
            },
          }}
        >
          {users.map((user) => (
            <CardActionArea
              key={user.id}
              onClick={() => setEditing(user)}
              sx={{
                border: "1px solid",
                borderColor: "divider",
                borderRadius: 3,
                p: 2,
              }}
            >
              <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
                <Box
                  sx={{
                    width: 42,
                    height: 42,
                    borderRadius: "50%",
                    display: "grid",
                    placeItems: "center",
                    bgcolor: "primary.dark",
                    fontWeight: 800,
                  }}
                >
                  {user.display_name[0]}
                </Box>
                <Box sx={{ flex: 1, minWidth: 0 }}>
                  <Typography sx={{ fontWeight: 730 }} noWrap>
                    {user.display_name}{" "}
                    <Typography
                      component="span"
                      variant="body2"
                      color="text.secondary"
                    >
                      @{user.username}
                    </Typography>
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    마지막 로그인 {formatDate(user.last_login_at, true)}
                  </Typography>
                </Box>
                <Chip
                  size="small"
                  label={roleName(user.role)}
                  color={user.role === "admin" ? "primary" : "default"}
                />
                <Chip
                  size="small"
                  label={user.status === "active" ? "활성" : "비활성"}
                  color={user.status === "active" ? "success" : "default"}
                  variant="outlined"
                />
              </Box>
            </CardActionArea>
          ))}
        </Box>
      </CardContent>
      <UserDialog
        open={creating}
        close={() => setCreating(false)}
        saved={load}
      />
      <UserDialog
        open={Boolean(editing)}
        user={editing ?? undefined}
        close={() => setEditing(null)}
        saved={load}
      />
    </Card>
  );
}
function roleName(role: string) {
  return role === "admin" ? "관리자" : role === "team_lead" ? "팀장" : "멤버";
}
function UserDialog({
  open,
  user,
  close,
  saved,
}: {
  open: boolean;
  user?: User;
  close: () => void;
  saved: () => void;
}) {
  const [form, setForm] = useState({
    username: "",
    email: "",
    display_name: "",
    role: "member",
    status: "active",
    password: "",
  });
  const [error, setError] = useState("");
  useEffect(() => {
    if (open)
      setForm(
        user
          ? {
              username: user.username,
              email: user.email,
              display_name: user.display_name,
              role: user.role,
              status: user.status,
              password: "",
            }
          : {
              username: "",
              email: "",
              display_name: "",
              role: "member",
              status: "active",
              password: "",
            },
      );
  }, [open, user]);
  const save = async () => {
    try {
      await api(user ? `/admin/users/${user.id}` : "/admin/users", {
        method: user ? "PUT" : "POST",
        body: JSON.stringify(form),
      });
      saved();
      close();
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "사용자를 저장하지 못했습니다.",
      );
    }
  };
  return (
    <Dialog open={open} onClose={close} fullWidth maxWidth="sm">
      <DialogTitle>{user ? "사용자 수정" : "사용자 추가"}</DialogTitle>
      <DialogContent dividers>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" },
            gap: 2,
          }}
        >
          <TextField
            disabled={Boolean(user)}
            label="사용자 ID"
            value={form.username}
            onChange={(e) => setForm({ ...form, username: e.target.value })}
          />
          <TextField
            label="이름"
            value={form.display_name}
            onChange={(e) => setForm({ ...form, display_name: e.target.value })}
          />
          <TextField
            label="이메일"
            type="email"
            value={form.email}
            onChange={(e) => setForm({ ...form, email: e.target.value })}
          />
          <TextField
            select
            label="역할"
            value={form.role}
            onChange={(e) => setForm({ ...form, role: e.target.value })}
          >
            <MenuItem value="member">멤버</MenuItem>
            <MenuItem value="team_lead">팀장</MenuItem>
            <MenuItem value="admin">관리자</MenuItem>
          </TextField>
          {user && (
            <TextField
              select
              label="상태"
              value={form.status}
              onChange={(e) => setForm({ ...form, status: e.target.value })}
            >
              <MenuItem value="active">활성</MenuItem>
              <MenuItem value="disabled">비활성</MenuItem>
            </TextField>
          )}
          <TextField
            label={user ? "새 비밀번호 (선택)" : "초기 비밀번호 (선택)"}
            type="password"
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            helperText="로컬 로그인 사용 시 10자 이상"
          />
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={close}>취소</Button>
        <Button
          variant="contained"
          disabled={!form.username || !form.display_name}
          onClick={save}
        >
          저장
        </Button>
      </DialogActions>
    </Dialog>
  );
}

interface Permission {
  id: string;
  user_name: string;
  key_version: number;
  key_status: string;
  principal_type: string;
  principal_id: string;
  permissions: string[];
}
function SecurityPanel({
  value,
  changed,
}: {
  value: AdminSettings["security"];
  changed: (v: AdminSettings["security"]) => void;
}) {
  const [permissions, setPermissions] = useState<Permission[]>();
  const [message, setMessage] = useState("");
  const load = useCallback(
    () =>
      api<{ permissions: Permission[] }>("/admin/key-permissions").then((v) =>
        setPermissions(v.permissions),
      ),
    [],
  );
  useEffect(() => {
    void load();
  }, [load]);
  const savePolicy = async () => {
    await api("/admin/settings/security", {
      method: "PUT",
      body: JSON.stringify(value),
    });
    setMessage("키 정책을 저장했습니다.");
  };
  return (
    <Box sx={{ display: "grid", gap: 3 }}>
      <Panel
        title="키 정책"
        description="개인별 데이터 키의 회전 주기와 사용자 자율 권한을 정합니다."
      >
        <SaveNotice message={message} />
        <Box
          sx={{
            display: "flex",
            gap: 3,
            alignItems: "center",
            flexWrap: "wrap",
          }}
        >
          <TextField
            type="number"
            label="권장 회전 주기(일)"
            value={value.rotation_days}
            onChange={(e) =>
              changed({ ...value, rotation_days: +e.target.value })
            }
          />
          <FormControlLabel
            control={
              <Switch
                checked={value.allow_user_rotation}
                onChange={(e) =>
                  changed({ ...value, allow_user_rotation: e.target.checked })
                }
              />
            }
            label="사용자 직접 키 회전 허용"
          />
        </Box>
        <Button
          variant="contained"
          startIcon={<SaveRoundedIcon />}
          onClick={savePolicy}
          sx={{ mt: 3 }}
        >
          정책 저장
        </Button>
      </Panel>
      <Card>
        <CardContent sx={{ p: 3 }}>
          <Typography variant="h2">변경 가능한 키 권한</Typography>
          <Typography color="text.secondary" sx={{ mt: 0.5, mb: 2 }}>
            각 키 주체의 암호화·복호화·회전·위임 권한을 변경합니다.
          </Typography>
          {!permissions ? (
            <LoadingView />
          ) : (
            <Box sx={{ display: "grid", gap: 1.2 }}>
              {permissions.map((p) => (
                <Box
                  key={p.id}
                  sx={{
                    p: 2,
                    border: "1px solid",
                    borderColor: "divider",
                    borderRadius: 3,
                  }}
                >
                  <Box
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      gap: 1,
                      mb: 1,
                    }}
                  >
                    <Typography sx={{ flex: 1, fontWeight: 720 }}>
                      {p.user_name} · 키 v{p.key_version}
                    </Typography>
                    <Chip size="small" label={p.key_status} />
                    <Chip
                      size="small"
                      variant="outlined"
                      label={`${p.principal_type}:${p.principal_id.slice(0, 8)}`}
                    />
                  </Box>
                  <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
                    {["decrypt", "encrypt", "rotate", "delegate"].map(
                      (permission) => (
                        <FormControlLabel
                          key={permission}
                          control={
                            <Checkbox
                              size="small"
                              checked={p.permissions.includes(permission)}
                              onChange={async (e) => {
                                const next = e.target.checked
                                  ? [...p.permissions, permission]
                                  : p.permissions.filter(
                                      (v) => v !== permission,
                                    );
                                if (next.length === 0) return;
                                await api(`/admin/key-permissions/${p.id}`, {
                                  method: "PUT",
                                  body: JSON.stringify({ permissions: next }),
                                });
                                await load();
                              }}
                            />
                          }
                          label={permission}
                        />
                      ),
                    )}
                  </Box>
                </Box>
              ))}
            </Box>
          )}
        </CardContent>
      </Card>
    </Box>
  );
}

interface Audit {
  id: number;
  actor_name: string;
  action: string;
  resource_type: string;
  resource_id: string;
  ip_address: string;
  created_at: string;
}
function AuditPanel() {
  const [logs, setLogs] = useState<Audit[]>();
  useEffect(() => {
    api<{ audit_logs: Audit[] }>("/admin/audit").then((v) =>
      setLogs(v.audit_logs),
    );
  }, []);
  if (!logs) return <LoadingView />;
  return (
    <Card>
      <CardContent sx={{ p: 3 }}>
        <Typography variant="h2">감사 로그</Typography>
        <Typography color="text.secondary" sx={{ mt: 0.5, mb: 2 }}>
          인증, 설정, 키와 민감 데이터 변경 이력을 확인합니다.
        </Typography>
        <Box sx={{ overflowX: "auto" }}>
          <Box sx={{ minWidth: 760 }}>
            {logs.map((log) => (
              <Box
                key={log.id}
                sx={{
                  display: "grid",
                  gridTemplateColumns: "170px 140px 1fr 160px",
                  gap: 2,
                  py: 1.5,
                  borderBottom: "1px solid",
                  borderColor: "divider",
                }}
              >
                <Typography variant="body2" color="text.secondary">
                  {formatDate(log.created_at, true)}
                </Typography>
                <Typography>{log.actor_name}</Typography>
                <Box>
                  <Typography sx={{ fontWeight: 700 }}>{log.action}</Typography>
                  <Typography variant="body2" color="text.secondary">
                    {log.resource_type} · {log.resource_id.slice(0, 12)}
                  </Typography>
                </Box>
                <Typography variant="body2" color="text.secondary">
                  {log.ip_address}
                </Typography>
              </Box>
            ))}
          </Box>
        </Box>
      </CardContent>
    </Card>
  );
}
