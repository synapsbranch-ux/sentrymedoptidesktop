import type { APIErrorBody } from "./types";

export class APIError extends Error {
  constructor(public status: number, public body: APIErrorBody) { super(body.message); }
  get isConflict() { return this.status === 409 && this.body.code === "CONCURRENT_MODIFICATION"; }
}

let desktopSessionToken = "";

export function setDesktopSessionToken(token: string) {
  desktopSessionToken = token;
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  if (options.body && !(options.body instanceof FormData)) headers.set("Content-Type", "application/json");
  headers.set("Accept", "application/json");
  if (desktopSessionToken) headers.set("Authorization", `SentryMed ${desktopSessionToken}`);
  const controller = options.signal ? null : new AbortController();
  const timer = controller ? window.setTimeout(() => controller.abort(), 20000) : 0;
  let response: Response;
  try {
    response = await fetch(`/api/v1${path}`, { ...options, headers, credentials: "same-origin", signal: options.signal ?? controller?.signal });
  } catch (reason) {
    const timedOut = reason instanceof DOMException && reason.name === "AbortError";
    throw new APIError(0, { code: timedOut ? "REQUEST_TIMEOUT" : "CLINIC_SERVER_UNREACHABLE", message: timedOut ? "The clinic server did not respond in time. Check the Wi-Fi connection and try again." : "Cannot reach the clinic server. Check that this device is on the clinic Wi-Fi and that SentryMed is running." });
  } finally {
    if (timer) window.clearTimeout(timer);
  }
  if (response.status === 204) return undefined as T;
  const type = response.headers.get("content-type") ?? "";
  const body: unknown = type.includes("application/json") ? await response.json() : await response.text();
  if (!response.ok) {
    const error = typeof body === "object" && body !== null && "message" in body ? body as APIErrorBody : { code: "REQUEST_FAILED", message: String(body) };
    if (response.status === 401 && path !== "/auth/login" && path !== "/auth/me") window.dispatchEvent(new CustomEvent("sentrymed:session-expired"));
    throw new APIError(response.status, error);
  }
  return body as T;
}

/**
 * Fetches a stored file as a Blob. Plain <a href> and <img src> cannot carry the
 * desktop shell's in-memory Authorization header, so every document the viewer
 * shows is retrieved here and rendered from an object URL.
 */
async function requestBlob(path: string, signal?: AbortSignal): Promise<{ blob: Blob; filename: string }> {
  const headers = new Headers({ Accept: "*/*" });
  if (desktopSessionToken) headers.set("Authorization", `SentryMed ${desktopSessionToken}`);
  let response: Response;
  try {
    response = await fetch(`/api/v1${path}`, { headers, credentials: "same-origin", signal });
  } catch {
    throw new APIError(0, { code: "CLINIC_SERVER_UNREACHABLE", message: "Cannot reach the clinic server. Check that this device is on the clinic Wi-Fi and that SentryMed is running." });
  }
  if (!response.ok) {
    const type = response.headers.get("content-type") ?? "";
    const body: unknown = type.includes("application/json") ? await response.json() : await response.text();
    const error = typeof body === "object" && body !== null && "message" in body ? body as APIErrorBody : { code: "REQUEST_FAILED", message: String(body) };
    if (response.status === 401) window.dispatchEvent(new CustomEvent("sentrymed:session-expired"));
    throw new APIError(response.status, error);
  }
  const disposition = response.headers.get("content-disposition") ?? "";
  const match = /filename\*?=(?:UTF-8''|")?([^";]+)/i.exec(disposition);
  return { blob: await response.blob(), filename: match ? decodeURIComponent(match[1]) : "" };
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  blob: requestBlob,
  post: <T>(path: string, body?: unknown) => request<T>(path, { method: "POST", body: body instanceof FormData ? body : body === undefined ? undefined : JSON.stringify(body) }),
  // Symmetric with post(): a signature upload/draw sends FormData through PUT
  // (it replaces the one existing record rather than creating a new one), and
  // JSON.stringify()-ing a FormData instance silently serializes to "{}" and
  // sends it as JSON — the server then fails to parse it as multipart and the
  // caller sees an unrelated "too large" error instead of the real problem.
  put: <T>(path: string, body: unknown) => request<T>(path, { method: "PUT", body: body instanceof FormData ? body : JSON.stringify(body) }),
  patch: <T>(path: string, body: unknown) => request<T>(path, { method: "PATCH", body: JSON.stringify(body) }),
  delete: <T>(path: string) => request<T>(path, { method: "DELETE" }),
};
