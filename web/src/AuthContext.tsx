import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { useColorScheme } from "@mui/material";
import { api, setUnauthorizedHandler } from "./api";
import type { PublicConfig, User } from "./types";

interface AuthState {
  user: User | null;
  config: PublicConfig | null;
  loading: boolean;
  /** 세션 만료로 로그아웃된 상태인지. 로그인 화면에서 이유를 알려주는 데 쓴다. */
  expired: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const { setMode } = useColorScheme();
  const [user, setUser] = useState<User | null>(null);
  const [config, setConfig] = useState<PublicConfig | null>(null);
  const [loading, setLoading] = useState(true);
  // 세션이 끊겨서 로그아웃된 것인지, 원래 로그인 전인지 구분한다.
  const [expired, setExpired] = useState(false);

  const refresh = async () => {
    try {
      const result = await api<{ user: User }>("/me");
      setUser(result.user);
    } catch {
      setUser(null);
    }
  };

  // 어느 요청에서든 세션이 끊긴 것이 확인되면 화면의 사용자도 비운다.
  // 그래야 Protected가 로그인 화면으로 돌려보낸다.
  useEffect(() => {
    setUnauthorizedHandler(() => {
      setUser((current) => {
        if (current) setExpired(true);
        return null;
      });
    });
    return () => setUnauthorizedHandler(undefined);
  }, []);

  useEffect(() => {
    Promise.all([
      fetch("/api/v1/public/config")
        .then((r) => r.json())
        .then(setConfig)
        .catch(() => undefined),
      refresh(),
    ]).finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    if (!user) return;
    api<{
      preferences: {
        theme: "dark" | "light" | "system";
        font_scale: number;
        reduce_motion: boolean;
      };
    }>("/personal/preferences")
      .then(({ preferences }) => {
        setMode(preferences.theme);
        document.documentElement.style.fontSize = `${16 * preferences.font_scale}px`;
        document.documentElement.dataset.reduceMotion = String(preferences.reduce_motion);
      })
      .catch(() => undefined);
  }, [user, setMode]);

  const value = useMemo<AuthState>(
    () => ({
      user,
      config,
      loading,
      expired,
      login: async (username, password) => {
        const result = await api<{ user: User }>("/auth/login", {
          method: "POST",
          body: JSON.stringify({ username, password }),
        });
        setExpired(false);
        setUser(result.user);
      },
      logout: async () => {
        await api("/auth/logout", { method: "POST", body: "{}" });
        setExpired(false);
        setUser(null);
      },
      refresh,
    }),
    [user, config, loading, expired],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("AuthProvider is missing");
  return value;
}
