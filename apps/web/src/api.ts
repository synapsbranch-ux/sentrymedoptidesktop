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

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) => request<T>(path, { method: "POST", body: body instanceof FormData ? body : body === undefined ? undefined : JSON.stringify(body) }),
  put: <T>(path: string, body: unknown) => request<T>(path, { method: "PUT", body: JSON.stringify(body) }),
  patch: <T>(path: string, body: unknown) => request<T>(path, { method: "PATCH", body: JSON.stringify(body) }),
  delete: <T>(path: string) => request<T>(path, { method: "DELETE" }),
};
