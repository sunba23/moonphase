CREATE TABLE problem_configurations (
    id                     BIGSERIAL PRIMARY KEY,
    problem_id             BIGINT NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    holdsetup              SMALLINT NOT NULL REFERENCES board_editions (holdsetup),
    api_id                 INTEGER NOT NULL,
    angle                  SMALLINT NOT NULL,
    primary_angle          SMALLINT,
    grade                  TEXT NOT NULL,
    user_grade             TEXT,
    user_rating            INTEGER,
    is_benchmark           BOOLEAN NOT NULL DEFAULT false,
    is_competition_problem BOOLEAN NOT NULL DEFAULT false,
    is_primary             BOOLEAN NOT NULL DEFAULT false,
    repeats                INTEGER NOT NULL DEFAULT 0,
    comment                TEXT,
    date_updated           TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (problem_id, api_id)
);

CREATE INDEX idx_problem_configurations_holdsetup_angle_grade
    ON problem_configurations (holdsetup, angle, grade);
