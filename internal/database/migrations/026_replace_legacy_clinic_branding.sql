-- The first Clinique Le Bon Spécialiste release may be installed over a
-- workstation that already has the legacy clinic identity and logo stored
-- locally. Replace that historical default once, while retaining any change
-- made by the doctor after migration 025 was installed.
UPDATE settings
SET value_json = json_set(value_json, '$.name', 'Clinique Le Bon Spécialiste')
WHERE key = 'clinic'
  AND COALESCE(updated_at, '') <= COALESCE(
    (SELECT applied_at FROM schema_migrations WHERE version = 25),
    ''
  );

-- Removing only the metadata makes the bundled, compressed clinic logo the
-- active default again. The old local file is deliberately left on disk: it
-- is no longer exposed and can be cleaned up safely with other unused assets.
DELETE FROM branding_assets
WHERE key = 'clinic_logo'
  AND COALESCE(updated_at, '') <= COALESCE(
    (SELECT applied_at FROM schema_migrations WHERE version = 25),
    ''
  );
