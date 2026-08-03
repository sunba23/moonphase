CREATE TABLE board_editions (
    holdsetup  SMALLINT PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO board_editions (holdsetup, name) VALUES
    (1, '2016'),
    (15, 'Masters 2017'),
    (17, 'Masters 2019'),
    (21, '2024');
