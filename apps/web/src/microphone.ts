/**
 * `getUserMedia` refusals are reported by the browser, not by this application.
 * "The request is not allowed by the user agent or platform in the current
 * context" covers three unrelated causes, so the reason is established here and
 * turned into an instruction staff can act on instead of a raw browser string.
 */

export type MicrophoneBlockReason =
  | "insecure_context"
  | "unsupported_browser"
  | "permission_denied"
  | "permissions_policy"
  | "desktop_shell_blocked"
  | "no_microphone"
  | "microphone_busy"
  | "unknown";

export interface MicrophoneProblem {
  reason: MicrophoneBlockReason;
  message: string;
  /** How the clinic gets the microphone working again. */
  guidance: string;
}

export interface MicrophoneEnvironment {
  isSecureContext: boolean;
  hasMediaDevices: boolean;
  hasMediaRecorder: boolean;
  origin: string;
  /** Result of a Permissions-Policy check, when the browser exposes one. */
  policyAllowsMicrophone: boolean | null;
  /**
   * True inside the Wails desktop shell. Its webview has no address bar and no
   * site settings, so the browser instructions below cannot be followed there —
   * and on Linux the WebKitGTK webview refuses microphone access outright
   * rather than prompting.
   */
  isDesktopShell: boolean;
  /** "linux", "windows", "macos" or "" when it cannot be told apart. */
  platform: string;
}

const httpsGuidance =
  "Open SentryMed over its https:// address. Browsers block the microphone on a plain http:// page from anywhere except this computer's own localhost address. The clinic HTTPS address and its trust certificate are in System → Network.";

const permissionGuidance =
  "The microphone is blocked for this site in this browser profile. Select the padlock or camera icon in the address bar, set Microphone to Allow, then reload the page. On iOS also check Settings → Safari → Microphone.";

// The desktop window has no address bar and no per-site settings, so the browser
// instructions are useless there. On Linux in particular the WebKitGTK webview
// the desktop shell embeds declines microphone access without ever prompting,
// which is exactly the "Recording is not available on this device yet" the
// clinic sees. Recording from the browser on the same machine works, because
// there the prompt does appear.
const desktopGuidance =
  "The SentryMed desktop window cannot capture audio on this computer — its built-in webview declines the microphone without asking. Use \"Open in browser to record\" below: the clinic page opens in your normal browser, which will ask for the microphone. Everything else in the desktop window keeps working, and the recording is saved to the same consultation.";

const policyGuidance =
  "A Permissions-Policy header or an embedding frame is withholding the microphone from this page. If SentryMed is embedded in another page, that page's iframe needs allow=\"microphone\".";

export function readMicrophoneEnvironment(): MicrophoneEnvironment {
  const documentPolicy = (document as unknown as {
    featurePolicy?: { allowsFeature(feature: string): boolean };
    permissionsPolicy?: { allowsFeature(feature: string): boolean };
  });
  const policy = documentPolicy.permissionsPolicy ?? documentPolicy.featurePolicy;
  let policyAllowsMicrophone: boolean | null = null;
  try {
    if (policy && typeof policy.allowsFeature === "function") policyAllowsMicrophone = policy.allowsFeature("microphone");
  } catch {
    policyAllowsMicrophone = null;
  }
  return {
    isSecureContext: window.isSecureContext !== false,
    hasMediaDevices: typeof navigator !== "undefined" && Boolean(navigator.mediaDevices?.getUserMedia),
    hasMediaRecorder: typeof MediaRecorder !== "undefined",
    origin: window.location.origin,
    policyAllowsMicrophone,
    isDesktopShell: Boolean((window as { go?: unknown }).go),
    platform: detectPlatform(),
  };
}

function detectPlatform(): string {
  const agent = typeof navigator === "undefined" ? "" : `${navigator.userAgent} ${navigator.platform ?? ""}`.toLowerCase();
  if (agent.includes("android")) return "android";
  if (/iphone|ipad|ipod/.test(agent)) return "ios";
  if (agent.includes("win")) return "windows";
  if (agent.includes("mac")) return "macos";
  if (agent.includes("linux") || agent.includes("x11")) return "linux";
  return "";
}

/**
 * What the browser says about the microphone permission before it is asked for.
 * "prompt" means the request has never reached the person — which is how a
 * webview that declines silently is told apart from a person who said no.
 */
export async function readMicrophonePermissionState(): Promise<PermissionState | "unavailable"> {
  const permissions = typeof navigator === "undefined" ? undefined : navigator.permissions;
  if (!permissions?.query) return "unavailable";
  try {
    return (await permissions.query({ name: "microphone" as PermissionName })).state;
  } catch {
    return "unavailable";
  }
}

/**
 * Returns the problem that will stop a recording before `getUserMedia` is even
 * called, or null when the request is worth attempting.
 */
export function inspectMicrophoneEnvironment(environment: MicrophoneEnvironment = readMicrophoneEnvironment()): MicrophoneProblem | null {
  if (!environment.isSecureContext && environment.isDesktopShell)
    return { reason: "desktop_shell_blocked", message: "The desktop window's built-in webview does not treat its own page as a secure context, so it will not allow microphone access.", guidance: desktopGuidance };
  if (!environment.isSecureContext)
    return {
      reason: "insecure_context",
      message: `This page is served over an insecure connection (${environment.origin}), so the browser refuses microphone access before SentryMed can ask for it.`,
      guidance: httpsGuidance,
    };
  if (environment.policyAllowsMicrophone === false)
    return { reason: "permissions_policy", message: "This page is not permitted to use the microphone.", guidance: policyGuidance };
  if (!environment.hasMediaDevices)
    return {
      reason: "unsupported_browser",
      message: "This browser does not expose microphone capture to the page.",
      guidance: "Use Chrome, Edge or Safari, kept up to date, or record from the SentryMed desktop application.",
    };
  if (!environment.hasMediaRecorder)
    return {
      reason: "unsupported_browser",
      message: "This browser cannot record audio (MediaRecorder is unavailable).",
      guidance: "Use Chrome, Edge or Safari, kept up to date, or record from the SentryMed desktop application.",
    };
  return null;
}

/** Turns a rejected `getUserMedia` promise into the same actionable shape. */
export function describeMicrophoneFailure(reason: unknown, environment: MicrophoneEnvironment = readMicrophoneEnvironment()): MicrophoneProblem {
  const name = reason instanceof DOMException ? reason.name : (reason as { name?: string } | null)?.name ?? "";
  switch (name) {
    case "NotAllowedError":
    case "SecurityError":
      // Chrome and Safari both report a refusal caused by an insecure page as
      // NotAllowedError, so the context has to be re-checked to name the cause.
      if (!environment.isSecureContext)
        return { reason: "insecure_context", message: `The browser refused microphone access because this page is served over an insecure connection (${environment.origin}).`, guidance: httpsGuidance };
      if (environment.policyAllowsMicrophone === false)
        return { reason: "permissions_policy", message: "This page is not permitted to use the microphone.", guidance: policyGuidance };
      if (environment.isDesktopShell)
        return { reason: "desktop_shell_blocked", message: "The desktop window's built-in webview declined microphone access.", guidance: desktopGuidance };
      return { reason: "permission_denied", message: "Microphone access was declined for this site.", guidance: permissionGuidance };
    case "NotFoundError":
    case "OverconstrainedError":
      return { reason: "no_microphone", message: "No microphone was found on this device.", guidance: "Connect a microphone or headset, then select Start recording again." };
    case "NotReadableError":
    case "AbortError":
      return { reason: "microphone_busy", message: "The microphone could not be opened. Another application may be using it.", guidance: "Close other applications that use the microphone (video calls, voice recorders), then try again." };
    default:
      return {
        reason: "unknown",
        message: reason instanceof Error && reason.message ? reason.message : "Microphone access failed.",
        guidance: "Reload the page and try again. If it keeps failing, note this message and contact clinic support.",
      };
  }
}

/**
 * Requests the microphone explicitly. Resolves with the stream, or with the
 * reason the clinic cannot record — never with a raw browser error string.
 */
export async function requestMicrophone(): Promise<{ stream: MediaStream } | { problem: MicrophoneProblem }> {
  const environment = readMicrophoneEnvironment();
  const blocked = inspectMicrophoneEnvironment(environment);
  if (blocked) return { problem: blocked };
  try {
    return { stream: await navigator.mediaDevices.getUserMedia({ audio: true }) };
  } catch (reason) {
    return { problem: describeMicrophoneFailure(reason, environment) };
  }
}
