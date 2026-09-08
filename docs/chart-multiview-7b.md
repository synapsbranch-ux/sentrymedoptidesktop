# Consultation chart views: 2D, 2.5D and 3D

The anterior-segment and fundus editors now offer three views for each eye.
Confrontation fields, motility and Amsler keep their existing clinical grids.

## Workflow for the doctor

1. Place a finding on the original 2D chart and choose its recorded structure.
2. Select a finding from its point or the finding list. Selecting a point no
   longer deletes it; deletion uses the explicit Remove control.
3. Switch to 2.5D for a shaded sagittal section or to 3D for a rotatable model.
   A supported, explicitly recorded structure is highlighted in these views.
4. Explore structures through the illustration or the accessible structure
   buttons. Exploration changes the view selection only, never the record.
5. Use rotation, tilt, zoom, cutaway, isolation and Reset view in 3D. Touch users
   can select structures and use sliders without trapping the page's scroll.
6. Edit a selected finding's description or recorded structure and explicitly
   Save chart. Its ID, tracking ID and original x/y position are preserved.

The full draft, notes, revision history, concurrency checks and signature lock
from phase 7A remain shared by all views. Signed charts can be explored but their
clinical controls are disabled.

## Clinical meaning and limits

`generic-eye-v1` is an illustrative atlas, not patient-specific imaging or a
diagnostic reconstruction. Its named structures are cornea, iris, lens, sclera,
vitreous, retina, macula, optic disc and optic nerve. It does not model every
structure or retinal layer. The cross-section does not locate the macula or
optic disc; the interface states this when either is selected.

The atlas maps only explicitly named structures. For example, `retina` has a
region; `haemorrhage`, `drusen`, `cup_disc_ratio` and `other` do not supply a
location and are not automatically mapped. Exploring or highlighting the retina
does not imply a lesion affecting its entire surface.

No legacy point is projected onto a globe. The exact recorded location remains
on the original 2D chart; no depth, lesion size, millimetres or patient anatomy
are inferred. Changing view creates no database write or revision. This is why
this increment requires no new schema migration.

The atlas uses +Y superior and +Z anterior. The nasal axis is mirrored between
OD and OS, consistently with the existing paired fundus drawing. The section's
horizontal axis is anterior/posterior, not nasal/temporal. Dimensions are generic
illustration units. Clinician review remains necessary before using a future
patient-specific projection or enabling direct placement of lesions in 3D.

## Implementation and device behavior

- `chart-anatomy.ts`: versioned atlas regions, explicit mapping, mirrored
  landmarks and bounded camera navigation.
- `chart-section.tsx`: SVG section with shaded depth cues and structure picking.
- `chart-eye-3d.tsx`: real Three.js/WebGL2 geometry, clipping, ray-based picking,
  lighting and camera controls. All geometry is generated locally; there are no
  external model files, textures or runtime CDN requests.
- `chart-atlas.tsx`: common explorer controls and a lazy-loading error boundary.
- `clinical-charts.tsx`: shared clinical selection and editing, with inverse SVG
  screen transforms for accurate 2D placement under resizing/letterboxing.

The 3D code is fetched only when the doctor opens 3D. Its production chunk is
approximately 519 kB minified / 132 kB gzip; ordinary 2D use does not load it.
Rendering is on demand, not a continuous animation. Pixel ratio is capped at
1.5. Observers, pointer listeners, GPU geometry, materials and contexts are
released when the view closes. A failed module load, WebGL initialization error,
render exception or lost context returns to the existing 2D editor with the draft
intact. Sliders and named structure buttons provide alternatives to dragging
and precise canvas picking.

## Validation

- Unit tests verify explicit mapping, OD/OS landmark mirroring and camera bounds.
- Mobile Chromium tests use a real WebGL2 software renderer. They verify lazy
  loading, shared selection, 2.5D/3D navigation, touch picking, reset, unchanged
  coordinates and identities, saved shared edits, and signed read-only behavior.
- Error tests verify recovery from a failed 3D module, unavailable WebGL and lost contexts while
  retaining an unsaved note.
- The existing mobile consultation/draft/conflict and clinic workflow scenarios
  remain in the suite. The 3D screenshot is visually inspected during development.

This is technical browser verification, not clinical validation or a physical
device performance study.

References for implementation and the basic illustrative anatomy:

- https://threejs.org/docs/
- https://threejs.org/manual/en/cleanup.html
- https://www.nei.nih.gov/eye-health-information/healthy-vision/how-eyes-work

Integration note: PR #5 was merged into `codex/clinical-phases-4-6` after that
branch's earlier main merge. This branch includes that merge and current main,
so its PR to main also carries the previously reviewed phase 7A foundations.
