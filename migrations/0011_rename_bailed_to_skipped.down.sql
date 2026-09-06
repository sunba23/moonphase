-- Irreversible: the original RPE values of the rewritten 'bailed' rows were
-- dropped by the up migration and cannot be restored. This no-op exists only so
-- golang-migrate has a down step to run.
SELECT 1;
