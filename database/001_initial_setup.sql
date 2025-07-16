-- Initial setup for ezHealthKonnect
-- This file runs automatically on first Docker startup

-- Enable necessary extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Create a simple health check table
CREATE TABLE IF NOT EXISTS system_health (
    id SERIAL PRIMARY KEY,
    component VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL,
    checked_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Insert initial health check
INSERT INTO system_health (component, status) 
VALUES ('database', 'healthy') 
ON CONFLICT DO NOTHING;

-- Log that setup completed
SELECT 'Database initialization completed' AS setup_status;
