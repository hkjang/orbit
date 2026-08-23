import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Avatar,
  Box,
  Button,
  Card,
  CardActionArea,
  CardContent,
  Chip,
  InputAdornment,
  MenuItem,
  TextField,
  Typography,
} from "@mui/material";
import AddRoundedIcon from "@mui/icons-material/AddRounded";
import SearchRoundedIcon from "@mui/icons-material/SearchRounded";
import NorthEastRoundedIcon from "@mui/icons-material/NorthEastRounded";
import PushPinRoundedIcon from "@mui/icons-material/PushPinRounded";
import { useNavigate, useSearchParams } from "react-router-dom";
import { api, formatDate } from "../api";
import { PageHeader } from "../components/PageHeader";
import { StateChip } from "../components/StateChip";
import { EmptyView, ErrorView, LoadingView } from "../components/StateViews";
import { PersonFormDialog } from "../components/PersonFormDialog";
import { STATE_META, STATE_ORDER, type RelationState } from "../orbitGrammar";
import {
  countByState,
  filterByState,
  isPeopleSort,
  SORTS,
  sortPeople,
} from "../peopleView";
import type { Person } from "../types";

export function PeoplePage() {
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [people, setPeople] = useState<Person[]>();
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");
  const [open, setOpen] = useState(params.get("new") === "1");
  // 고른 상태와 정렬은 URL에 남긴다. 뒤로가기로 돌아오거나 링크로 건네도
  // 같은 화면이 나와야 "멀어지는 중인 사람들"이 하나의 장소가 된다.
  const state = (params.get("state") ?? "") as RelationState | "";
  const sortParam = params.get("sort");
  const sort = isPeopleSort(sortParam) ? sortParam : "importance";
  const setParam = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    setParams(next, { replace: true });
  };
  const load = useCallback(async () => {
    setError("");
    try {
      const data = await api<{ people: Person[] }>(
        `/people/?q=${encodeURIComponent(query)}`,
      );
      setPeople(data.people);
    } catch (e) {
      setError(e instanceof Error ? e.message : "관계를 불러오지 못했습니다.");
    }
  }, [query]);
  useEffect(() => {
    const t = setTimeout(() => void load(), 200);
    return () => clearTimeout(t);
  }, [load]);
  const counts = useMemo(() => countByState(people ?? []), [people]);
  const shown = useMemo(
    () => sortPeople(filterByState(people ?? [], state), sort),
    [people, state, sort],
  );
  const close = () => {
    setOpen(false);
    if (params.has("new")) {
      params.delete("new");
      setParams(params, { replace: true });
    }
  };
  return (
    <>
      <PageHeader
        title="관계"
        description="연락처가 아니라, 나와 그 사람 사이의 이야기를 관리합니다."
        action={
          <Button
            variant="contained"
            startIcon={<AddRoundedIcon />}
            onClick={() => setOpen(true)}
          >
            사람 추가
          </Button>
        }
      />
      <TextField
        fullWidth
        placeholder="이름, 회사, 역할, 관계, 소속으로 찾기"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        slotProps={{ input: {
          startAdornment: (
            <InputAdornment position="start">
              <SearchRoundedIcon />
            </InputAdornment>
          ),
        } }}
        sx={{ maxWidth: 560, mb: 2 }}
      />
      {people && people.length > 0 && (
        <Box
          sx={{
            display: "flex",
            gap: 1,
            flexWrap: "wrap",
            alignItems: "center",
            mb: 3,
          }}
        >
          <Chip
            label={`전체 ${people.length}`}
            variant={state ? "outlined" : "filled"}
            color={state ? "default" : "primary"}
            clickable
            aria-pressed={!state}
            onClick={() => setParam("state", "")}
          />
          {STATE_ORDER.filter((s) => counts[s]).map((s) => (
            <Chip
              key={s}
              label={`${STATE_META[s].label} ${counts[s]}`}
              title={STATE_META[s].hint}
              variant={state === s ? "filled" : "outlined"}
              color={state === s ? "primary" : "default"}
              clickable
              aria-pressed={state === s}
              icon={
                <Box
                  component="span"
                  sx={{
                    width: 8,
                    height: 8,
                    borderRadius: "50%",
                    bgcolor: STATE_META[s].tone,
                    ml: "9px!important",
                  }}
                />
              }
              onClick={() => setParam("state", state === s ? "" : s)}
            />
          ))}
          <TextField
            select
            size="small"
            value={sort}
            onChange={(e) => setParam("sort", e.target.value)}
            aria-label="정렬"
            sx={{ minWidth: 140, ml: { sm: "auto" } }}
          >
            {SORTS.map((option) => (
              <MenuItem key={option.value} value={option.value}>
                {option.label}
              </MenuItem>
            ))}
          </TextField>
        </Box>
      )}
      {error ? (
        <ErrorView message={error} retry={load} />
      ) : !people ? (
        <LoadingView label="관계를 불러오는 중…" />
      ) : people.length === 0 ? (
        <EmptyView
          title={query ? "찾는 사람이 없어요" : "아직 등록된 사람이 없어요"}
          description={
            query
              ? "다른 이름이나 회사로 다시 찾아보세요."
              : "중요한 사람 한 명을 추가하면 Orbit이 시작됩니다."
          }
          action={
            !query && (
              <Button variant="contained" onClick={() => setOpen(true)}>
                첫 사람 추가
              </Button>
            )
          }
        />
      ) : shown.length === 0 ? (
        <EmptyView
          title={`${state ? STATE_META[state].label : "해당"} 궤도에는 아무도 없어요`}
          description="다행일 수도 있습니다. 다른 상태를 골라 보세요."
          action={
            <Button variant="outlined" onClick={() => setParam("state", "")}>
              전체 보기
            </Button>
          }
        />
      ) : (
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill,minmax(270px,1fr))",
            gap: 2,
          }}
        >
          {shown.map((person) => (
            <Card key={person.id}>
              <CardActionArea
                onClick={() => navigate(`/people/${person.id}`)}
                sx={{ height: "100%" }}
              >
                <CardContent sx={{ p: 2.5 }}>
                  <Box sx={{ display: "flex", gap: 1.5, alignItems: "center" }}>
                    <Avatar
                      src={person.avatar_url}
                      sx={{ width: 50, height: 50, bgcolor: "primary.dark" }}
                    >
                      {person.display_name[0]}
                    </Avatar>
                    <Box sx={{ minWidth: 0, flex: 1 }}>
                      <Typography variant="h3" noWrap>
                        {person.display_name}
                      </Typography>
                      <Typography variant="body2" color="text.secondary" noWrap>
                        {person.relationship_label ||
                          [person.company, person.role_title]
                            .filter(Boolean)
                            .join(" · ") ||
                          "관계를 알려주세요"}
                      </Typography>
                    </Box>
                    <NorthEastRoundedIcon color="disabled" />
                  </Box>
                  <Box
                    sx={{ display: "flex", gap: 0.7, mt: 2, flexWrap: "wrap" }}
                  >
                    {person.categories.slice(0, 3).map((c) => (
                      <Chip key={c} size="small" label={c} />
                    ))}
                    <StateChip person={person} hideStable />
                    {person.anchored && (
                      <Chip
                        size="small"
                        variant="outlined"
                        icon={<PushPinRoundedIcon sx={{ fontSize: 14 }} />}
                        label="고정"
                        title="교류가 뜸해져도 바깥 궤도로 밀려나지 않습니다."
                      />
                    )}
                  </Box>
                  <Typography
                    variant="body2"
                    color="text.secondary"
                    sx={{ mt: 2 }}
                  >
                    마지막 교류 · {formatDate(person.last_interaction_at)}
                  </Typography>
                </CardContent>
              </CardActionArea>
            </Card>
          ))}
        </Box>
      )}
      <PersonFormDialog open={open} onClose={close} onSaved={load} />
    </>
  );
}
