# Deprecated — moved to `src/features/`

Pages have been reorganized into feature slices under `src/features/`:

- `Home.tsx` → `features/home/Home.tsx`
- `Explorer.tsx` → `features/explorer/Explorer.tsx`
- `Owner.tsx` → `features/owner/Owner.tsx`
- `Settings.tsx` → `features/settings/Settings.tsx`
- `Monitor.tsx` + `Monitor/*` → `features/monitor/`
- `Repo/*` → `features/repo/`
- `Archive.tsx` → `features/archive/Archive.tsx`
- `IpfsExplorer.tsx` → `features/ipfs/IpfsExplorer.tsx`

Please update imports. This directory contains shims for one release cycle.
