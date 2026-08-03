CREATE TABLE problems (
    id            BIGSERIAL PRIMARY KEY,
    external_id   INTEGER NOT NULL,
    holdsetup     SMALLINT NOT NULL REFERENCES board_editions (holdsetup),
    name          TEXT NOT NULL,
    setter        TEXT,
    setby_id      TEXT,
    climb_method  TEXT,
    holdsets      TEXT,
    coordinates   TEXT,
    beta_videos   INTEGER NOT NULL DEFAULT 0,
    moves_raw     TEXT NOT NULL,
    date_inserted TIMESTAMPTZ,
    date_updated  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (holdsetup, external_id)
);

CREATE INDEX idx_problems_holdsetup ON problems (holdsetup);
