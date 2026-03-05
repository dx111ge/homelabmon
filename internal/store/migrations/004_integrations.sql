-- Encrypted secrets for integration credentials
CREATE TABLE IF NOT EXISTS secrets (
    key   TEXT PRIMARY KEY,
    value BLOB NOT NULL
);

-- Integration configurations
CREATE TABLE IF NOT EXISTS integrations (
    id      TEXT PRIMARY KEY,
    type    TEXT NOT NULL,           -- fritzbox, unifi, homeassistant, pihole, pfsense
    name    TEXT NOT NULL DEFAULT '',
    config  TEXT NOT NULL DEFAULT '{}', -- non-secret config (URL, etc.) as JSON
    enabled INTEGER NOT NULL DEFAULT 1,
    status  TEXT NOT NULL DEFAULT 'unknown', -- ok, error, unknown
    error   TEXT NOT NULL DEFAULT '',
    last_sync DATETIME
);
