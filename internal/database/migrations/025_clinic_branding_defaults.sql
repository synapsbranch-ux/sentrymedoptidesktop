-- Adopt the commissioned clinic identity on existing installations that still
-- carry a product/development placeholder. Never overwrite a doctor's custom
-- clinic name; future edits remain regular versioned settings updates.
UPDATE settings
SET value_json = json_set(value_json, '$.name', 'Clinique Le Bon Spécialiste')
WHERE key = 'clinic'
  AND trim(COALESCE(json_extract(value_json, '$.name'), '')) IN (
    '', 'SentryMed', 'SentryMed Opti', 'Development Eye Clinic'
  );
