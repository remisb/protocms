import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { api, getToken, setToken } from "@/lib/api";
import type { Me } from "@/lib/types";

type AuthStatus = "loading" | "authed" | "anon";

interface AuthContextValue {
  status: AuthStatus;
  user: Me | null;
  role: string | null;
  isAdmin: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>("loading");
  const [user, setUser] = useState<Me | null>(null);

  const reset = useCallback(() => {
    setUser(null);
    setStatus("anon");
  }, []);

  // Validate any persisted token on mount.
  useEffect(() => {
    let alive = true;
    if (!getToken()) {
      setStatus("anon");
      return;
    }
    api.me().then(
      (me) => {
        if (!alive) return;
        setUser(me);
        setStatus("authed");
      },
      () => {
        if (!alive) return;
        setToken(null);
        reset();
      },
    );
    return () => {
      alive = false;
    };
  }, [reset]);

  // The API client fires this when the server rejects our token mid-session.
  useEffect(() => {
    const onUnauthorized = () => reset();
    window.addEventListener("protocms:unauthorized", onUnauthorized);
    return () =>
      window.removeEventListener("protocms:unauthorized", onUnauthorized);
  }, [reset]);

  const login = useCallback(async (username: string, password: string) => {
    const res = await api.login(username, password);
    setToken(res.token);
    const me = await api.me();
    setUser(me);
    setStatus("authed");
  }, []);

  const logout = useCallback(() => {
    api.logout();
    reset();
  }, [reset]);

  const role = user?.roles?.[0] ?? null;

  const value = useMemo<AuthContextValue>(
    () => ({
      status,
      user,
      role,
      isAdmin: role === "admin",
      login,
      logout,
    }),
    [status, user, role, login, logout],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
