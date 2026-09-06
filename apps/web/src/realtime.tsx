import * as React from "react";
import { api } from "./api";
import { desktopBridge } from "./native";

interface RealtimeValue { connected: boolean; revision: number }
const RealtimeContext = React.createContext<RealtimeValue>({ connected: false, revision: 0 });

export function RealtimeProvider({ children }: { children: React.ReactNode }) {
  const [connected, setConnected] = React.useState(false);
  const [revision, setRevision] = React.useState(0);
  React.useEffect(() => {
    if (desktopBridge()) {
      let active = true;
      let serverRevision: number | null = null;
      const poll = async () => {
        try {
          const result = await api.get<{ revision: number }>("/events/revision");
          if (!active) return;
          setConnected(true);
          if (serverRevision !== null && result.revision !== serverRevision) setRevision((value) => value + 1);
          serverRevision = result.revision;
        } catch {
          if (active) setConnected(false);
        }
      };
      void poll();
      const timer = window.setInterval(poll, 2500);
      return () => { active = false; window.clearInterval(timer); };
    }
    const stream = new EventSource("/api/v1/events");
    stream.addEventListener("connected", () => setConnected(true));
    stream.addEventListener("update", () => setRevision((value) => value + 1));
    stream.onerror = () => setConnected(false);
    return () => stream.close();
  }, []);
  return <RealtimeContext.Provider value={{ connected, revision }}>{children}</RealtimeContext.Provider>;
}

export function useRealtime() { return React.useContext(RealtimeContext); }
