ALTER TABLE session_problems
    ADD COLUMN rpe        SMALLINT,
    ADD COLUMN completion TEXT,
    ADD COLUMN climbed_at  TIMESTAMPTZ;
