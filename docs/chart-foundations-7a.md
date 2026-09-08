# Clinical chart foundations — phase 7A

This increment prepares the existing consultation charts for the planned 2D,
2.5D and 3D views. It delivers persistence and review workflows; it does not
introduce an anatomical projection or a 3D renderer.

## Delivered behavior

- Each saved chart has immutable revisions, including notes, examination status,
  author and timestamp. Revisions are created by database triggers in the same
  transaction as the chart write.
- Migration 011 preserves existing JSON, coordinates and versions unchanged.
  Its initial snapshot is labelled as a migration baseline; earlier revisions
  cannot be reconstructed.
- Annotations receive a UUID and a longitudinal tracking UUID. Existing marks
  without IDs receive deterministic IDs when read. A clinician can explicitly
  bring a previous finding into the current draft for re-examination. The server
  checks that its tracking reference belongs to the same patient, chart and eye.
- The previous chart is selected separately for each chart type and eye. A more
  recent OS chart no longer hides an older OD comparison.
- History is available to authenticated clinical users through
  `GET /api/v1/encounters/{id}/eye-diagrams/{chartType}/{eye}/history`.
  It returns 20 revisions per page, with `nextOffset` for pagination.
- The doctor explicitly selects an examination status: unspecified, not
  examined, no findings recorded, or findings recorded. An empty chart does not
  automatically assert normality or resolution.
- Drafts stay in memory while switching charts or consultation tabs. Closing a
  consultation with unsaved chart changes requests confirmation. Browser unload
  uses its native unsaved-change prompt; drafts are not persisted across a crash
  or a full reload.
- A version conflict preserves the local draft, displays the server version,
  and requires an explicit discard or reviewed rebase before another save.
  A failed request also retains the draft. Edits made while saving remain unsaved
  until the doctor saves them explicitly.
- Signed or archived encounters reject chart writes inside SQLite, including a
  finalization that races with an in-flight request.

## Compatibility and boundaries

The API reports schema version 2 while retaining the existing x/y mark format.
The coordinate-system identifier is `legacy-svg-percent-v1`: these are schematic
SVG coordinates, not millimetres or patient-specific anatomy. Unsupported
coordinate systems are rejected. A client that omits the examination status
cannot preserve a contradictory normal/not-examined status when adding marks.

History is a read-only list of saved observations, not a diagnostic diff. A mark
missing from a later revision is not automatically labelled resolved. Historical
follow-up creates a draft and never silently asserts that a lesion persists.
Amsler history is readable here; acquisition stays in the existing vision-test
workflow.

## Next integration work

1. Define versioned anatomical anchors and explicit OD/OS orientation for new
   marks, with a lossless legacy display path.
2. Build the 2.5D layer and section views over those anchors.
3. Add a lazy-loaded 3D view with controlled picking, shared selection and a
   fully functional 2D fallback when graphics support is unavailable.
4. Validate orientation and projection against clinician-reviewed cases before
   enabling 3D editing. Do not infer physical depth or dimensions from old SVG
   points.

## Verification

Backend tests cover legacy migration, immutable revisions, explicit status,
tracking scope, previous-by-eye lookup, optimistic conflicts and the database
signature lock. Frontend tests cover draft preservation, request failure,
in-flight edits and explicit conflict resolution. The mobile browser scenario
exercises consultation tabs, the close confirmation, a second-device conflict,
persisted history and horizontal fit on a phone. Notes have explicit eye-specific
accessible names.

Run `go test ./...`, `go test -race ./internal/server ./internal/database
./internal/backup`, then `npm test -- --run`, `npm run build`,
`npm run typecheck:e2e` and `npm run test:e2e` in `apps/web` for the frontend
commands.

An initial run of the pre-existing optical-delivery browser scenario returned
`LAB_STATUS_FAILED` once. The subsequent complete three-scenario run passed.
This intermittent lab-status failure has not been diagnosed by this chart change.

This branch is based on phase 4–6 (PR #4); merge that prerequisite first.
