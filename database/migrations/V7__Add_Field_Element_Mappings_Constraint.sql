-- Migration: Add missing mapping_configuration_id column and constraints
-- File: database/migrations/V10__Add_Mapping_Configuration_Link.sql
-- Purpose: Fix field_element_mappings table to properly link to hl7_fhir_mappings

-- Migration Description:
-- The field_element_mappings table is missing the foreign key relationship
-- to link field mappings to their parent mapping configuration.
-- This migration adds the missing column and constraints.

-- Step 1: Add the missing mapping_configuration_id column
ALTER TABLE field_element_mappings 
ADD COLUMN IF NOT EXISTS mapping_configuration_id INTEGER;

-- Step 2: Add foreign key constraint to hl7_fhir_mappings
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'fk_field_mappings_config'
    ) THEN
        ALTER TABLE field_element_mappings 
        ADD CONSTRAINT fk_field_mappings_config 
        FOREIGN KEY (mapping_configuration_id) 
        REFERENCES hl7_fhir_mappings(id) 
        ON DELETE CASCADE;
        
        RAISE NOTICE 'Added foreign key constraint: fk_field_mappings_config';
    ELSE
        RAISE NOTICE 'Foreign key constraint already exists, skipping';
    END IF;
END $$;

-- Step 3: Add unique constraint to prevent duplicate mappings
-- This allows the backend to use ON CONFLICT clauses
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'field_element_mappings_unique_mapping'
    ) THEN
        ALTER TABLE field_element_mappings 
        ADD CONSTRAINT field_element_mappings_unique_mapping 
        UNIQUE (mapping_configuration_id, hl7_field);
        
        RAISE NOTICE 'Added unique constraint: field_element_mappings_unique_mapping';
    ELSE
        RAISE NOTICE 'Constraint already exists, skipping';
    END IF;
END $$;

-- Step 4: Add performance indexes
CREATE INDEX IF NOT EXISTS idx_field_mappings_config_id 
ON field_element_mappings(mapping_configuration_id);

CREATE INDEX IF NOT EXISTS idx_field_mappings_hl7_field 
ON field_element_mappings(hl7_field);

CREATE INDEX IF NOT EXISTS idx_field_mappings_fhir_resource 
ON field_element_mappings(fhir_resource_type);

CREATE INDEX IF NOT EXISTS idx_field_mappings_segment 
ON field_element_mappings(segment_name);

-- Step 5: Add column comments
COMMENT ON COLUMN field_element_mappings.mapping_configuration_id 
IS 'Foreign key linking to the parent mapping configuration in hl7_fhir_mappings';

COMMENT ON CONSTRAINT field_element_mappings_unique_mapping ON field_element_mappings 
IS 'Ensures unique mapping per HL7 field within a mapping configuration';

-- Step 6: Verify the migration
SELECT 
    'Migration completed successfully' as status,
    column_name,
    data_type,
    is_nullable
FROM information_schema.columns 
WHERE table_name = 'field_element_mappings' 
    AND column_name = 'mapping_configuration_id'

UNION ALL

SELECT 
    'Constraints added' as status,
    conname as constraint_name,
    contype::text as constraint_type,
    '' as is_nullable
FROM pg_constraint 
WHERE conrelid = 'field_element_mappings'::regclass
    AND conname IN ('field_element_mappings_unique_mapping', 'fk_field_mappings_config');