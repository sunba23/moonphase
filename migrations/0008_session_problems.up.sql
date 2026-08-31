CREATE TABLE session_problems (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id               UUID NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    seq                      INTEGER NOT NULL,
    problem_id               BIGINT NOT NULL REFERENCES problems (id),
    problem_configuration_id BIGINT NOT NULL REFERENCES problem_configurations (id),
    recommended_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (session_id, seq)
);

CREATE INDEX idx_session_problems_session_seq ON session_problems (session_id, seq);
