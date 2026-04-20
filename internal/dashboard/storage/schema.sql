-- Table des pushes reçus, stockés au format brut (raw protobuf).
-- Le raw_payload permet de reconstituer un push a posteriori (debug, audit).
CREATE TABLE IF NOT EXISTS pushes (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    machine_id   TEXT    NOT NULL,
    timestamp_ns INTEGER NOT NULL,
    nonce        TEXT    NOT NULL UNIQUE,
    raw_payload  BLOB    NOT NULL,
    received_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_pushes_machine_time
    ON pushes(machine_id, timestamp_ns DESC);

-- Table des métriques extraites de chaque push, indexées pour les requêtes graphe.
-- labels est un JSON string (ex: {"cpu":"0","mode":"user"}).
CREATE TABLE IF NOT EXISTS metrics (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    push_id      INTEGER NOT NULL REFERENCES pushes(id) ON DELETE CASCADE,
    machine_id   TEXT    NOT NULL,
    module       TEXT    NOT NULL,
    name         TEXT    NOT NULL,
    value        REAL    NOT NULL,
    labels       TEXT,
    timestamp_ns INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_metrics_query
    ON metrics(machine_id, name, timestamp_ns DESC);

-- État courant d'un module pour un agent (containers actuels, certs scannés, etc.).
-- state_json contient la structure complète sérialisée — schéma libre par module.
-- Une seule ligne par (machine_id, module) : écrasée à chaque push.
CREATE TABLE IF NOT EXISTS module_state (
    machine_id TEXT    NOT NULL,
    module     TEXT    NOT NULL,
    state_json TEXT    NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (machine_id, module)
);

-- Agents observés (premier push et dernier push).
-- Upsert à chaque push : INSERT ... ON CONFLICT(machine_id) DO UPDATE ...
CREATE TABLE IF NOT EXISTS agents (
    machine_id    TEXT    PRIMARY KEY,
    first_seen_at INTEGER NOT NULL,
    last_push_at  INTEGER NOT NULL
);
