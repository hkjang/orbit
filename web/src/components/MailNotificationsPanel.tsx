import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Chip,
  Divider,
  FormControlLabel,
  MenuItem,
  Switch,
  TextField,
  Typography,
} from "@mui/material";
import SaveRoundedIcon from "@mui/icons-material/SaveRounded";
import SendRoundedIcon from "@mui/icons-material/SendRounded";
import RefreshRoundedIcon from "@mui/icons-material/RefreshRounded";
import { api, formatDate } from "../api";
import { useAuth } from "../AuthContext";
import {
  defaultTestRecipient,
  deliveryStatus,
  mailEventLabel,
} from "../mailLabels";
import { LoadingView } from "./StateViews";

/**
 * 관리 API 의 mail 네임스페이스 필드. 이름은 사내 표준의 설정 키와 같다
 * (smtp_host 는 곧 mail.smtp_host).
 */
export interface MailSettings {
  enabled: boolean;
  smtp_host: string;
  smtp_port: number;
  security: "auto" | "none" | "starttls" | "tls";
  skip_tls_verify: boolean;
  username: string;
  password?: string;
  clear_password?: boolean;
  from_address: string;
  from_name: string;
  base_url: string;
  timeout_seconds: number;
  notify_approval_request: boolean;
  notify_approval_decided: boolean;
  notify_account_created: boolean;
}

export interface MailSettingsView {
  smtp: MailSettings;
  has_password: boolean;
}

interface Delivery {
  id: string;
  event: string;
  recipient: string;
  subject: string;
  status: string;
  attempts: number;
  error_message?: string;
  created_at: string;
  updated_at: string;
}

interface DeliveryPage {
  items: Delivery[];
  summary: { total: number; status: Record<string, number> };
}

export function MailNotificationsPanel({
  value,
  changed,
  reload,
}: {
  value: MailSettingsView;
  changed: (v: MailSettingsView) => void;
  reload: () => Promise<void>;
}) {
  const { user } = useAuth();
  const v = value.smtp;
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [recipient, setRecipient] = useState(() =>
    defaultTestRecipient(user?.email),
  );
  const [testResult, setTestResult] = useState<{
    ok: boolean;
    text: string;
  }>();
  const [sending, setSending] = useState(false);
  const [deliveriesVersion, setDeliveriesVersion] = useState(0);
  const update = (patch: Partial<MailSettings>) =>
    changed({ ...value, smtp: { ...v, ...patch } });
  const save = async () => {
    setError("");
    setMessage("");
    try {
      await api("/admin/settings/mail", {
        method: "PUT",
        body: JSON.stringify(v),
      });
      setMessage(
        v.enabled
          ? "메일 알림 설정을 저장했습니다. 시험 발송으로 릴레이를 확인하세요."
          : "메일 알림 설정을 저장했습니다. (알림은 꺼져 있습니다)",
      );
      // 비밀번호는 되읽히지 않으므로 저장 뒤 "설정됨" 표시만 다시 가져온다.
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : "설정을 저장하지 못했습니다.");
    }
  };
  const sendTest = async () => {
    setSending(true);
    setTestResult(undefined);
    try {
      await api("/admin/mail/test", {
        method: "POST",
        body: JSON.stringify({ recipient }),
      });
      setTestResult({ ok: true, text: `${recipient} 로 보냈습니다.` });
    } catch (e) {
      setTestResult({
        ok: false,
        text: e instanceof Error ? e.message : "시험 발송에 실패했습니다.",
      });
    } finally {
      setSending(false);
      setDeliveriesVersion((n) => n + 1);
    }
  };
  return (
    <Box sx={{ display: "grid", gap: 3 }}>
      <Card>
        <CardContent sx={{ p: { xs: 3, sm: 4 }, maxWidth: 900 }}>
          <Typography variant="h2">메일 알림</Typography>
          <Typography color="text.secondary" sx={{ mt: 0.7, mb: 3 }}>
            사내 SMTP 릴레이로 검토 요청·검토 결과·계정 준비 알림을 보냅니다.
            기본은 꺼짐이고, 릴레이가 응답하지 않아도 사용자의 요청은 평소처럼
            끝납니다.
          </Typography>
          {message && (
            <Alert severity="success" sx={{ mb: 2 }}>
              {message}
            </Alert>
          )}
          {error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {error}
            </Alert>
          )}
          <FormControlLabel
            control={
              <Switch
                checked={v.enabled}
                onChange={(e) => update({ enabled: e.target.checked })}
              />
            }
            label="메일 알림 사용"
          />
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: { xs: "1fr", sm: "2fr 1fr" },
              gap: 2,
              mt: 2,
            }}
          >
            <TextField
              label="SMTP 호스트"
              placeholder="relay.internal 또는 postra.internal"
              value={v.smtp_host}
              onChange={(e) => update({ smtp_host: e.target.value })}
            />
            <TextField
              label="포트"
              type="number"
              value={v.smtp_port}
              onChange={(e) => update({ smtp_port: +e.target.value })}
              helperText="사내 릴레이는 대개 25"
            />
            <TextField
              select
              label="보안"
              value={v.security}
              onChange={(e) =>
                update({ security: e.target.value as MailSettings["security"] })
              }
              helperText="auto 는 서버가 알리는 대로 STARTTLS 를 씁니다"
            >
              <MenuItem value="auto">auto — 서버가 알리는 대로</MenuItem>
              <MenuItem value="none">none — 평문</MenuItem>
              <MenuItem value="starttls">starttls — 반드시 STARTTLS</MenuItem>
              <MenuItem value="tls">tls — 처음부터 TLS (465)</MenuItem>
            </TextField>
            <FormControlLabel
              control={
                <Checkbox
                  checked={v.skip_tls_verify}
                  onChange={(e) =>
                    update({ skip_tls_verify: e.target.checked })
                  }
                />
              }
              label="인증서 검증 생략 (사설 인증서일 때만)"
            />
            <TextField
              label="사용자 이름 (선택)"
              value={v.username}
              onChange={(e) => update({ username: e.target.value })}
              helperText="인증 없는 릴레이면 비워 둡니다"
            />
            <TextField
              type="password"
              label="비밀번호 (선택)"
              value={v.password ?? ""}
              onChange={(e) => update({ password: e.target.value })}
              placeholder={value.has_password ? "설정됨 · 유지" : "비밀번호"}
              helperText={
                value.has_password
                  ? "설정됨. 비워두면 기존 값을 유지합니다."
                  : ""
              }
            />
            {value.has_password && (
              <FormControlLabel
                sx={{ gridColumn: "1/-1" }}
                control={
                  <Checkbox
                    checked={Boolean(v.clear_password)}
                    onChange={(e) =>
                      update({ clear_password: e.target.checked })
                    }
                  />
                }
                label="저장된 비밀번호 제거"
              />
            )}
            <TextField
              label="보내는 주소"
              placeholder="orbit@example.internal"
              value={v.from_address}
              onChange={(e) => update({ from_address: e.target.value })}
              helperText="비우면 orbit@<SMTP 호스트>"
            />
            <TextField
              label="보내는 이름"
              value={v.from_name}
              onChange={(e) => update({ from_name: e.target.value })}
            />
            <TextField
              label="메일 속 링크의 서비스 주소"
              placeholder="https://orbit.internal"
              value={v.base_url}
              onChange={(e) => update({ base_url: e.target.value })}
              helperText="비우면 메일에 링크를 넣지 않습니다"
            />
            <TextField
              label="제한 시간(초)"
              type="number"
              value={v.timeout_seconds}
              onChange={(e) => update({ timeout_seconds: +e.target.value })}
              helperText="1~120"
            />
          </Box>
          <Divider sx={{ my: 2 }} />
          <Typography sx={{ fontWeight: 700, mb: 0.5 }}>
            보낼 이벤트
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            자기가 한 일은 자기에게 보내지 않고, 잇달아 올라온 검토 요청은 15분
            안에 한 통으로 묶습니다.
          </Typography>
          <Box sx={{ display: "grid" }}>
            <FormControlLabel
              control={
                <Checkbox
                  checked={v.notify_approval_request}
                  onChange={(e) =>
                    update({ notify_approval_request: e.target.checked })
                  }
                />
              }
              label="검토 요청 — 기억이 검토를 기다리면 검토자에게"
            />
            <FormControlLabel
              control={
                <Checkbox
                  checked={v.notify_approval_decided}
                  onChange={(e) =>
                    update({ notify_approval_decided: e.target.checked })
                  }
                />
              }
              label="검토 결과 — 승인·반려되면 요청한 사람에게"
            />
            <FormControlLabel
              control={
                <Checkbox
                  checked={v.notify_account_created}
                  onChange={(e) =>
                    update({ notify_account_created: e.target.checked })
                  }
                />
              }
              label="계정 준비 — 관리자가 계정을 만들면 그 사람에게"
            />
          </Box>
          <Button
            variant="contained"
            startIcon={<SaveRoundedIcon />}
            onClick={save}
            sx={{ mt: 3 }}
          >
            메일 설정 저장
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardContent sx={{ p: { xs: 3, sm: 4 }, maxWidth: 900 }}>
          <Typography variant="h2">시험 발송</Typography>
          <Typography color="text.secondary" sx={{ mt: 0.7, mb: 2 }}>
            저장한 설정으로 실제 한 통을 보내고 결과를 여기서 봅니다. 릴레이
            설정은 한 번에 맞는 일이 드뭅니다.
          </Typography>
          <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
            <TextField
              size="small"
              label="받는 사람"
              value={recipient}
              onChange={(e) => setRecipient(e.target.value)}
              sx={{ flex: "1 1 260px", maxWidth: 360 }}
            />
            <Button
              variant="outlined"
              startIcon={<SendRoundedIcon />}
              onClick={sendTest}
              disabled={sending || !recipient.includes("@")}
            >
              {sending ? "보내는 중…" : "시험 발송"}
            </Button>
          </Box>
          {testResult && (
            <Alert
              severity={testResult.ok ? "success" : "error"}
              sx={{ mt: 2 }}
            >
              {testResult.text}
            </Alert>
          )}
        </CardContent>
      </Card>

      <DeliveryLog version={deliveriesVersion} />
    </Box>
  );
}

function DeliveryLog({ version }: { version: number }) {
  const [page, setPage] = useState<DeliveryPage>();
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const load = useCallback(async () => {
    try {
      const search = new URLSearchParams();
      if (status) search.set("status", status);
      setPage(await api<DeliveryPage>(`/admin/mail/deliveries?${search}`));
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "발송 기록을 불러오지 못했습니다.",
      );
    }
  }, [status]);
  useEffect(() => {
    void load();
  }, [load, version]);
  return (
    <Card>
      <CardContent sx={{ p: { xs: 3, sm: 4 } }}>
        <Box
          sx={{
            display: "flex",
            gap: 1.5,
            flexWrap: "wrap",
            alignItems: "center",
            mb: 2,
          }}
        >
          <Box sx={{ flex: "1 1 auto" }}>
            <Typography variant="h2">발송 기록</Typography>
            <Typography color="text.secondary" sx={{ mt: 0.5 }}>
              무엇이 언제 누구에게 나갔는지 남깁니다. 본문은 담지 않습니다.
            </Typography>
          </Box>
          <TextField
            select
            size="small"
            value={status}
            onChange={(e) => setStatus(e.target.value)}
            aria-label="발송 상태"
            slotProps={{ select: { displayEmpty: true } }}
            sx={{ minWidth: 160 }}
          >
            <MenuItem value="">모든 상태</MenuItem>
            <MenuItem value="sent">발송됨</MenuItem>
            <MenuItem value="failed">실패</MenuItem>
            <MenuItem value="queued">대기</MenuItem>
          </TextField>
          <Button
            size="small"
            startIcon={<RefreshRoundedIcon />}
            onClick={() => void load()}
          >
            새로고침
          </Button>
        </Box>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        {!page ? (
          <LoadingView />
        ) : (
          <>
            <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mb: 2 }}>
              <Chip size="small" label={`전체 ${page.summary.total}`} />
              {Object.entries(page.summary.status).map(([key, count]) => (
                <Chip
                  key={key}
                  size="small"
                  color={deliveryStatus(key).color}
                  variant="outlined"
                  label={`${deliveryStatus(key).label} ${count}`}
                />
              ))}
            </Box>
            {page.items.length === 0 && (
              <Typography color="text.secondary" sx={{ py: 3 }}>
                아직 나간 메일이 없습니다.
              </Typography>
            )}
            <Box sx={{ overflowX: "auto" }}>
              <Box sx={{ minWidth: 760 }}>
                {page.items.map((item) => (
                  <Box
                    key={item.id}
                    sx={{
                      display: "grid",
                      gridTemplateColumns: "170px 110px 1fr 90px",
                      gap: 2,
                      py: 1.5,
                      borderBottom: "1px solid",
                      borderColor: "divider",
                      alignItems: "start",
                    }}
                  >
                    <Typography variant="body2" color="text.secondary">
                      {formatDate(item.created_at, true)}
                    </Typography>
                    <Typography>{mailEventLabel(item.event)}</Typography>
                    <Box>
                      <Typography sx={{ fontWeight: 700 }}>
                        {item.subject}
                      </Typography>
                      <Typography variant="body2" color="text.secondary">
                        {item.recipient}
                        {item.attempts > 1 ? ` · ${item.attempts}회 시도` : ""}
                      </Typography>
                      {item.error_message && (
                        <Typography variant="body2" color="error.main">
                          {item.error_message}
                        </Typography>
                      )}
                    </Box>
                    <Chip
                      size="small"
                      color={deliveryStatus(item.status).color}
                      label={deliveryStatus(item.status).label}
                    />
                  </Box>
                ))}
              </Box>
            </Box>
          </>
        )}
      </CardContent>
    </Card>
  );
}
