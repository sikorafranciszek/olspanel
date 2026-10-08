import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { get, post, setCsrf } from "./api";
import type { Me, User } from "./types";

interface AuthState {
  user: User | null;
  impersonating: boolean;
  version: string;
  loading: boolean;
  login: (username: string, password: string) => Promise<User>;
  logout: () => Promise<void>;
  applyMe: (me: Me) => void;
  refresh: () => Promise<void>;
}

const Ctx = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [impersonating, setImpersonating] = useState(false);
  const [version, setVersion] = useState("");
  const [loading, setLoading] = useState(true);
  const qc = useQueryClient();

  const applyMe = useCallback(
    (me: Me) => {
      setUser(me.user);
      setImpersonating(me.impersonating);
      setVersion(me.version);
      setCsrf(me.csrf ?? "");
      qc.clear();
    },
    [qc],
  );

  const refresh = useCallback(async () => {
    try {
      const me = await get<Me>("/auth/me");
      applyMe(me);
    } catch {
      setUser(null);
    } finally {
      setLoading(false);
    }
  }, [applyMe]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const login = useCallback(
    async (username: string, password: string) => {
      const me = await post<Me>("/auth/login", { username, password });
      applyMe(me);
      return me.user!;
    },
    [applyMe],
  );

  const logout = useCallback(async () => {
    try {
      await post("/auth/logout");
    } finally {
      setUser(null);
      setImpersonating(false);
      setCsrf("");
      qc.clear();
    }
  }, [qc]);

  const value = useMemo(
    () => ({ user, impersonating, version, loading, login, logout, applyMe, refresh }),
    [user, impersonating, version, loading, login, logout, applyMe, refresh],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAuth(): AuthState {
  const v = useContext(Ctx);
  if (!v) throw new Error("useAuth outside AuthProvider");
  return v;
}
