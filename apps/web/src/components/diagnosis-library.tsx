import * as React from "react";
import { BookOpen } from "lucide-react";
import { api } from "../api";
import { useDebouncedValue, useLoad } from "../hooks";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "./ui/dialog";
import { EmptyState, ErrorState, Skeleton, Table, Td, Th } from "./ui/data";
import { Input, Select } from "./ui/input";
import type { CodeEntry } from "./code-search";

interface CodePage { items: (CodeEntry & { source: string })[]; page: number; limit: number; total: number; hasMore: boolean }

/**
 * The full diagnosis reference, browsable rather than only searchable. A
 * clinician who does not know what to type can read down a category; one who
 * does can search it in plain language. Selecting an entry fills the diagnosis
 * fields, so the library is a way into the record and not just a lookup.
 */
export function DiagnosisLibrary({ onSelect }: { onSelect(entry: CodeEntry): void }) {
  const [open, setOpen] = React.useState(false);
  return <Dialog open={open} onOpenChange={setOpen}>
    <DialogTrigger asChild><Button variant="outline" type="button"><BookOpen className="h-4 w-4" />Browse diagnosis library</Button></DialogTrigger>
    <DialogContent className="max-w-4xl">
      <DialogHeader><DialogTitle>Diagnosis reference</DialogTitle><DialogDescription>Search in plain language ("red eye", "blurry vision") or by code, or read down a category. Selecting an entry fills the diagnosis and code fields.</DialogDescription></DialogHeader>
      <LibraryBrowser onSelect={(entry) => { onSelect(entry); setOpen(false); }} />
    </DialogContent>
  </Dialog>;
}

function LibraryBrowser({ onSelect }: { onSelect(entry: CodeEntry): void }) {
  const [query, setQuery] = React.useState(""); const [category, setCategory] = React.useState(""); const [page, setPage] = React.useState(1);
  const debounced = useDebouncedValue(query, 300);
  React.useEffect(() => { setPage(1); }, [debounced, category]);
  const categories = useLoad(() => api.get<{ items: { category: string; count: number }[] }>("/diagnosis-codes/categories"), []);
  const codes = useLoad(() => api.get<CodePage>(`/diagnosis-codes?q=${encodeURIComponent(debounced)}&category=${encodeURIComponent(category)}&page=${page}&limit=25`), [debounced, category, page]);
  return <div className="grid gap-4">
    <div className="grid gap-3 sm:grid-cols-[1fr_220px]">
      <Input value={query} placeholder="Search by complaint or code…" onChange={(event) => setQuery(event.target.value)} />
      <Select value={category} onChange={(event) => setCategory(event.target.value)}><option value="">All categories</option>{categories.data?.items.map((entry) => <option key={entry.category} value={entry.category}>{entry.category} ({entry.count})</option>)}</Select>
    </div>
    {codes.loading ? <Skeleton className="h-72" /> : codes.error ? <ErrorState message={codes.error.message} retry={codes.reload} /> : codes.data?.items.length ? <>
      <div className="max-h-[50vh] overflow-y-auto"><Table><thead><tr><Th>Code</Th><Th>Description</Th><Th>Also called</Th><Th>{""}</Th></tr></thead><tbody>{codes.data.items.map((entry) => <tr key={entry.code} className="hover:bg-[var(--muted)]"><Td className="whitespace-nowrap font-mono text-xs font-bold">{entry.code}</Td><Td><strong>{entry.description}</strong><div className="text-xs text-[var(--muted-foreground)]">{entry.category}{entry.source === "clinic" ? " · clinic list" : ""}</div></Td><Td className="text-xs text-[var(--muted-foreground)]">{(entry.synonyms ?? []).slice(0, 3).join(" · ")}</Td><Td><Button size="sm" variant="outline" onClick={() => onSelect(entry)}>Use</Button></Td></tr>)}</tbody></Table></div>
      <div className="flex items-center justify-between text-sm text-[var(--muted-foreground)]"><span>{codes.data.total} matching codes</span><span className="flex gap-2"><Button size="sm" variant="outline" disabled={page === 1} onClick={() => setPage(page - 1)}>Previous</Button><Button size="sm" variant="outline" disabled={!codes.data.hasMore} onClick={() => setPage(page + 1)}>Next</Button></span></div>
    </> : <EmptyState title="No matching codes" description="Try the complaint in plain language, or clear the category filter." />}
  </div>;
}
