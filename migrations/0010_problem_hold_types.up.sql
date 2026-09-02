SET LOCAL statement_timeout = 0;

CREATE TABLE problem_hold_types (
    problem_id   BIGINT PRIMARY KEY REFERENCES problems (id) ON DELETE CASCADE,
    crimp        SMALLINT NOT NULL DEFAULT 0,
    sloper       SMALLINT NOT NULL DEFAULT 0,
    pinch        SMALLINT NOT NULL DEFAULT 0,
    jug          SMALLINT NOT NULL DEFAULT 0,
    pocket       SMALLINT NOT NULL DEFAULT 0,
    unknown      SMALLINT NOT NULL DEFAULT 0,
    total_scored SMALLINT NOT NULL DEFAULT 0,
    dominant     TEXT
);

INSERT INTO problem_hold_types
    (problem_id, crimp, sloper, pinch, jug, pocket, unknown, total_scored, dominant)
SELECT
    pm.problem_id,
    count(*) FILTER (WHERE h.primary_type = 'crimp'),
    count(*) FILTER (WHERE h.primary_type = 'sloper'),
    count(*) FILTER (WHERE h.primary_type = 'pinch'),
    count(*) FILTER (WHERE h.primary_type = 'jug'),
    count(*) FILTER (WHERE h.primary_type = 'pocket'),
    count(*) FILTER (WHERE h.primary_type IS NULL OR h.primary_type NOT IN ('crimp', 'sloper', 'pinch', 'jug', 'pocket')),
    count(*),
    (
        SELECT t.name
        FROM (VALUES
            ('crimp',  count(*) FILTER (WHERE h.primary_type = 'crimp')),
            ('jug',    count(*) FILTER (WHERE h.primary_type = 'jug')),
            ('pinch',  count(*) FILTER (WHERE h.primary_type = 'pinch')),
            ('pocket', count(*) FILTER (WHERE h.primary_type = 'pocket')),
            ('sloper', count(*) FILTER (WHERE h.primary_type = 'sloper'))
        ) AS t(name, n)
        WHERE t.n > 0
        ORDER BY t.n DESC, t.name ASC
        LIMIT 1
    )
FROM problem_moves pm
JOIN holds h ON h.holdsetup = pm.holdsetup AND h.grid_ref = pm.grid_ref
WHERE pm.move_type <> 'f'
GROUP BY pm.problem_id;
