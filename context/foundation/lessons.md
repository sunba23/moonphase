# Lessons Learned

> Append-only register of recurring rules and patterns. Re-read at start by /10x-frame, /10x-research, /10x-plan, /10x-plan-review, /10x-implement, /10x-impl-review.

## Manual verification steps must be concrete and copy-pasteable

- **Context**: Manual Verification steps written into `context/changes/<id>/plan.md` phases (by `/10x-plan` and `/10x-implement`).
- **Problem**: Abstract manual-verification steps (e.g. "a small hand-crafted sample with 2-3 problems, one active, one inactive, one deleted") force the user to design the fixture and derive exact commands themselves before they can even start verifying.
- **Rule**: Manual Verification steps must be concrete and copy-pasteable: include the exact fixture content (e.g. inline JSON), the exact shell command to run, and the expected output/result — never a prose description of what the user should construct.
- **Applies to**: plan, implement

## CLI binaries should auto-load .env, not rely on the operator sourcing it

- **Context**: cmd/* CLI binaries -- any one-shot Go CLI under cmd/ that calls config.Load() (cmd/catalog, cmd/migrate, future CLIs).
- **Problem**: config.Load() reads only os.Getenv, so every manual verification run requires the operator to remember `set -a; source .env; set +a` first, or it fails with "SUPABASE_URL is required" -- happened during Phase 4 manual testing of catalog holds status.
- **Rule**: Each cmd/* binary should load the .env file from the project root automatically (e.g. via godotenv) before calling config.Load(), so operators never need to manually source it.
- **Applies to**: plan, implement
