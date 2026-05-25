-- V32: Add deployment configuration for interface lifecycle management
-- This enables auto-deploy, manual-deploy, and delayed-deploy strategies

-- Add deployment configuration columns
ALTER TABLE interfaces
ADD COLUMN IF NOT EXISTS deployment_mode VARCHAR(50) DEFAULT 'manual',
ADD COLUMN IF NOT EXISTS auto_start BOOLEAN DEFAULT false,
ADD COLUMN IF NOT EXISTS deployment_delay_seconds INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS last_deployed_at TIMESTAMP WITH TIME ZONE,
ADD COLUMN IF NOT EXISTS deployment_status VARCHAR(50) DEFAULT 'not_deployed';

-- Add comments for documentation
COMMENT ON COLUMN interfaces.deployment_mode IS 'Deployment strategy: auto, manual, delayed (default: manual)';
COMMENT ON COLUMN interfaces.auto_start IS 'Whether to automatically start interface on service startup (default: false)';
COMMENT ON COLUMN interfaces.deployment_delay_seconds IS 'Delay in seconds before auto-deploying interface (default: 0, only applies to delayed mode)';
COMMENT ON COLUMN interfaces.last_deployed_at IS 'Timestamp of last successful deployment';
COMMENT ON COLUMN interfaces.deployment_status IS 'Current deployment status: not_deployed, deploying, deployed, failed, stopped';

-- Create index for auto-start queries (performance optimization)
CREATE INDEX IF NOT EXISTS idx_interfaces_auto_start
ON interfaces(auto_start, deployment_mode, is_active)
WHERE auto_start = true AND is_active = true;

-- Create index for deployment status queries
CREATE INDEX IF NOT EXISTS idx_interfaces_deployment_status
ON interfaces(deployment_status, is_active)
WHERE is_active = true;

-- Update existing interfaces to have manual deployment mode
UPDATE interfaces
SET deployment_mode = 'manual',
    auto_start = false,
    deployment_status = CASE
        WHEN status = 'active' THEN 'deployed'
        WHEN status = 'inactive' THEN 'not_deployed'
        ELSE 'not_deployed'
    END
WHERE deployment_mode IS NULL;
