-- The pre-test is preparatory, not a gate. Until now the only record of where a
-- consultation stood was the waiting-room queue entry, so a consultation started
-- outside the waiting room had nowhere to move to when the pre-test finished,
-- and a completed pre-test pushed the queue *backwards* from 'in_consultation'
-- to 'waiting_doctor'. The consultation now carries its own stage.
ALTER TABLE encounters ADD COLUMN workflow_stage TEXT NOT NULL DEFAULT 'doctor_exam';

-- A pre-test that is deliberately not performed is recorded as skipped, with who
-- skipped it and why, rather than being left indistinguishable from one nobody
-- has got to yet.
ALTER TABLE pretests ADD COLUMN skipped_at TEXT;
ALTER TABLE pretests ADD COLUMN skipped_by TEXT REFERENCES users(id);
ALTER TABLE pretests ADD COLUMN skip_reason TEXT;

-- Existing draft consultations whose pre-test is still open start in the
-- pre-test stage; everything else is already past it. Finalized rows are left
-- untouched: the lock trigger refuses any update to them, and their status
-- already says where they are.
UPDATE encounters SET workflow_stage='pre_test'
 WHERE status='draft' AND archived_at IS NULL
   AND EXISTS (SELECT 1 FROM pretests p WHERE p.encounter_id=encounters.id AND p.completed_at IS NULL);

CREATE INDEX idx_encounters_workflow_stage ON encounters(workflow_stage, status);
