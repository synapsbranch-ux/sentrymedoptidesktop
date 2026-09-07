# Clinical analytics acceptance criteria

These criteria define the behaviour required before the longitudinal analytics and
clinical charts may be treated as complete. They are deliberately conservative: every
derived value is decision support, never a diagnosis, and the original measurement is
always preserved.

## Shared rules

- Analytics read existing consultation, pre-test and prescription data. They never ask
  staff to re-enter a measurement solely for reporting.
- A missing or unparsable value remains missing; it is never converted to zero.
- Every derived refraction identifies its source (`subjective`, `prescription` or
  `autorefraction`). The UI must not silently join values from different sources.
- Thresholds for rapid myopic shift, significant acuity loss, elevated/asymmetric IOP
  and thin/thick cornea are doctor-configurable and validated by the server.
- Raw IOP and central corneal thickness remain visible. Corneal thickness may add a
  directional interpretation, but SentryMed does not silently replace measured IOP
  with an untraceable corrected value.
- Every alert is labelled as a signal for clinician review.

## Longitudinal patient view

1. Refraction charts expose sphere, cylinder and spherical equivalent for OD and OS,
   with source selection and a rapid-progression signal.
2. The visit delta compares visual acuity, IOP, refraction, treatment plan, medication
   prescriptions and diagnoses. Diagnoses and medications identify additions and
   removals.
3. Visual acuity accepts common Snellen, metric/decimal and low-vision notations,
   exposes logMAR, reports lines gained/lost and signals a configured significant loss.
4. IOP exposes the full OD/OS series, latest asymmetry and historical peak.
5. Pachymetry provides corneal-context warnings beside, rather than in place of, the
   measured IOP.

## Clinical charts

6. A fundus mark stores its retinal structure, label, coordinates and derived clock
   hour with correct OD/OS orientation.
7. Confrontation fields use a touch target for each zone and can show the previous
   visit as a grey overlay that can be hidden.
8. Ocular motility uses the nine diagnostic gaze positions and supports grades -4
   through +4 including zero. Cover-test observations are stored separately from the
   motility grade.

## Required verification

- Table-driven Go tests cover parsers, source priority, thresholds and delta semantics.
- API tests cover multiple backdated visits, prescriptions and missing data.
- Frontend tests cover series selection, overlays and clock-hour calculation.
- Existing encounter locking, RBAC, optimistic concurrency, audit and realtime tests
  continue to pass.

