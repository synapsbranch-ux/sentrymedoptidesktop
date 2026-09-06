import * as React from "react";
import { FileText, Glasses, PackageSearch, Search, UserRound } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { api } from "../api";
import { Dialog, DialogContent, DialogTitle } from "./ui/dialog";
import { Input } from "./ui/input";

interface Result { type: string; id: string; primary: string; secondary: string }

export function CommandPalette({ open, onOpenChange }: { open: boolean; onOpenChange(open: boolean): void }) {
  const [query, setQuery] = React.useState("");
  const [items, setItems] = React.useState<Result[]>([]);
  const navigate = useNavigate();

  React.useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        onOpenChange(!open);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onOpenChange]);

  React.useEffect(() => {
    if (query.trim().length < 2) { setItems([]); return; }
    const timer = window.setTimeout(() => {
      api.get<{ items: Result[] }>(`/search?q=${encodeURIComponent(query)}`)
        .then((result) => setItems(result.items))
        .catch(() => setItems([]));
    }, 180);
    return () => clearTimeout(timer);
  }, [query]);

  const openItem = (item: Result) => {
    const route = item.type === "patient" ? `/patients?id=${item.id}` : item.type === "invoice" ? `/billing?id=${item.id}` : item.type === "lab_order" ? "/lab" : "/inventory";
    navigate(route);
    onOpenChange(false);
  };
  const icons: Record<string, React.ReactNode> = { patient: <UserRound />, invoice: <FileText />, lab_order: <Glasses />, inventory: <PackageSearch /> };

  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent className="p-0">
      <DialogTitle className="sr-only">Clinic search</DialogTitle>
      <div className="flex items-center border-b px-4"><Search className="h-5 w-5 shrink-0 text-zinc-400" /><Input autoFocus className="min-w-0 border-0 shadow-none focus-visible:ring-0" placeholder="Search the clinic…" value={query} onChange={(event) => setQuery(event.target.value)} /></div>
      <div className="max-h-[min(24rem,70dvh)] overflow-y-auto p-2">
        {query.length < 2 && <p className="p-6 text-center text-sm text-zinc-500">Type at least two characters.</p>}
        {query.length >= 2 && items.length === 0 && <p className="p-6 text-center text-sm text-zinc-500">No matching records.</p>}
        {items.map((item) => <button key={`${item.type}-${item.id}`} onClick={() => openItem(item)} className="flex w-full min-w-0 items-center gap-3 rounded-md p-3 text-left hover:bg-zinc-100"><span className="grid h-8 w-8 shrink-0 place-items-center rounded border text-zinc-500 [&>svg]:h-4 [&>svg]:w-4">{icons[item.type]}</span><span className="min-w-0 flex-1"><span className="block truncate text-sm font-semibold">{item.primary}</span><span className="block truncate text-xs text-zinc-500">{item.secondary}</span></span><span className="hidden shrink-0 text-[10px] font-bold uppercase tracking-wide text-zinc-400 sm:block">{item.type.replace("_", " ")}</span></button>)}
      </div>
    </DialogContent>
  </Dialog>;
}
