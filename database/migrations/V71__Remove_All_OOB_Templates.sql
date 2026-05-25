-- V71: Remove all OOB (system) interface templates
-- Templates will be rebuilt later when requirements are clearer

DELETE FROM interface_templates WHERE is_system = TRUE;
