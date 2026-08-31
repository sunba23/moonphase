CREATE TABLE sessions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
    holdsetup  SMALLINT NOT NULL REFERENCES board_editions (holdsetup),
    angle      SMALLINT NOT NULL,
    max_grade  TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'active',
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at   TIMESTAMPTZ
);

CREATE UNIQUE INDEX sessions_one_active_per_user ON sessions (user_id) WHERE status = 'active';
CREATE INDEX sessions_user_started ON sessions (user_id, started_at DESC);
