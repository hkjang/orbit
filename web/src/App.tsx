import { lazy, Suspense } from "react";
import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import { Box, CircularProgress } from "@mui/material";
import { useAuth } from "./AuthContext";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { Layout } from "./Layout";

const LoginPage = lazy(() =>
  import("./pages/LoginPage").then((m) => ({ default: m.LoginPage })),
);
const OrbitPage = lazy(() =>
  import("./pages/OrbitPage").then((m) => ({ default: m.OrbitPage })),
);
const PeoplePage = lazy(() =>
  import("./pages/PeoplePage").then((m) => ({ default: m.PeoplePage })),
);
const PersonPage = lazy(() =>
  import("./pages/PersonPage").then((m) => ({ default: m.PersonPage })),
);
const MemoriesPage = lazy(() =>
  import("./pages/MemoriesPage").then((m) => ({ default: m.MemoriesPage })),
);
const AIPage = lazy(() =>
  import("./pages/AIPage").then((m) => ({ default: m.AIPage })),
);
const ApprovalsPage = lazy(() =>
  import("./pages/ApprovalsPage").then((m) => ({ default: m.ApprovalsPage })),
);
const PersonalPage = lazy(() =>
  import("./pages/PersonalPage").then((m) => ({ default: m.PersonalPage })),
);
const AdminPage = lazy(() =>
  import("./pages/AdminPage").then((m) => ({ default: m.AdminPage })),
);

function Protected() {
  const { user, loading, expired } = useAuth();
  const location = useLocation();
  if (loading)
    return (
      <Box sx={{ minHeight: "100vh", display: "grid", placeItems: "center" }}>
        <CircularProgress />
      </Box>
    );
  if (!user)
    return (
      <Navigate
        to="/login"
        replace
        state={{ from: location.pathname, expired }}
      />
    );
  return <Layout />;
}

function HomeRedirect() {
  const last = localStorage.getItem("orbit:last-route");
  return (
    <Navigate
      to={last && last !== "/" && last !== "/login" ? last : "/orbit"}
      replace
    />
  );
}

export default function App() {
  const { user } = useAuth();
  const location = useLocation();
  return (
    // 경계는 경로마다 새로 세운다. 한 번 무너진 채로 남으면 다른 화면으로
    // 옮겨가도 계속 오류만 보인다.
    <ErrorBoundary key={location.pathname}>
      <Suspense
        fallback={
          <Box
            sx={{ minHeight: "100vh", display: "grid", placeItems: "center" }}
          >
            <CircularProgress />
          </Box>
        }
      >
        <Routes>
          <Route
            path="/login"
            element={user ? <Navigate to="/" replace /> : <LoginPage />}
          />
          <Route element={<Protected />}>
            <Route path="/" element={<HomeRedirect />} />
            <Route path="/orbit" element={<OrbitPage />} />
            <Route path="/people" element={<PeoplePage />} />
            <Route path="/people/:personId" element={<PersonPage />} />
            <Route path="/memories" element={<MemoriesPage />} />
            <Route path="/ai" element={<AIPage />} />
            <Route path="/approvals" element={<ApprovalsPage />} />
            <Route path="/personal" element={<PersonalPage />} />
            <Route path="/admin/*" element={<AdminPage />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Suspense>
    </ErrorBoundary>
  );
}
