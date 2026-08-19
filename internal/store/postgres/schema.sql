-- Loupe run store. Two tables: one row per run, one row per committed step.
-- Everything is CREATE ... IF NOT EXISTS so applying it on connect is safe to
-- repeat.

CREATE TABLE IF NOT EXISTS runs (
    id          TEXT PRIMARY KEY,
    task        TEXT NOT NULL,
    status      TEXT NOT NULL,
    error       TEXT NOT NULL DEFAULT '',
    worker      TEXT NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS steps (
    run_id     TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    id         INTEGER NOT NULL,           -- per-run, 1-based, in order
    kind       TEXT NOT NULL,
    thought    TEXT NOT NULL DEFAULT '',
    tool       TEXT NOT NULL DEFAULT '',
    input      TEXT NOT NULL DEFAULT '',
    output     TEXT NOT NULL DEFAULT '',
    is_error   BOOLEAN NOT NULL DEFAULT false,
    answer     TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ,
    ended_at   TIMESTAMPTZ,
    PRIMARY KEY (run_id, id)
);

-- Narrow the claim scan to runs that are actually claimable.
CREATE INDEX IF NOT EXISTS runs_claimable
    ON runs (created_at)
    WHERE status IN ('pending', 'running');
