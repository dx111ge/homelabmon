-- Add site label for peer federation (multi-site mesh)
ALTER TABLE hosts ADD COLUMN site TEXT NOT NULL DEFAULT '';
ALTER TABLE peers ADD COLUMN site TEXT NOT NULL DEFAULT '';
