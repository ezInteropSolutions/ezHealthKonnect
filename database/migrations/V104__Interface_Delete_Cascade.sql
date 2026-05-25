-- V80: Add ON DELETE CASCADE to all foreign keys referencing interfaces.id
-- This allows hard-deleting an interface without manually deleting dependent rows first.
-- Soft deletes (is_active=false) are unaffected — CASCADE only fires on DELETE.

DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN
        SELECT tc.constraint_name, tc.table_name
        FROM information_schema.table_constraints tc
        JOIN information_schema.referential_constraints rc
            ON tc.constraint_name = rc.constraint_name
        JOIN information_schema.table_constraints tc2
            ON rc.unique_constraint_name = tc2.constraint_name
        WHERE tc2.table_name = 'interfaces'
          AND tc.constraint_type = 'FOREIGN KEY'
    LOOP
        -- Drop old constraint and re-add with ON DELETE CASCADE
        EXECUTE format(
            'ALTER TABLE %I DROP CONSTRAINT IF EXISTS %I',
            r.table_name, r.constraint_name
        );
        EXECUTE format(
            'ALTER TABLE %I ADD CONSTRAINT %I
             FOREIGN KEY (interface_id) REFERENCES interfaces(id) ON DELETE CASCADE',
            r.table_name, r.constraint_name
        );
    END LOOP;
END $$;
