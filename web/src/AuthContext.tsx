import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { useColorScheme } from "@mui/material";
import { api } from "./api";
import type { PublicConfig, User } from "./types";

interface AuthState {
  user: User | null;
  config: PublicConfig | null;
  loading: boolean;
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

  const refresh = async () => {
    try {
      const result = await api<{ user: User }>("/me");
      setUser(result.user);
    } catch {
      setUser(null);
    }
  };

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
      login: async (username, password) => {
        const result = await api<{ user: User }>("/auth/login", {
          method: "POST",
          body: JSON.stringify({ username, password }),
        });
        setUser(result.user);
      },
      logout: async () => {
        await api("/auth/logout", { method: "POST", body: "{}" });
        setUser(null);
      },
      refresh,
    }),
    [user, config, loading],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("AuthProvider is missing");
  return value;
}
