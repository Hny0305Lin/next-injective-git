# Repository Identity

`RepositoryCore` assigns a stable `bytes32 repoId`. Ownership and name form the
current locator, while every prior locator remains an immutable alias to that
same ID. An alias can never be claimed by another repository.

Reads of a historical locator return the canonical current locator. Writes to
an old locator fail with a moved-repository error so callers cannot create a
split history. Refs, collaborators, moderation, economics, badges, recovery,
and fork lineage key state by `repoId`, not mutable display location.

Ownership transfer is an explicit state machine with proposal, target action,
cancellation, and expiry. Guardian recovery is separate and Core accepts its
ownership capability only from the Directory-bound Recovery module. Pending
transfers and recoveries are not included in migration snapshots.

Fork creation records immutable source lineage and is bounded against abusive
depth or fan-out according to contract limits. Client display may resolve
usernames, but canonical authorization uses EVM addresses.
