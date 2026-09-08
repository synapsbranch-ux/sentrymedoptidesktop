-- D1: the two optional registration fields the clinic asked for. Both are
-- nullable, so no existing record changes meaning.
ALTER TABLE patients ADD COLUMN civil_status TEXT;
ALTER TABLE patients ADD COLUMN religion TEXT;
-- Free text captured only when religion is recorded as "other".
ALTER TABLE patients ADD COLUMN religion_other TEXT;

CREATE INDEX idx_patients_civil_status ON patients(civil_status);
