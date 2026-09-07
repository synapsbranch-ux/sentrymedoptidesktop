import * as React from "react";
import { api } from "../api";
import { useLoad } from "../hooks";
import { Input } from "./ui/input";

export interface CodeEntry { code: string; description: string }

export function CodeCombobox({ endpoint, placeholder, onSelect }: { endpoint: "/codes/icd10" | "/codes/procedures"; placeholder: string; onSelect(entry: CodeEntry): void }) {
  const [query, setQuery] = React.useState("");
  const [open, setOpen] = React.useState(false);
  const results = useLoad(
    () => (query.trim().length >= 2 ? api.get<{ items: CodeEntry[] }>(`${endpoint}?q=${encodeURIComponent(query)}`) : Promise.resolve({ items: [] })),
    [endpoint, query],
  );
  return (
    <div className="relative">
      <Input
        value={query}
        placeholder={placeholder}
        onChange={(event) => { setQuery(event.target.value); setOpen(true); }}
        onFocus={() => setOpen(true)}
        onBlur={() => window.setTimeout(() => setOpen(false), 150)}
      />
      {open && results.data && results.data.items.length > 0 && (
        <div className="absolute z-20 mt-1 max-h-56 w-full overflow-y-auto rounded-md border bg-[var(--card)] shadow-lg">
          {results.data.items.map((item) => (
            <button
              key={item.code}
              type="button"
              className="block w-full px-3 py-2 text-left text-sm hover:bg-zinc-50"
              onMouseDown={() => {
                onSelect(item);
                setQuery("");
                setOpen(false);
              }}
            >
              <span className="font-mono font-bold">{item.code}</span> — {item.description}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
