CREATE TABLE profiles (
    id         UUID PRIMARY KEY REFERENCES auth.users (id) ON DELETE CASCADE,
    max_grade  TEXT NOT NULL,
    holdsetup  SMALLINT NOT NULL REFERENCES board_editions (holdsetup),
    angle      SMALLINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
