import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
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
  TextField,
  Typography,
} from "@mui/material";
import CheckRoundedIcon from "@mui/icons-material/CheckRounded";
import CloseRoundedIcon from "@mui/icons-material/CloseRounded";
import { api, formatDate } from "../api";
import { useAuth } from "../AuthContext";
import { PageHeader } from "../components/PageHeader";
import { EmptyView, ErrorView, LoadingView } from "../components/StateViews";

interface Approval {
  id: string;
  requester_name: string;
  resource_type: string;
  resource_id: string;
  resource_title: string;
  action: string;
  status: string;
  request_note: string;
  review_note: string;
  created_at: string;
  reviewed_at?: string;
}
const APPROVAL_FILTERS = [
  { value: "pending", label: "검토 대기" },
  { value: "approved", label: "승인됨" },
  { value: "rejected", label: "반려됨" },
  { value: "", label: "전체" },
];

export function ApprovalsPage() {
  const { user } = useAuth();
  const [params, setParams] = useSearchParams();
  const [data, setData] = useState<{
    enabled: boolean;
    can_review: boolean;
    approvals: Approval[];
    counts: Record<string, number>;
  }>();
  const [error, setError] = useState("");
  const [review, setReview] = useState<{
    item: Approval;
    decision: "approved" | "rejected";
  }>();
  // 기본은 실제로 처리할 항목이다. 이력까지 한꺼번에 보이면 할 일이 묻힌다.
  const status = params.get("status") ?? "pending";
  const setStatus = (value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set("status", value);
    else next.set("status", "all");
    setParams(next, { replace: true });
  };
  const queried = status === "all" ? "" : status;
  const load = useCallback(async () => {
    try {
      setData(
        await api(`/approvals?status=${encodeURIComponent(queried)}`),
      );
      setError("");
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "검토 요청을 불러오지 못했습니다.",
      );
    }
  }, [queried]);
  useEffect(() => {
    void load();
  }, [load]);
  if (error) return <ErrorView message={error} retry={load} />;
  if (!data) return <LoadingView />;
  if (!data.enabled)
    return (
      <>
        <PageHeader title="검토 및 승인" />
        <EmptyView
          title="승인 프로세스를 사용하지 않고 있어요"
          description="관리자가 프로세스를 켜기 전에는 기억이 검토 단계 없이 바로 반영됩니다."
        />
      </>
    );
  return (
    <>
      <PageHeader
        title="검토 및 승인"
        description={
          user?.role === "member"
            ? "내가 요청한 기록의 진행 상태입니다."
            : "팀원이 남긴 관계 기록을 맥락과 함께 검토합니다."
        }
      />
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mb: 3 }}>
        {APPROVAL_FILTERS.map((option) => {
          const selected =
            option.value === "" ? status === "all" : status === option.value;
          const count =
            option.value === ""
              ? Object.values(data.counts ?? {}).reduce((a, b) => a + b, 0)
              : (data.counts?.[option.value] ?? 0);
          return (
            <Chip
              key={option.value || "all"}
              label={`${option.label} ${count}`}
              variant={selected ? "filled" : "outlined"}
              color={
                selected
                  ? option.value === "pending"
                    ? "warning"
                    : "primary"
                  : "default"
              }
              clickable
              aria-pressed={selected}
              onClick={() => setStatus(option.value)}
            />
          );
        })}
      </Box>
      {data.approvals.length === 0 ? (
        <EmptyView
          title={
            status === "pending"
              ? "지금 검토할 요청이 없어요"
              : "이 상태의 요청이 없어요"
          }
          description="처리된 이력까지 보려면 전체를 눌러 보세요."
          action={
            status !== "all" && (
              <Button variant="outlined" onClick={() => setStatus("")}>
                전체 보기
              </Button>
            )
          }
        />
      ) : (
        <Box sx={{ display: "grid", gap: 1.5 }}>
          {data.approvals.map((item) => (
            <Card key={item.id}>
              <CardContent
                sx={{
                  display: "flex",
                  alignItems: { xs: "flex-start", md: "center" },
                  justifyContent: "space-between",
                  gap: 2,
                  flexDirection: { xs: "column", md: "row" },
                }}
              >
                <Box>
                  <Box
                    sx={{
                      display: "flex",
                      gap: 1,
                      alignItems: "center",
                      mb: 0.7,
                    }}
                  >
                    <Chip
                      size="small"
                      label={
                        item.status === "pending"
                          ? "대기"
                          : item.status === "approved"
                            ? "승인"
                            : "반려"
                      }
                      color={
                        item.status === "pending"
                          ? "warning"
                          : item.status === "approved"
                            ? "success"
                            : "default"
                      }
                    />
                    <Typography variant="body2" color="text.secondary">
                      {formatDate(item.created_at, true)}
                    </Typography>
                  </Box>
                  <Typography variant="h3">
                    {item.resource_title || item.resource_type}
                  </Typography>
                  <Typography color="text.secondary" sx={{ mt: 0.4 }}>
                    요청자 {item.requester_name}
                    {item.request_note && ` · ${item.request_note}`}
                  </Typography>
                </Box>
              {item.status === "pending" && data.can_review && (
                    <Box sx={{ display: "flex", gap: 1 }}>
                      <Button
                        color="inherit"
                        startIcon={<CloseRoundedIcon />}
                        onClick={() =>
                          setReview({ item, decision: "rejected" })
                        }
                      >
                        반려
                      </Button>
                      <Button
                        variant="contained"
                        color="success"
                        startIcon={<CheckRoundedIcon />}
                        onClick={() =>
                          setReview({ item, decision: "approved" })
                        }
                      >
                        승인
                      </Button>
                    </Box>
                  )}
              </CardContent>
            </Card>
          ))}
        </Box>
      )}
      <ReviewDialog
        review={review}
        close={() => setReview(undefined)}
        saved={load}
      />
    </>
  );
}
function ReviewDialog({
  review,
  close,
  saved,
}: {
  review?: { item: Approval; decision: "approved" | "rejected" };
  close: () => void;
  saved: () => void;
}) {
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const submit = async () => {
    if (!review) return;
    try {
      await api(`/approvals/${review.item.id}/review`, {
        method: "POST",
        body: JSON.stringify({ decision: review.decision, note }),
      });
      setNote("");
      saved();
      close();
    } catch (e) {
      setError(e instanceof Error ? e.message : "처리하지 못했습니다.");
    }
  };
  return (
    <Dialog open={Boolean(review)} onClose={close} fullWidth maxWidth="xs">
      <DialogTitle>
        {review?.decision === "approved" ? "기억 승인" : "기억 반려"}
      </DialogTitle>
      <DialogContent>
        {error && <Alert severity="error">{error}</Alert>}
        <Typography color="text.secondary" sx={{ my: 1 }}>
          {review?.item.resource_title}
        </Typography>
        <TextField
          fullWidth
          multiline
          minRows={3}
          label="검토 의견"
          value={note}
          onChange={(e) => setNote(e.target.value)}
          sx={{ mt: 1 }}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={close}>취소</Button>
        <Button
          variant="contained"
          color={review?.decision === "approved" ? "success" : "warning"}
          onClick={submit}
        >
          {review?.decision === "approved" ? "승인하기" : "반려하기"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
