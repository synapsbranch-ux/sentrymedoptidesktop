import * as React from "react";
import { apiGetPage } from "./api";

export function useLoad<T>(loader: () => Promise<T>, dependencies: React.DependencyList = []) {
  const [data, setData] = React.useState<T | null>(null);
  const [error, setError] = React.useState<Error | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [revision, setRevision] = React.useState(0);
  const reload = React.useCallback(() => setRevision((value) => value + 1), []);
  React.useEffect(() => {
    let active = true;
    setLoading(true);
    setError(null);
    loader().then((value) => { if (active) setData(value); }).catch((reason: unknown) => { if (active) setError(reason instanceof Error ? reason : new Error(String(reason))); }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...dependencies, revision]);
  return { data, error, loading, reload, setData };
}


/**
 * Delays a fast-changing value, so a search box issues one request after the
 * user stops typing rather than one per keystroke.
 */
export function useDebouncedValue<T>(value: T, delayMilliseconds = 300) {
  const [debounced, setDebounced] = React.useState(value);
  React.useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delayMilliseconds);
    return () => window.clearTimeout(timer);
  }, [value, delayMilliseconds]);
  return debounced;
}

export interface Paged<T> { items: T[]; page: number; limit: number; total: number; hasMore: boolean }

/**
 * A list that is paged on the server. The page number is state here, but the
 * rows are never held in full: each page is a separate request, so a clinic with
 * ten thousand invoices ships fifty rows over the LAN and not ten thousand.
 *
 * Changing a filter resets to the first page, because page 4 of the previous
 * filter is meaningless under the new one.
 */
export function usePagedList<T>(path: (page: number, limit: number) => string, filterKeys: React.DependencyList = [], pageSize = 50) {
  const [page, setPage] = React.useState(1);
  const key = JSON.stringify(filterKeys);
  React.useEffect(() => { setPage(1); }, [key]);
  const query = useLoad<Paged<T>>(() => apiGetPage<T>(path(page, pageSize)), [key, page, pageSize]);
  const total = query.data?.total ?? 0;
  return {
    ...query,
    page,
    pageSize,
    total,
    lastPage: Math.max(1, Math.ceil(total / pageSize)),
    hasMore: query.data?.hasMore ?? false,
    items: query.data?.items ?? [],
    next: () => setPage((value) => value + 1),
    previous: () => setPage((value) => Math.max(1, value - 1)),
    setPage,
  };
}
