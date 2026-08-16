import { useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Slider,
  TextField,
  Typography,
} from "@mui/material";
import { api } from "../api";
import type { Person } from "../types";

export function PersonFormDialog({
  open,
  person,
  onClose,
  onSaved,
}: {
  open: boolean;
  person?: Person;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [form, setForm] = useState({
    display_name: "",
    company: "",
    role_title: "",
    email: "",
    phone: "",
    note: "",
    first_met: "",
    importance: 0.6,
    relationship_label: "",
    categories: "",
  });
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (open) {
      setForm(
        person
          ? {
              display_name: person.display_name,
              company: person.company,
              role_title: person.role_title,
              email: person.email,
              phone: person.phone,
              note: person.note,
              first_met: person.first_met?.slice(0, 10) ?? "",
              importance: person.importance,
              relationship_label: person.relationship_label,
              categories: person.categories.join(", "),
            }
          : {
              display_name: "",
              company: "",
              role_title: "",
              email: "",
              phone: "",
              note: "",
              first_met: "",
              importance: 0.6,
              relationship_label: "",
              categories: "",
            },
      );
      setError("");
    }
  }, [open, person]);
  const save = async () => {
    setBusy(true);
    setError("");
    try {
      const body = {
        ...form,
        categories: form.categories
          .split(",")
          .map((v) => v.trim())
          .filter(Boolean),
      };
      await api(person ? `/people/${person.id}` : "/people/", {
        method: person ? "PUT" : "POST",
        body: JSON.stringify(body),
      });
      onSaved();
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : "저장하지 못했습니다.");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog
      open={open}
      onClose={busy ? undefined : onClose}
      fullWidth
      maxWidth="sm"
    >
      <DialogTitle>
        {person ? "관계 정보 다듬기" : "새로운 행성 만들기"}
      </DialogTitle>
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
            pt: 0.5,
          }}
        >
          <TextField
            required
            label="이름"
            value={form.display_name}
            onChange={(e) => setForm({ ...form, display_name: e.target.value })}
          />
          <TextField
            label="우리 관계"
            placeholder="예: 대학 친구 · 12년"
            value={form.relationship_label}
            onChange={(e) =>
              setForm({ ...form, relationship_label: e.target.value })
            }
          />
          <TextField
            label="회사/소속"
            value={form.company}
            onChange={(e) => setForm({ ...form, company: e.target.value })}
          />
          <TextField
            label="역할"
            value={form.role_title}
            onChange={(e) => setForm({ ...form, role_title: e.target.value })}
          />
          <TextField
            label="이메일"
            type="email"
            value={form.email}
            onChange={(e) => setForm({ ...form, email: e.target.value })}
          />
          <TextField
            label="전화번호"
            value={form.phone}
            onChange={(e) => setForm({ ...form, phone: e.target.value })}
          />
          <TextField
            label="처음 만난 날"
            type="date"
            slotProps={{ inputLabel: { shrink: true } }}
            value={form.first_met}
            onChange={(e) => setForm({ ...form, first_met: e.target.value })}
          />
          <TextField
            label="관계 맥락"
            placeholder="가족, 러닝, AI 프로젝트"
            value={form.categories}
            onChange={(e) => setForm({ ...form, categories: e.target.value })}
          />
          <Box sx={{ gridColumn: "1/-1", px: 0.5 }}>
            <Typography gutterBottom>내 삶에서의 중요도</Typography>
            <Slider
              value={form.importance}
              min={0.1}
              max={1}
              step={0.1}
              valueLabelDisplay="auto"
              valueLabelFormat={(v) =>
                v > 0.8 ? "매우 중요" : v > 0.5 ? "중요" : "보통"
              }
              onChange={(_, v) => setForm({ ...form, importance: v as number })}
            />
            <Typography variant="caption" color="text.secondary">
              최근 연락 빈도와 별개인 장기적인 중요도입니다.
            </Typography>
          </Box>
          <TextField
            sx={{ gridColumn: "1/-1" }}
            label="나만의 메모"
            multiline
            minRows={3}
            value={form.note}
            onChange={(e) => setForm({ ...form, note: e.target.value })}
          />
        </Box>
      </DialogContent>
      <DialogActions sx={{ p: 2 }}>
        <Button onClick={onClose}>취소</Button>
        <Button
          variant="contained"
          disabled={busy || !form.display_name.trim()}
          onClick={save}
        >
          {busy ? "저장 중…" : "저장하기"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
