-- 'bailed' is renamed to 'skipped' and no longer carries an RPE. Rewrite any
-- existing rows to the new shape.
UPDATE session_problems
SET completion = 'skipped',
    rpe        = NULL,
    climbed_at = COALESCE(climbed_at, now())
WHERE completion = 'bailed';
