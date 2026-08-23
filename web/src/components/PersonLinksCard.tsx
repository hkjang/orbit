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
  IconButton,
  MenuItem,
  TextField,
  Typography,
} from "@mui/material";
import CloseRoundedIcon from "@mui/icons-material/CloseRounded";
import HubRoundedIcon from "@mui/icons-material/HubRounded";
import { useNavigate } from "react-router-dom";
import { api } from "../api";
import type { Person, PersonLink } from "../types";

const LINK_KINDS = [
  { value: "colleague", label: "함께 일한 사이" },
  { value: "family", label: "가족" },
  { value: "friend", label: "친구" },
  { value: "community", label: "같은 모임" },
  { value: "knows", label: "아는 사이" },
];

/**
 * 이 사람과 다른 사람을 잇는 카드.
 * 나를 거치지 않는 관계까지 기록되면 우주가 별들의 그물이 됩니다.
 */
export function PersonLinksCard({
  personId,
  personName,
}: {
  personId: string;
  personName: string;
}) {
  const navigate = useNavigate();
  const [links, setLinks] = useState<PersonLink[]>();
  const [error, setError] = useState("");
  const [adding, setAdding] = useState(false);
  const load = useCallback(async () => {
    try {
      const data = await api<{ links: PersonLink[] }>(
        `/people/${personId}/links`,
      );
      setLinks(data.links);
    } catch (e) {
      setError(e instanceof Error ? e.message : "연결을 불러오지 못했습니다.");
    }
  }, [personId]);
  useEffect(() => {
    void load();
  }, [load]);
  return (
    <Card sx={{ mt: 2 }}>
      <CardContent>
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 1,
          }}
        >
          <Typography variant="h3">이어진 사람들</Typography>
          <Button
            size="small"
            startIcon={<HubRoundedIcon />}
            onClick={() => setAdding(true)}
          >
            잇기
          </Button>
        </Box>
        {error && (
          <Alert severity="error" sx={{ mt: 1.5 }}>
            {error}
          </Alert>
        )}
        {links && links.length === 0 && (
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1.5 }}>
            아직 이어진 사람이 없습니다. {personName}님과 함께 아는 사람을
            이어두면 우주 지도에 두 행성 사이의 인력이 그려집니다.
          </Typography>
        )}
        {links?.map((link) => (
          <Box
            key={link.id}
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 1,
              mt: 1.5,
            }}
          >
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Typography
                sx={{ fontWeight: 700, cursor: "pointer" }}
                onClick={() => navigate(`/people/${link.person_id}`)}
              >
                {link.person_name}
              </Typography>
              <Typography variant="body2" color="text.secondary" noWrap>
                {[link.company, link.role_title].filter(Boolean).join(" · ") ||
                  "소속 기록 없음"}
              </Typography>
            </Box>
            <Chip size="small" variant="outlined" label={link.kind_label} />
            <IconButton
              size="small"
              aria-label={`${link.person_name} 연결 끊기`}
              onClick={async () => {
                try {
                  await api(`/people/${personId}/links/${link.id}`, {
                    method: "DELETE",
                  });
                  await load();
                } catch (e) {
                  setError(
                    e instanceof Error ? e.message : "연결을 끊지 못했습니다.",
                  );
                }
              }}
            >
              <CloseRoundedIcon fontSize="small" />
            </IconButton>
          </Box>
        ))}
      </CardContent>
      <LinkDialog
        open={adding}
        personId={personId}
        linked={links?.map((link) => link.person_id) ?? []}
        onClose={() => setAdding(false)}
        onSaved={load}
      />
    </Card>
  );
}

function LinkDialog({
  open,
  personId,
  linked,
  onClose,
  onSaved,
}: {
  open: boolean;
  personId: string;
  linked: string[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [people, setPeople] = useState<Person[]>([]);
  const [target, setTarget] = useState("");
  const [kind, setKind] = useState("colleague");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    if (!open) return;
    setError("");
    setTarget("");
    api<{ people: Person[] }>("/people")
      .then((data) => setPeople(data.people))
      .catch((e) =>
        setError(e instanceof Error ? e.message : "목록을 불러오지 못했습니다."),
      );
  }, [open]);
  // 이미 이어진 사람과 자기 자신은 고를 수 없습니다.
  const options = people.filter(
    (p) => p.id !== personId && !linked.includes(p.id),
  );
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="xs">
      <DialogTitle>누구와 이어져 있나요?</DialogTitle>
      <DialogContent>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        <TextField
          select
          fullWidth
          label="상대"
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          sx={{ mt: 1 }}
          helperText={
            options.length === 0 ? "이을 수 있는 사람이 없습니다." : undefined
          }
        >
          {options.map((p) => (
            <MenuItem key={p.id} value={p.id}>
              {p.display_name}
              {p.company ? ` · ${p.company}` : ""}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          select
          fullWidth
          label="관계"
          value={kind}
          onChange={(e) => setKind(e.target.value)}
          sx={{ mt: 2 }}
        >
          {LINK_KINDS.map((k) => (
            <MenuItem key={k.value} value={k.value}>
              {k.label}
            </MenuItem>
          ))}
        </TextField>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>취소</Button>
        <Button
          variant="contained"
          disabled={!target || saving}
          onClick={async () => {
            setSaving(true);
            try {
              await api(`/people/${personId}/links`, {
                method: "POST",
                body: JSON.stringify({ person_id: target, kind }),
              });
              onSaved();
              onClose();
            } catch (e) {
              setError(e instanceof Error ? e.message : "잇지 못했습니다.");
            } finally {
              setSaving(false);
            }
          }}
        >
          잇기
        </Button>
      </DialogActions>
    </Dialog>
  );
}
