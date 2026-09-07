import * as React from "react";
import { api } from "./api";
import { desktopBridge } from "./native";

interface RealtimeValue { connected: boolean; revision: number; brandingRevision: number }
const RealtimeContext = React.createContext<RealtimeValue>({ connected: false, revision: 0, brandingRevision: 0 });

export function RealtimeProvider({ children }: { children: React.ReactNode }) {
  const [connected, setConnected] = React.useState(false);
  const [revision, setRevision] = React.useState(0);
  // brandingRevision only advances for branding-entity events, so the clinic logo (an <img> whose
  // src is cache-busted by this value) is not re-fetched on every unrelated clinic write.
  const [brandingRevision, setBrandingRevision] = React.useState(0);
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
    let active = true;
    let serverRevision: number | null = null;
    const poll = async () => {
      try {
        const result = await api.get<{ revision: number }>("/events/revision");
        if (!active) return;
        setConnected(true);
        if (serverRevision !== null && result.revision !== serverRevision) setRevision((value) => value + 1);
        serverRevision = result.revision;
      } catch { if (active) setConnected(false); }
    };
    const stream = new EventSource("/api/v1/events");
    stream.addEventListener("connected", () => setConnected(true));
    stream.addEventListener("update", (event) => {
      setRevision((value) => value + 1);
      try {
        const payload = JSON.parse((event as MessageEvent<string>).data) as { entityType?: string };
        if (payload.entityType === "branding") setBrandingRevision((value) => value + 1);
      } catch {
        // Malformed or unparsable event payloads simply skip the branding-specific refresh.
      }
    });
    stream.onerror = () => setConnected(false);
    void poll();
    const timer = window.setInterval(poll, 5000);
    return () => { active = false; window.clearInterval(timer); stream.close(); };
  }, []);
  return <RealtimeContext.Provider value={{ connected, revision, brandingRevision }}>{children}</RealtimeContext.Provider>;
}

export function useRealtime() { return React.useContext(RealtimeContext); }
