// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "./api";
import { usePagedList } from "./hooks";
import { I18nProvider } from "./i18n";
import { Pager } from "./components/ui/data";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

function Rows({ filter }: { filter: string }) {
  const list = usePagedList<{ id: string }>((page, limit) => `/things?filter=${filter}&page=${page}&limit=${limit}`, [filter], 2);
  return <I18nProvider>
    <ul>{list.items.map((item) => <li key={item.id}>{item.id}</li>)}</ul>
    <Pager page={list.page} pageSize={list.pageSize} total={list.total} hasMore={list.hasMore} onPrevious={list.previous} onNext={list.next} />
  </I18nProvider>;
}

// The i18n provider fetches its own settings through the same client, so only
// the list's requests are counted here.
const listCalls = (get: { mock: { calls: unknown[][] } }) => get.mock.calls.map((call) => String(call[0])).filter((path) => path.startsWith("/things"));

function stubPages() {
  return vi.spyOn(api, "get").mockImplementation((path: string) => {
    if (!path.startsWith("/things")) return Promise.resolve({ items: [] } as never);
    const page = Number(new URL(path, "http://x").searchParams.get("page") ?? 1);
    const items = page === 1 ? [{ id: "a" }, { id: "b" }] : [{ id: "c" }];
    return Promise.resolve({ items, page, limit: 2, total: 3, hasMore: page * 2 < 3 } as never);
  });
}

describe("usePagedList", () => {
  it("fetches one page rather than the whole list", async () => {
    const get = stubPages();
    render(<Rows filter="all" />);
    await waitFor(() => expect(screen.getByText("a")).toBeTruthy());
    expect(screen.queryByText("c")).toBeNull();
    expect(listCalls(get)).toHaveLength(1);
    expect(listCalls(get)[0]).toContain("limit=2");
  });

  it("asks the server for the next page instead of slicing a loaded array", async () => {
    const get = stubPages();
    render(<Rows filter="all" />);
    await waitFor(() => expect(screen.getByText("a")).toBeTruthy());
    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Next" })); });
    await waitFor(() => expect(screen.getByText("c")).toBeTruthy());
    expect(listCalls(get)[1]).toContain("page=2");
    expect(screen.queryByText("a")).toBeNull();
  });

  it("returns to the first page when a filter changes", async () => {
    const get = stubPages();
    const view = render(<Rows filter="all" />);
    await waitFor(() => expect(screen.getByText("a")).toBeTruthy());
    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Next" })); });
    await waitFor(() => expect(screen.getByText("c")).toBeTruthy());
    view.rerender(<Rows filter="narrowed" />);
    await waitFor(() => expect(listCalls(get).at(-1)).toContain("filter=narrowed&page=1"));
  });

  it("shows which rows are on screen out of how many exist", async () => {
    stubPages();
    render(<Rows filter="all" />);
    await waitFor(() => expect(screen.getByText(/1–2 of 3/)).toBeTruthy());
  });

  it("stops offering a next page on the last one", async () => {
    stubPages();
    render(<Rows filter="all" />);
    await waitFor(() => expect(screen.getByText("a")).toBeTruthy());
    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Next" })); });
    await waitFor(() => expect(screen.getByRole("button", { name: "Next" }).hasAttribute("disabled")).toBe(true));
  });
});
