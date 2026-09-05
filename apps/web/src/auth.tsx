import * as React from "react";
import { api } from "./api";
import type { User } from "./types";

interface AuthValue {
  user: User | null;
  setupRequired: boolean;
  loading: boolean;
  signIn(identity: string, password: string): Promise<void>;
  signOut(): Promise<void>;
  completeSetup(input: Record<string, string>): Promise<void>;
}

const AuthContext = React.createContext<AuthValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = React.useState<User | null>(null);
  const [setupRequired, setSetupRequired] = React.useState(false);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    api.get<{ required: boolean }>("/setup/status").then(async ({ required }) => {
      setSetupRequired(required);
      if (!required) {
        try { setUser((await api.get<{ user: User }>("/auth/me")).user); } catch { setUser(null); }
      }
    }).finally(() => setLoading(false));
  }, []);

  const signIn = async (identity: string, password: string) => {
    const result = await api.post<{ user: User }>("/auth/login", { identity, password });
    setUser(result.user);
  };
  const signOut = async () => { await api.post<void>("/auth/logout"); setUser(null); };
  const completeSetup = async (input: Record<string, string>) => { await api.post("/setup/complete", input); setSetupRequired(false); };

  return <AuthContext.Provider value={{ user, setupRequired, loading, signIn, signOut, completeSetup }}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = React.useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used inside AuthProvider");
  return value;
}

