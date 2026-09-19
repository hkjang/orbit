import { useEffect, useState } from "react";
import { Box, Button, Card, CardContent, Slider, Typography } from "@mui/material";
import { formatDate } from "../api";
import { dayToTravelValue } from "../timeTravel";

/**
 * 과거의 우주로 옮겨 가는 슬라이더.
 *
 * 되살아나는 것은 교류가 만든 거리와 흐름이다. 중요도·소속·고정 여부는
 * 사용자가 직접 정하는 값이라 변경 이력이 없어 오늘의 값을 쓴다. 그 사실을
 * 화면에서도 숨기지 않는다.
 *
 * 끄는 동안에는 날짜 표시만 따라가고(`draft`), 서버에 묻는 것은 손을 뗄 때
 * 한 번이다. 칸마다 우주를 다시 부르면 1년치 슬라이더에서 수백 요청이 나간다.
 */
export function TimeTravel({
  earliest,
  value,
  onChange,
}: {
  earliest: string;
  value?: string;
  onChange: (value?: string) => void;
}) {
  const start = new Date(earliest).getTime();
  const end = Date.now();
  const current = value ? new Date(value).getTime() : end;
  const days = Math.max(1, Math.round((end - start) / 86_400_000));
  const dayOf = (time: number) => Math.round((time - start) / 86_400_000);
  // 끄는 중인 칸. 손을 떼거나 부모가 값을 바꾸면("현재로", 뒤로 가기) 비운다.
  const [draft, setDraft] = useState<number>();
  useEffect(() => {
    setDraft(undefined);
  }, [value]);
  const shown =
    draft === undefined
      ? value
      : new Date(start + draft * 86_400_000).toISOString();
  return (
    <Card sx={{ mb: 2.5 }}>
      <CardContent sx={{ py: "16px!important" }}>
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            gap: 2,
            flexWrap: "wrap",
          }}
        >
          <Box sx={{ minWidth: 160 }}>
            <Typography
              variant="overline"
              color="primary.light"
              sx={{ letterSpacing: ".13em" }}
            >
              TIME TRAVEL
            </Typography>
            <Typography sx={{ fontWeight: 720 }}>
              {shown ? formatDate(shown) : "지금의 우주"}
            </Typography>
          </Box>
          <Slider
            value={draft ?? dayOf(current)}
            min={0}
            max={days}
            step={1}
            aria-label="돌아볼 시점"
            valueLabelDisplay="auto"
            valueLabelFormat={(day) =>
              formatDate(new Date(start + day * 86_400_000).toISOString())
            }
            onChange={(_, day) => setDraft(day as number)}
            onChangeCommitted={(_, day) => {
              setDraft(undefined);
              onChange(dayToTravelValue(start, days, day as number));
            }}
            sx={{ flex: "1 1 240px", mx: 1 }}
          />
          {value && (
            <Button size="small" onClick={() => onChange(undefined)}>
              현재로
            </Button>
          )}
        </Box>
        {value && (
          <Typography variant="caption" color="text.secondary">
            그날의 거리와 흐름을 교류 기록에서 다시 계산했습니다.
            중요도·소속·고정은 변경 이력이 없어 오늘의 값을 씁니다.
          </Typography>
        )}
      </CardContent>
    </Card>
  );
}
