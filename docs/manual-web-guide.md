# 03 · Web Application Guide

Status: manual chapter. Audience: users. Last updated: 2026-10-05.

The igit web application at <https://www.igit.xyz> is a **direct-to-chain
client**. It has no backend of its own: it reads the same Injective EVM
contracts and the same storage your CLI uses, straight from your browser. That
has three consequences worth internalizing before you click anything.

1. **Anything you can see, you can verify.** The wallet address, the ref, the
   pack digest, and the transaction hash are all inspectable on chain.
2. **The SuiteDirectory is the trust root.** If the configured Directory does
   not verify, the application says so and refuses to show repository data. It
   does not fall back.
3. **Public means public.** Repository data is served over anonymous HTTPS.
   "Mine" filters a view; it never hides anything from anyone else.

This chapter covers every screen. The version-specific walkthroughs are in
[Chapter 04 (Suite v4)](manual-suite-v4.md) and
[Chapter 05 (Suite v2 + v3)](manual-suite-v2-v3.md).

## The suite query parameter

Before the screens, one piece of URL syntax explains most of what you will see.

A repository name can exist in more than one generation at once. The web
application resolves a name by walking the configured SuiteDirectory list, so
the same URL can legitimately lead to two different repositories. The `?suite=`
parameter is how you say which one you mean:

```text
https://www.igit.xyz/<owner>/<repo>?suite=4     # the Suite v4 (BYOS) copy
https://www.igit.xyz/<owner>/<repo>?suite=3     # the Suite v3 (IPFS) copy
https://www.igit.xyz/<owner>/<repo>             # first configured SuiteDirectory
```

Mechanically, the parameter becomes a version preference that the resolver uses
while walking the configured directories: it keeps looking until it finds a
directory that resolves *this* name to a repository whose on-chain
`suite_version` matches the requested one. Every subsequent read and every write
on the page then runs against that repository's own `SuiteDirectory`, not
against whichever one happens to be first.

Accepted values are `3` and `4`. Any other value is ignored and the page falls
back to the default resolution order. The parameter is generated for you by the
owner page's repository links, so in normal browsing you rarely type it.

Concretely, for the worked example in this manual:

| Generation | URL |
|---|---|
| Suite v4 (latest EVM, BYOS) | `https://www.igit.xyz/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase?suite=4` |
| Suite v3 (earlier EVM, IPFS) | `https://www.igit.xyz/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase?suite=3` |
| CosmWasm v1 (archive) | `https://www.igit.xyz/archive/cosmwasm-v1/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase` |

Note that the archive is **not** a `?suite=` value. It is a separate route
prefix, because it is a separate trust root — see
[Chapter 06](manual-cosmwasm-v1-archive.md).

## Route map

| Route | Renders | Needs a verified Suite? |
|---|---|---|
| `/` | Dashboard — your repositories plus recent contract activity | yes |
| `/search?q=…` | Search results across repositories and owners | yes |
| `/settings` | Network profile, SuiteDirectory, testnet wallet table | no |
| `/monitor` | Public monitor — network pulse, gateways, V1 boundary | no |
| `/mapmonitor` | Map monitor — reachable infrastructure on a world tour | no |
| `/explorer` | Block explorer — on-chain activity and transaction detail | yes |
| `/ipfs` | IPFS explorer — inspect CIDs, audit a repository's packfiles | yes |
| `/archive/cosmwasm-v1` | CosmWasm v1 archive entry — open an archived repository | no |
| `/archive/cosmwasm-v1/:owner` | Archived owner's repository list | no |
| `/archive/cosmwasm-v1/:owner/:repo/*` | Archived repository viewer (read-only) | no |
| `/:owner` | Owner page — repositories and badges | yes |
| `/:owner/:repo/*` | Repository page — code, commits, refs, sponsors | yes |

Routes that do **not** require a verified Suite — the archive, the monitor, and
the map monitor — deliberately stay usable so you can always inspect
infrastructure and history even while a Directory is misconfigured.

On every other route, if the Suite is not verified the application shows a
banner instead of repository data:

| State | Title | Explanation |
|---|---|---|
| Checking | Verifying EVM Suite | "Checking the Directory, modules, bindings, and code hashes." |
| Unconfigured | EVM Suite is not configured | "Wallet connection and INJ balance remain available, but repository data needs a verified SuiteDirectory." |
| Failed | EVM Suite verification failed | "The saved Directory is not a valid active Suite for this network. Review the address in Settings." |

Except while checking, the banner carries a **Configure** link to Settings.
Suite verification is cached for 30 seconds; a small toast reports
"Refreshing cache" and then "Cache updated".

## Connecting a wallet

Click **Connect wallet** in the top right. The modal is titled "Connect a
wallet" and lists every EVM wallet it can find through EIP-6963 discovery, with
a legacy-injection fallback for wallets that do not announce themselves.

Supported wallets include MetaMask, Rabby, OKX Wallet, Bitget Wallet, Trust
Wallet, Coinbase Wallet, Gate Wallet, Brave Wallet, Keplr (EVM), and Compass
(Leap EVM). A row shows **Connect** when the wallet is present and **Install**
when it is not.

If nothing is detected:

> No supported EVM wallet was detected in this browser. Open iGit in Chrome or
> Brave where your wallet extension is installed and enabled.

### WalletConnect

**Scan with WalletConnect** opens a QR panel for mobile wallets. WalletConnect
is pinned to Injective EVM testnet — chain `1439` — and the panel says so:

> Use a WalletConnect wallet that supports Injective EVM testnet (chain 1439).

One caveat is stated in the UI and worth repeating: Keplr Mobile does not
currently list chain 1439 for EVM WalletConnect, so that particular QR will not
pair in Keplr. WalletConnect also requires the deployment to be configured with
a project ID; if it is not, the button explains that it is unavailable.

### Connected and writable are different things

A connected wallet is not automatically able to write. The application tracks a
separate **writable** state, which is true only when the wallet's active chain
equals the configured EVM chain ID.

- Connected but not writable: reads work, every write control is disabled, and
  the UI explains why — "Switch to Injective EVM testnet to write."
- The account menu shows the same hint next to your address.
- A write attempted through the wrong chain fails with "Switch the wallet to
  Injective EVM testnet before writing."

This is why a repository page can show you everything while its Edit and
Sponsor controls sit inert.

## Dashboard

`/` is the landing page.

- Signed out, the heading is **Dashboard** and the copy reads "Browse
  repositories, inspect on-chain activity, and resolve IPFS objects." The
  repository panel invites you to "Connect your workspace" or to explore public
  activity instead.
- Signed in, the heading becomes **Your repositories** and lists the
  repositories owned by the connected address, with a client-side filter
  ("Find a repository…").

An overview strip summarizes the workspace: the **Repositories** count, the
**Network** state (which reads "Injective" when a Suite is configured and
"Setup needed" when it is not), and the **Objects** rail, which reads "IPFS" —
the legacy pack rail, shown for continuity with v3 repositories.

The right-hand column shows **Recent activity** — the latest confirmed contract
actions, lazily loaded as you scroll. Each row carries an action badge, the
sender (abbreviated), and a relative timestamp, with a `failed` marker for
transactions that reverted. Before the Suite verifies, the panel says:

> Activity is unavailable until the EVM Suite passes verification.

Both the empty state and the activity panel link onward to the explorer.

### Empty states you may meet

| Situation | What the page says |
|---|---|
| No wallet connected | "Connect your workspace" — "Connect a wallet to see your repositories, or inspect public activity on the chain." |
| Wallet connected, no Suite | "Repository data is not configured" — "Connect remains available, but repository reads require a verified EVM Suite." |
| Wallet connected, Suite verified, no repositories | "No repositories yet" — "Create the first repository from your terminal." with `igit init my-repo "hello chain"` |

That last empty state is the fastest path from the browser to a real
repository: copy the command, run it, reload.

## Repository page

`/:owner/:repo/*` is the page you will spend the most time on. It renders the
repository at a particular ref, and the ref is part of the URL.

### Header

- The owner, linked to the owner page. Long `inj1…` addresses are abbreviated
  for display; the full address is always available on the owner page.
- The repository name in bold.
- A **contract type badge**. Which one appears tells you the generation at a
  glance:
  - **EVM V4** with a **BYOS** tag — packfiles resolve from BYOS storage
    buckets through a verified manifest.
  - **EVM V2 + V3** — the Injective EVM V2/V3 Suite contract.
  - **CosmWasm V1** — the read-only archive contract.
- A **suite version badge** — "Suite v3 · IPFS" or "Suite v4 · BYOS" — read
  from `SuiteDirectory.suiteVersion()`. Hovering explains it: "SuiteDirectory
  protocol version: v4. Fully supported." A version the application has not
  been taught about renders as "Suite vN (untested)" with a warning title,
  because it will be read through a best-effort fallback.
- A **moderation badge** (`delisted` or `frozen`) when the repository is not
  `active`. Active repositories show no badge.
- The description, when set.
- Fork lineage as "forked from owner/repo", linked to the source.

For the worked example, the Suite v4 URL shows the **EVM V4 / BYOS** badge and
"Suite v4 · BYOS"; the `?suite=3` URL shows **EVM V2 + V3** and "Suite v3 · IPFS".

### Stats row

Four figures across the head:

| Figure | Meaning |
|---|---|
| `<sha>` HEAD | First 8 characters of the ref's commit SHA; `—` when the ref has no commit |
| `<n>` branches | Number of `refs/heads/*` entries |
| `<n>` tags | Number of `refs/tags/*` entries |
| `<n>` packfiles | Number of pack objects this ref points at |

On Suite v3 the packfile count is the length of the on-chain `packUris` array.
On Suite v4 the ref carries a manifest commitment, and pack locations come from
the verified manifest instead.

### Clone box

A protocol selector switches between two exact strings:

```text
igit://      igit clone igit://inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
https://     https://www.igit.xyz/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/demo-showcase
```

The `https://` form is a web URL for sharing and browsing; the `igit://` form is
the Git remote. The copy button places the selected command on your clipboard
and confirms with a toast.

### Tabs

| Tab | Route suffix | Shows |
|---|---|---|
| **Code** | (base) | File tree at the selected ref, or the blob view for a file |
| **Commits** | `/commits/<ref>` | Commit history for the ref |
| **Refs** | `/refs` | Branches and tags, with their commits, pack counts, and updated times |
| **Sponsors** | `/sponsors` | Lifetime sponsorship, revenue splits, collaborators, badges |

The **Sponsors** tab appears only for EVM repositories. Archived CosmWasm v1
repositories show, in its place:

> Sponsorship is unavailable for archived CosmWasm V1 repositories.

#### Code

The tree view lists files and folders with their last commit and updated time,
folders first. A `..` row walks to the parent directory. If the directory
contains a `README.md`, it is rendered inline beneath the listing. A branch/tag
selector at the top switches refs; tags are marked with a `⌂` prefix.

An empty repository on the Code tab says plainly:

> empty repository — push something first.

Opening a file gives the blob view. Markdown files render as formatted
documents; everything else renders as plain text. Binary content is detected
and reported rather than printed: "binary file (N bytes) — not rendered".

#### Commits

The history list shows message, author, and date. The author column names the
on-chain ref updater where available. While the application reconstructs history
from pack data it shows "reconstructing history…"; an empty ref says "no commits
yet."

Clicking a commit opens the commit view: the abbreviated object ID, author,
date, and parent, followed by a per-file diff. Binary or oversized files are
labelled rather than diffed, and an empty commit says "no changes (empty
commit)."

#### Refs

Two sections, **Branches (n)** and **Tags (n)**, each with columns for commit,
pack count, and update time. Both branch and tag names link to the tree at that
ref.

#### Sponsors

This tab aggregates the economic surface of the repository:

- **Lifetime sponsorship** — the total received, with a sponsor wall. When
  nobody has sponsored yet it suggests the exact command:
  `igit sponsor <owner> <repo> 0.1`.
- **Revenue split** — the current distribution, with the owner's remainder
  shown explicitly, plus an editor for owners. Each recipient gets a percentage
  between 0.01% and 100%, at most 20 recipients, and the total may not exceed
  100%.
- **Collaborators** — the accounts and roles recorded on chain.
- **Badges awarded** — the trophy wall for the repository, and for the current
  owner, a form to award a new badge with a reason.

Writes on this tab require a writable wallet and, for the split editor and badge
award, ownership of the repository. Sponsoring is open to anyone with a wallet
on the right chain.

### Editing repository metadata

Owners see a pencil control beside the clone box. It opens a **Repository
settings** dialog with two fields:

- **Description** — free text, up to 1024 characters.
- **Default branch** — up to 64 characters.

Saving broadcasts a metadata update and confirms with "Repository updated". The
editor closes on Escape or by clicking the backdrop, and refuses to close while
a save is in flight.

### Ownership transfer

Owners see an **Ownership transfer** panel. Ownership never moves in a single
step:

1. The current owner enters a target address (`inj1…` or `0x…`) and clicks
   **Start transfer**.
2. The panel then shows the proposed owner and two deadlines, read from chain
   state: when the transfer becomes acceptable, and when it expires. The exact
   copy is "Accept after <timestamp>; expires <timestamp>."
3. Depending on your role, the panel offers **Cancel** (current owner),
   **Reject** and **Accept** (proposed new owner), or **Clear expired transfer**
   (after the deadline has passed).

The maturation and expiry windows are on-chain values. The interface renders the
timestamps it reads; it does not apply a rule of its own. The CLI prints the
same figures — see
[Chapter 02 · ownership transfer](manual-cli-reference.md#ownership-transfer--the-7-day-rule).

### When a repository is not found

Two distinct situations produce two distinct messages.

- **The locator does not exist on this Suite.** The page reports the failure
  and offers a discovery link:
  **Open this locator in the CosmWasm V1 Archive**. This is a convenience for
  the very common case of an address that predates the EVM generations, and it
  never happens automatically — you choose to follow it.
- **The repository was renamed or transferred.** The page shows
  "This repository has moved." with a link to the current location.

## Owner page

`/:owner` accepts an `inj1…` address or a registered username. It shows the
owner's repositories and badges.

- Repositories link to `/<owner>/<name>?suite=<version>`, which is where the
  `?suite=` parameter comes from in normal browsing: a v4 repository links to
  `?suite=4`, a v3 repository to `?suite=3`.
- Badges list the repository each badge came from, when it was awarded, and the
  recorded reason.
- A filter narrows the repository list client-side.

Empty states: "no repositories on chain.", "no matching repositories.", and
"no badges awarded yet."

## Search

Two ways in: the header search box, and the `/search?q=…` route.

- **Keyboard.** Press `/` anywhere outside a text field to focus the search box.
- **Instant results.** Typing two or more characters searches a locally built
  repository index and shows up to seven matches, ranked exact match first,
  then prefix, then substring, then owner-address match. Delisted repositories
  are excluded. While the index is still building, the dropdown says "Building
  repository index from chain…".
- **Recent queries.** The last five searches are kept locally and offered under
  "Recent", with a **Clear** control.
- **Full results.** Pressing Enter opens `/search?q=…`, which shows matching
  repositories grouped with their owner, each row marked as coming from a chain
  event or an owner listing.

The search index is a **navigation aid, not a source of truth**: selecting a
result re-resolves the repository from the contract before opening it. The index
grows as you browse owners, connect a wallet, or when the public explorer source
is reachable. If nothing is found:

> No repositories found for "<query>".
> The index grows as you browse owners, connect a wallet, or when the explorer
> source is reachable.

### Archive results in search

A search for an archived owner or repository also offers **View other V1 Archive
results**, which queries the CosmWasm v1 contract directly and links into the
archive routes. If the query does not resolve to an archived owner, it says so
rather than failing silently.

## Block explorer

`/explorer` lists on-chain activity for the configured SuiteDirectory. The
header states exactly what is being watched:

```text
Activity across SuiteDirectory 0x9873964750… · EVM chain 1439
```

A configuration card reports the platform fee, the suite version, the treasury
address, and the admin address. A lookup field accepts a transaction hash and
renders the full transaction: hash, success or revert code, block height, gas
used, signature mode, public key when present, the decoded messages, and the
raw log on failure.

Two scopes control the list:

- **All** — every contract transaction observed in the window.
- **Mine** — filtered to your connected address.

The privacy note is explicit and worth quoting in full:

> On-chain data is public. "Mine" only filters this view to your address; it
> does not hide anything from others.

Empty states: "no activity from your address yet." and "no contract
transactions found." A hash that has not been indexed yet reports "tx not found
(or not indexed yet)."

## Public monitor

`/monitor` is read-only and needs no wallet. It is the operational pulse of the
protocol.

- **Network pulse** with a live connection indicator and a manual **Refresh**.
- **Source cards** for each surface: the EVM V2/V3 Suite (verified Directory
  trust root, latest block, Blockscout link), **EVM V4** (successor suite, BYOS
  storage buckets — pack storage is stated as "S3 / Cloudflare R2"), and the
  CosmWasm v1 archive as an independent historical read-only surface.
- **Gateway cards** for the Hong Kong and US IPFS gateways.
- **Storage provider cards** for the legacy replica and the current archive
  path, with an honest disclaimer:

  > Provider roles are inventory metadata, not live S3 health checks.
  > Provider badges describe the configured role; private capacity and
  > operational telemetry stay server-side.

- **Three tab views.** *Activity* shows the activity pulse, source notes
  ("Counts are bounded observations, never all-time totals."), recent actions,
  and gateway probes measured by median latency over three direct browser
  checks. *Storage* shows pack and repository counts per surface. *V1 boundary*
  states the archive's status and limits:

  > Legacy CosmWasm V1 is read-only.
  > The archive remains available at a session snapshot. Migration to EVM V2 is
  > required and is not available from this page.

  with links to open the archive viewer and to view the contract on an explorer.

Metric cards report recent activity within a bounded block window, the latest
observed block, the observation window size, and the last refresh time. The
monitor refreshes every 90 seconds.

If no Suite is configured, the monitor still loads and tells you precisely what
is missing — "Add a verified EVM V2/V3 SuiteDirectory in Settings" or
"Add a verified EVM V4 SuiteDirectory in Settings" — instead of showing zeros.

## Map monitor

`/mapmonitor` is a visual tour of the infrastructure the client can reach:
IPFS gateways, S3 storage endpoints, and BYOS bucket edges, plotted on a world
map with a rotating focus every 45 seconds.

- Header: "Reachable infrastructure" — "Storage buckets, IPFS nodes, and
  gateways the igit client can reach, on a rotating world tour."
- Hovering pauses the rotation; clicking a stop pins it.
- A deep link `?stop=<id>` focuses a specific endpoint.
- The storage board lists each endpoint with its observed pack and repository
  counts, plus the archive row, and labels the freshness ("Live" or "Cached").
- Popups list the repositories observed at an endpoint, with a link onward to
  the owner page.

The map falls back to a development tile source when no map key is configured,
and it says so rather than rendering a blank canvas.

## IPFS explorer

`/ipfs` is the data-plane inspector for the legacy IPFS generation.

- It states the active gateway: "Packfiles are stored on IPFS and referenced
  on-chain. Gateway: `<configured gateway>`".
- **Inspect a CID** accepts `ipfs://bafy…` or a bare CID and reports
  reachability, size, whether it is a Git packfile, its pack version, and its
  object count, with a link to open it on the gateway.
- **Audit a repo's packfiles** takes `owner/repo` and checks every CID the
  repository references, producing a pinning health view: "N/M CIDs reachable
  via this gateway", with per-CID status.

When a CID is not served, the message does not pretend otherwise:

> no reachable node is serving this CID — it may need pinning.

Your owned repositories appear as quick-fill chips at the top.

This surface is specific to the IPFS generations. A Suite v4 repository stores
packs in your own bucket rather than on IPFS, so its availability is a question
about your bucket, not about a gateway.

## Settings

`/settings` configures the network profile and documents the local test
accounts.

### Network profile

- **Profile** — the selectable network profile. Endpoints, chain IDs, and
  contract addresses all come from the selected profile.
- **EVM chain** and **RPC** are displayed for the selected profile, so you can
  read back exactly what the client will use.

For Injective testnet the values are EVM chain `1439` and RPC
`https://k8s.testnet.json-rpc.injective.network/`.

### SuiteDirectory address

This field holds the single trust root, and it behaves differently depending on
where the application is running.

**On a public deployment** — anything that is not localhost — the field is
**read-only**. It is pinned by the released profile, and the page explains:

> This public deployment pins the Directory from its released profile, so the
> address is copy-only here. Run igit-web locally to verify and save your own
> override.

**Running locally** the field is editable, and accepts one or more
comma-separated addresses:

> Enter one or more comma-separated SuiteDirectory addresses. The first address
> is the primary read target; repos from all listed suites are merged in
> listings. A local override is saved only after every listed Suite verifies.

The multi-address semantics matter, so here they are stated plainly:

- The **first** address is the primary read target used for direct resolution.
- Repositories from **all** listed suites are **merged** in listings.
- Because the same name can exist in two suites, listings keep both as distinct
  entries — one per suite version — which is precisely why `?suite=` exists.
- The override is stored in your browser only, and **Verify and save** refuses
  to persist a Directory that does not verify.

The released testnet profile lists two addresses, in this order:

```text
0xf987396475d0a4c96b722e993a95d8720a6292ad   # Suite v4 (BYOS successor)
0xf8844F90887731FFd607E1f59e39a3918F6eAb35   # Suite v3 (IPFS)
```

**Restore defaults** re-applies the released profile.

### Testnet wallets

The page also lists the known development accounts for the active profile.
These are documented so that example commands in this manual can be reproduced:

| Name | Injective | EVM |
|---|---|---|
| MetaMask Wallet | `inj1w5v3vhwpk7v8csaqxv5pzzfzvgaqn8qfuh5p5d` | `0x7519165DC1B7987C43A03328110922623A099C09` |
| Test Wallet 2 | `inj1ylxm0a96uxsfk5j7xza7jyycs6zvz9k4r9vkuc` | `0x27CDB7F4BAE1A09B525E30BBE910988684C116D5` |
| igit-dev | `inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5` | `0x85EAC7BC081488AA77D1D82F9CB8E053DE1E4FA8` |
| dev | `inj1p6dn32ss5cxnfgcw8n4mu08x9vc2tskhnj7y3j` | `0x0E9B38AA10A60D34A30E3CEBBE3CE62B30A5C2D7` |
| collab-bob | `inj1kwq44vsld7zk2l9d8vvgn7dkjh4jgvlffhqp3d` | `0xB3815AB21F6F85657CAD3B1889F9B695EB2433E9` |

The Injective column links to each account's owner page. An **Open Blockscout**
link leads to the block explorer for the profile.

These are **testnet development accounts**. They are listed for reproducibility,
not as a recommendation, and they are not production identities.

## Read-only versus write surfaces

Every write in the web application is an EVM transaction signed by your wallet.
Nothing else in the interface changes state.

| Action | Where | Who can do it | Requires writable wallet |
|---|---|---|---|
| Award a contribution badge | Sponsors tab | Repository owner | yes |
| Update revenue splits | Sponsors tab | Repository owner | yes |
| Sponsor a repository | Sponsors tab | Anyone | yes |
| Edit description / default branch | Repository head (pencil) | Repository owner | yes |
| Start / cancel an ownership transfer | Ownership transfer panel | Current owner | yes |
| Accept / reject an ownership transfer | Ownership transfer panel | Proposed new owner | yes |
| Clear an expired transfer | Ownership transfer panel | Anyone | yes |
| Everything else | everywhere | — | no — pure reads |

Everything that is not in that table — browsing code, reading commits and refs,
searching, the monitors, the IPFS explorer, the archive, the Settings page — is
a pure read and works with no wallet at all.

## What the browser is actually doing

Understanding the read path explains most surprising behaviour:

1. **Verify the Suite.** The client checks the chain ID, `suiteVersion()`, the
   active state, `configuredChainId`, every module address, and every module
   code hash. The result is cached for 30 seconds.
2. **Resolve the owner.** An `inj1…` address is used directly; anything else is
   resolved through the username module.
3. **Resolve the repository.** The configured directories are walked, honouring
   the `?suite=` preference, and the canonical owner/name is reported — which is
   how a renamed repository produces the "This repository has moved." notice.
4. **Read the ref.** For Suite v3 the ref carries `commitSha` and `packUris`.
   For Suite v4 it carries a manifest commitment.
5. **Load the packs.** v3 downloads pack objects through an IPFS gateway. v4
   downloads the canonical JSON manifest, verifies it, then fetches each pack
   from the recorded locations and verifies raw SHA-256 and size **before**
   handing the bytes to the Git object reader.
6. **Parse Git objects.** Trees, blobs, commits, and diffs are reconstructed in
   the browser from the verified pack data.

Step 5 is the one to remember: on Suite v4 the digest is checked before the
bytes are trusted, which is what makes a user-owned bucket safe to read from.

## Next

- The latest EVM generation end to end →
  [Chapter 04 · Suite v4](manual-suite-v4.md)
- The earlier EVM generation end to end →
  [Chapter 05 · Suite v2 + v3](manual-suite-v2-v3.md)
- The archive →
  [Chapter 06 · CosmWasm v1 archive](manual-cosmwasm-v1-archive.md)
- Something looks wrong →
  [Chapter 10 · Troubleshooting](manual-troubleshooting.md)
