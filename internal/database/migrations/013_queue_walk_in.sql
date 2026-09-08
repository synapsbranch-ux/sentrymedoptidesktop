-- B2: a walk-in is registered and queued in one step, and the waiting room shows
-- why each person is here. The queue had no reason of its own and could only
-- borrow one from a linked appointment, which a walk-in does not have.
ALTER TABLE queue_entries ADD COLUMN visit_reason TEXT;

UPDATE queue_entries
SET visit_reason = (SELECT a.reason FROM appointments a WHERE a.id = queue_entries.appointment_id)
WHERE appointment_id IS NOT NULL AND visit_reason IS NULL;
