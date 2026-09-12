/**
 * Parts of the clinic application that are finished and working, but which the
 * clinic has asked to keep out of the interface for now.
 *
 * Nothing here deletes anything. The pages, their components and the server
 * endpoints behind them are untouched and still covered by their tests; only
 * the ways a user could reach them are withheld — the sidebar entry and the
 * route. Set an entry back to `true` and the feature comes back exactly as it
 * was, with its data intact, because the records were never stopped from being
 * written or read.
 */
export const featureVisibility = {
  /** Quotes: the estimate a patient is given before committing to a sale. */
  quotes: false,
  /** Vision testing: the acuity/colour/Amsler screens driven from a second display. */
  visionTesting: false,
  /**
   * The audit log screen. Withholding the screen does not stop auditing: every
   * action staff take is still recorded on the clinic server exactly as before,
   * and a backup still carries the whole trail.
   */
  auditLog: false,
} as const;

export type FeatureName = keyof typeof featureVisibility;

export function featureVisible(name: FeatureName): boolean {
  return featureVisibility[name];
}
