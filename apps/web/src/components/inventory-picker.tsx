import * as React from "react";
import { api } from "../api";
import { useDebouncedValue, useLoad } from "../hooks";
import type { InventoryItem } from "../types";
import { Input } from "./ui/input";

/**
 * Picks a stock item by searching the server. A dropdown holding every item
 * stops being usable — and stops being complete — the moment a clinic's
 * catalogue outgrows one page, so the list here is always a server-side search
 * result rather than a slice of a prefetched array.
 *
 * A barcode scanner behaves as a keyboard that types the code and presses
 * Enter, so an exact code match on submit selects straight away with no
 * pointer involved.
 */
export function InventoryPicker({ label, category, exclude = [], onSelect }: { label: string; category?: string; exclude?: string[]; onSelect(item: InventoryItem): void }) {
  const [query, setQuery] = React.useState("");
  const debounced = useDebouncedValue(query, 250);
  const results = useLoad(
    () => api.get<{ items: InventoryItem[] }>(`/inventory?q=${encodeURIComponent(debounced)}&category=${encodeURIComponent(category ?? "")}&limit=25`),
    [debounced, category],
  );
  const available = (results.data?.items ?? []).filter((item) => !exclude.includes(item.id));
  const choose = (item: InventoryItem) => { onSelect(item); setQuery(""); };
  const submitExactMatch = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== "Enter") return;
    event.preventDefault();
    const typed = query.trim().toLowerCase();
    const exact = available.find((item) => item.barcode.toLowerCase() === typed || item.sku.toLowerCase() === typed);
    if (exact) choose(exact);
  };
  return <div className="grid gap-2">
    <label className="grid gap-1.5 text-sm font-medium">
      <span>{label}</span>
      <Input value={query} placeholder="Search by name, SKU or barcode…" onChange={(event) => setQuery(event.target.value)} onKeyDown={submitExactMatch} />
    </label>
    <div role="listbox" aria-label={label} className="max-h-48 overflow-y-auto rounded-md border">
      {results.loading && !results.data ? <div className="p-3 text-sm text-[var(--muted-foreground)]">Searching…</div>
        : available.length === 0 ? <div className="p-3 text-sm text-[var(--muted-foreground)]">No matching stock items.</div>
        : available.map((item) => (
          <button key={item.id} type="button" role="option" aria-selected={false} className="flex w-full items-center justify-between gap-3 border-b px-3 py-2 text-left text-sm last:border-b-0 hover:bg-[var(--muted)]" onClick={() => choose(item)}>
            <span className="min-w-0"><span className="block truncate font-semibold">{item.name}</span><span className="font-mono text-xs text-[var(--muted-foreground)]">{item.sku}{item.barcode ? ` · ${item.barcode}` : ""}</span></span>
            <span className="whitespace-nowrap font-mono text-xs">{item.trackStock ? `${item.quantity} in stock` : "service"}</span>
          </button>
        ))}
    </div>
  </div>;
}
