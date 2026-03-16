-- V54__Drop_Messages_Parent_Table.sql
-- Removes the messages partitioned parent table created in V53.
--
-- Architectural decision: ezHealthKonnect uses a table-per-interface design.
-- Each interface owns:
--   • PostgreSQL: messages_intf_{uuid}   — message metadata, status, audit columns
--   • MongoDB:    raw_messages_intf_{uuid} — raw + parsed message content
--
-- Reasons table-per-interface is the right model here:
--   1. Natural tenant isolation — no cross-interface data leakage without explicit query
--   2. Independent lifecycle — TRUNCATE, DROP, or archive one interface without touching others
--   3. Dynamic query generation — callers build the table name at runtime, zero routing overhead
--   4. MongoDB symmetry — the same per-interface collection pattern already used for raw content
--   5. Simple security — grant SELECT on messages_intf_X to interface-X service account
--
-- V53's partitioned parent (messages) added complexity without benefit for this model.
-- No data is in the parent (all 26 existing tables were left as standalone in V53).

DROP TABLE IF EXISTS messages;
