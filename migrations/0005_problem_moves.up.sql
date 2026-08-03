CREATE TABLE problem_moves (
    id         BIGSERIAL PRIMARY KEY,
    problem_id BIGINT NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    holdsetup  SMALLINT NOT NULL,
    seq        INTEGER NOT NULL,
    move_type  TEXT NOT NULL,
    grid_ref   TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (problem_id, seq),
    FOREIGN KEY (holdsetup, grid_ref) REFERENCES holds (holdsetup, grid_ref)
);

CREATE INDEX idx_problem_moves_holdsetup_grid_ref ON problem_moves (holdsetup, grid_ref);
CREATE INDEX idx_problem_moves_problem_id ON problem_moves (problem_id);
