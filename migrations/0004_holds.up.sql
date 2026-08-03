CREATE TABLE holds (
    holdsetup     SMALLINT NOT NULL REFERENCES board_editions (holdsetup),
    grid_ref      TEXT NOT NULL,
    primary_type  TEXT,
    modifiers     TEXT[] NOT NULL DEFAULT '{}',
    is_tagged     BOOLEAN NOT NULL DEFAULT false,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    tagged_at     TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (holdsetup, grid_ref)
);
