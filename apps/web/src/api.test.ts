// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "./api";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function stubFetch(status = 200, body: unknown = {}) {
  const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

/**
 * Regression: api.put() unconditionally JSON.stringify()'d its body, unlike
 * api.post(), which already special-cased FormData. That's invisible in any
 * test that mocks api.put itself (the signature save flow's tests did exactly
 * that), so this exercises the real request() body-encoding logic against a
 * mocked fetch — the layer where the bug actually lived. A FormData body must
 * reach fetch untouched, with no Content-Type set (the browser adds the
 * multipart boundary itself); a plain object must still be JSON-encoded.
 */
describe("api.put body encoding", () => {
  it("sends a FormData body through untouched, not JSON.stringify()'d", async () => {
    const fetchMock = stubFetch();
    const form = new FormData();
    form.append("method", "drawn");
    form.append("signature", new Blob(["png-bytes"], { type: "image/png" }), "signature.png");

    await api.put("/me/signature", form);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, init] = fetchMock.mock.calls[0];
    if (!init) throw new Error("fetch was not called with init");
    expect(init.body).toBe(form);
    const headers = new Headers(init.headers);
    // Setting Content-Type here would strip fetch's auto-generated multipart
    // boundary and break parsing server-side, exactly like api.post() avoids.
    expect(headers.has("Content-Type")).toBe(false);
  });

  it("still JSON-encodes a plain object body", async () => {
    const fetchMock = stubFetch();
    await api.put("/settings/clinic", { value: { name: "Clinic" }, version: 1 });

    const [, init] = fetchMock.mock.calls[0];
    if (!init) throw new Error("fetch was not called with init");
    expect(init.body).toBe(JSON.stringify({ value: { name: "Clinic" }, version: 1 }));
    const headers = new Headers(init.headers);
    expect(headers.get("Content-Type")).toBe("application/json");
  });
});
