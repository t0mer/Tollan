import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { api, auth, type Me } from "@/lib/api";
import { ApiError } from "@/lib/api";

type AuthState = {
  loading: boolean;
  authEnabled: boolean;
  needsSetup: boolean;
  user: Me | null;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [loading, setLoading] = useState(true);
  const [authEnabled, setAuthEnabled] = useState(false);
  const [needsSetup, setNeedsSetup] = useState(false);
  const [user, setUser] = useState<Me | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const status = await auth.status();
      setAuthEnabled(status.auth_enabled);
      setNeedsSetup(status.needs_setup);
      if (status.auth_enabled && !status.needs_setup) {
        try {
          setUser(await auth.me());
        } catch (e) {
          if (e instanceof ApiError && e.status === 401) setUser(null);
        }
      } else if (!status.auth_enabled) {
        // Open mode: report the server's synthetic principal.
        try {
          setUser(await api.version().then(() => ({ id: "", username: "anonymous", role: "admin" as const })));
        } catch {
          setUser({ id: "", username: "anonymous", role: "admin" });
        }
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const logout = useCallback(async () => {
    await auth.logout();
    setUser(null);
    await refresh();
  }, [refresh]);

  return (
    <AuthContext.Provider value={{ loading, authEnabled, needsSetup, user, refresh, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}

/** canEdit reports whether the current role may create/modify entities. */
export function useCanEdit(): boolean {
  const { user } = useAuth();
  return user?.role === "admin" || user?.role === "editor";
}

export function useIsAdmin(): boolean {
  return useAuth().user?.role === "admin";
}
