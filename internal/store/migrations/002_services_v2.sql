-- Add category, source, container fields to services
ALTER TABLE services ADD COLUMN category TEXT NOT NULL DEFAULT '';
ALTER TABLE services ADD COLUMN source TEXT NOT NULL DEFAULT 'fingerprint';
ALTER TABLE services ADD COLUMN container_id TEXT NOT NULL DEFAULT '';
ALTER TABLE services ADD COLUMN container_image TEXT NOT NULL DEFAULT '';
