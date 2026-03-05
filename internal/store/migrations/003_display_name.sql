-- Add display_name for user-defined friendly names (works for both agent and passive hosts)
ALTER TABLE hosts ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
