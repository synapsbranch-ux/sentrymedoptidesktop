import * as React from "react";
import { api } from "../api";
import { useDebouncedValue, useLoad } from "../hooks";
import { Input } from "./ui/input";

export interface CodeEntry { code: string; description: string; category?: string; synonyms?: string[] }

export function CodeCombobox({ endpoint, placeholder, onSelect }: { endpoint: "/codes/icd10" | "/codes/procedures"; placeholder: string; onSelect(entry: CodeEntry): void }) {
  const [query, setQuery] = React.useState("");
  const [open, setOpen] = React.useState(false);
  // Search once the clinician stops typing, rather than on every keystroke.
  const debounced = useDebouncedValue(query, 300);
  const results = useLoad(
    () => (debounced.trim().length >= 2 ? api.get<{ items: CodeEntry[] }>(`${endpoint}?q=${encodeURIComponent(debounced)}`) : Promise.resolve({ items: [] })),
    [endpoint, debounced],
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
        <div role="listbox" className="absolute z-20 mt-1 max-h-56 w-full overflow-y-auto rounded-md border bg-[var(--card)] shadow-lg">
          {results.data.items.map((item) => (
            <button
              key={item.code}
              type="button"
              role="option"
              aria-selected={false}
              className="block w-full px-3 py-2 text-left text-sm hover:bg-zinc-50"
              // mousedown only stops the input's blur from closing the list; the
              // selection happens on click, which keyboard and assistive-technology
              // activation also produce.
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => {
                onSelect(item);
                setQuery("");
                setOpen(false);
              }}
            >
              <span className="font-mono font-bold">{item.code}</span> — {item.description}
              {/* The lay terms that matched are shown so it is clear why an entry
                  came back for a plain-language search such as "red eye". */}
              {item.synonyms && item.synonyms.length > 0 && <span className="mt-0.5 block truncate text-xs text-[var(--muted-foreground)]">{item.synonyms.slice(0, 4).join(" · ")}</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
