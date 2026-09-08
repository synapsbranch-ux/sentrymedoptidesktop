// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { hasExpired, recordingRetentionMs } from "./recording-store";

const hour = 60 * 60 * 1000;

describe("recording retention", () => {
  it("keeps audio long enough to survive a weekend", () => {
    // A Friday-evening interruption must still be recoverable on Monday morning.
    expect(recordingRetentionMs).toBeGreaterThanOrEqual(64 * hour);
  });

  it("does not keep patient audio on the device indefinitely", () => {
    const now = Date.now();
    expect(hasExpired({ startedAt: new Date(now - hour).toISOString() }, now)).toBe(false);
    expect(hasExpired({ startedAt: new Date(now - recordingRetentionMs + hour).toISOString() }, now)).toBe(false);
    expect(hasExpired({ startedAt: new Date(now - recordingRetentionMs - hour).toISOString() }, now)).toBe(true);
  });

  it("treats a record with an unusable timestamp as expired rather than keeping it forever", () => {
    expect(hasExpired({ startedAt: "" })).toBe(true);
    expect(hasExpired({ startedAt: "not-a-date" })).toBe(true);
  });
});
