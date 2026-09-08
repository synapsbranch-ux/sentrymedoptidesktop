// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { describeMicrophoneFailure, inspectMicrophoneEnvironment } from "./microphone";

const secure = { isSecureContext: true, hasMediaDevices: true, hasMediaRecorder: true, origin: "https://clinic.local:8787", policyAllowsMicrophone: true };
const insecure = { ...secure, isSecureContext: false, origin: "http://192.168.1.20:8787" };

function domError(name: string) {
  return new DOMException("The request is not allowed by the user agent or platform in the current context.", name);
}

describe("microphone availability", () => {
  it("names the insecure page before a doomed request is made", () => {
    const problem = inspectMicrophoneEnvironment(insecure);
    expect(problem?.reason).toBe("insecure_context");
    expect(problem?.message).toContain("http://192.168.1.20:8787");
    expect(problem?.guidance).toContain("https://");
  });

  it("names a Permissions-Policy denial on an otherwise valid page", () => {
    expect(inspectMicrophoneEnvironment({ ...secure, policyAllowsMicrophone: false })?.reason).toBe("permissions_policy");
  });

  it("names a browser without capture support", () => {
    expect(inspectMicrophoneEnvironment({ ...secure, hasMediaDevices: false })?.reason).toBe("unsupported_browser");
    expect(inspectMicrophoneEnvironment({ ...secure, hasMediaRecorder: false })?.reason).toBe("unsupported_browser");
  });

  it("allows the request when nothing is known to block it", () => {
    expect(inspectMicrophoneEnvironment(secure)).toBeNull();
  });
});

describe("microphone failure translation", () => {
  it("never surfaces the raw browser string", () => {
    for (const name of ["NotAllowedError", "SecurityError", "NotFoundError", "NotReadableError", "AbortError", "OverconstrainedError"]) {
      const problem = describeMicrophoneFailure(domError(name), secure);
      expect(problem.message).not.toContain("not allowed by the user agent");
      expect(problem.guidance.length).toBeGreaterThan(0);
    }
  });

  it("attributes the same NotAllowedError to the cause that actually applies", () => {
    expect(describeMicrophoneFailure(domError("NotAllowedError"), insecure).reason).toBe("insecure_context");
    expect(describeMicrophoneFailure(domError("NotAllowedError"), { ...secure, policyAllowsMicrophone: false }).reason).toBe("permissions_policy");
    const denied = describeMicrophoneFailure(domError("NotAllowedError"), secure);
    expect(denied.reason).toBe("permission_denied");
    expect(denied.guidance).toContain("padlock");
  });

  it("separates a missing microphone from one held by another application", () => {
    expect(describeMicrophoneFailure(domError("NotFoundError"), secure).reason).toBe("no_microphone");
    expect(describeMicrophoneFailure(domError("NotReadableError"), secure).reason).toBe("microphone_busy");
  });
});
