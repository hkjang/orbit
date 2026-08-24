import { Component, type ErrorInfo, type ReactNode } from "react";
import { Box, Button, Typography } from "@mui/material";
import RefreshRoundedIcon from "@mui/icons-material/RefreshRounded";

/**
 * 지연 로딩한 조각을 가져오지 못한 것인지 가린다.
 *
 * 앱을 열어둔 채 새 버전이 배포되면 이전 조각 이름이 사라진다. 그때 사용자가
 * 링크를 누르면 없는 파일을 가져오려다 렌더가 실패한다. 이건 고장이 아니라
 * 새로고침하면 풀리는 상황이라, 다르게 안내해야 한다.
 */
export function isStaleChunkError(error: unknown) {
  const message = error instanceof Error ? error.message : String(error ?? "");
  return /dynamically imported module|module script failed|ChunkLoadError|Loading chunk/i.test(
    message,
  );
}

interface Props {
  children: ReactNode;
}

interface State {
  error: Error | null;
}

/**
 * 렌더 중 예외를 받아 낸다.
 *
 * 경계가 없으면 React가 트리를 통째로 걷어내 흰 화면만 남는다. 사용자는
 * 무슨 일이 일어났는지도, 어떻게 빠져나가는지도 알 수 없다.
 */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // 서버로 보내지 않는다. 화면 내용에는 관계 기록이 섞일 수 있다.
    console.error("화면을 그리지 못했습니다", error, info.componentStack);
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    const stale = isStaleChunkError(error);
    return (
      <Box
        role="alert"
        sx={{
          minHeight: "60vh",
          display: "grid",
          placeItems: "center",
          textAlign: "center",
          px: 3,
        }}
      >
        <Box sx={{ maxWidth: 420 }}>
          <Typography variant="h2" sx={{ mb: 1 }}>
            {stale ? "새 버전이 준비되었어요" : "화면을 그리지 못했어요"}
          </Typography>
          <Typography color="text.secondary" sx={{ mb: 3 }}>
            {stale
              ? "앱이 업데이트되어 이전 화면 조각을 더 불러올 수 없습니다. 새로고침하면 이어서 사용할 수 있습니다."
              : "기록은 그대로 있습니다. 새로고침해도 계속된다면 이 화면을 알려주세요."}
          </Typography>
          <Button
            variant="contained"
            startIcon={<RefreshRoundedIcon />}
            onClick={() => window.location.reload()}
          >
            새로고침
          </Button>
        </Box>
      </Box>
    );
  }
}
