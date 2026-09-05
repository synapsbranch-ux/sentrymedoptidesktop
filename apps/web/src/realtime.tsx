import * as React from "react";

interface RealtimeValue { connected: boolean; revision: number }
const RealtimeContext = React.createContext<RealtimeValue>({ connected: false, revision: 0 });

export function RealtimeProvider({ children }: { children: React.ReactNode }) {
  const [connected, setConnected] = React.useState(false);
  const [revision, setRevision] = React.useState(0);
  React.useEffect(() => {
    const stream = new EventSource("/api/v1/events");
    stream.addEventListener("connected", () => setConnected(true));
    stream.addEventListener("update", () => setRevision((value) => value + 1));
    stream.onerror = () => setConnected(false);
    return () => stream.close();
  }, []);
  return <RealtimeContext.Provider value={{ connected, revision }}>{children}</RealtimeContext.Provider>;
}

export function useRealtime() { return React.useContext(RealtimeContext); }

