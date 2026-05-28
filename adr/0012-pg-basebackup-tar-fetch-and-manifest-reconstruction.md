# ADR-0012: pg_basebackup streams a single tar to stdout; backup_manifest is reconstructed on Databasus

- **Status:** Accepted
- **Date:** 2026-05-27
- **Tags:** backups, postgresql, physical, pg_basebackup, compression, manifest

## Context

[ADR-0008](./0008-why-pg17-native-backups-with-mandatory-wal-summary.md)
commits Databasus to driving PG 17's native physical-backup binaries;
[ADR-0009](./0009-why-remote-physical-backups-instead-of-agents.md)
commits to running them remotely from the Databasus host. Within those
constraints the FULL/INCR executor needs to:

1. Move backup bytes from PG to storage **without staging anything on
   local disk**. The Databasus host serves many sources; staging a 3 TB
   database per concurrent backup would require tens of TB of dedicated
   disk that does nothing useful between backups.
2. Produce **one self-contained artifact** per backup, ready for
   one-shot restore. The artifact must include base data and the WAL
   needed to make it consistent.
3. **Preserve `backup_manifest` alongside every artifact.**
   `pg_basebackup --incremental=<parent_manifest>` reads the parent
   manifest to identify changed blocks via WAL summaries — no manifest
   means no incrementals, which ADR-0008 forbids.

`pg_basebackup` exposes its feature matrix through CLI flags, and the
wrong combination silently breaks one of the three constraints above.
This ADR records the chosen flag set and the trade-offs each flag
carries, including the conflict between server-side compression and
embedded manifest that forces us to reconstruct the manifest ourselves.

## Decision

The FULL / INCR executor invokes:

```
pg_basebackup
  --pgdata=-
  --format=tar
  --wal-method=fetch
  --compress=server-zstd:5
  --no-manifest
  --manifest-checksums=SHA256
  -h <host> -p <port> -U <user>
```

The stdout byte stream is teed on Databasus into two branches:

```
pg_basebackup.Stdout
  → io.MultiWriter:
     ├─→ EncryptionWriter → CountingWriter → storage.SaveFile(".tar.zst")
     └─→ zstd.Reader → tar.Reader → ManifestBuilder
                                  → storage.SaveFile(".tar.zst.manifest")
```

The reconstructed `backup_manifest` is stored as a separate sidecar
object next to `<file>.tar.zst` and the existing `<file>.metadata`.
INCR runs download the sidecar, materialise it as a temp file and pass
it to `pg_basebackup --incremental=<temp>`.

The reconstructed manifest is **byte-exact** against PG's own
`backup_manifest.c` output. A CI test runs paired backups against the
same source — one with PG-generated manifest, one with our
reconstruction — and asserts byte-identical output.

Each flag choice below stands on its own; the reconstruction follows
mechanically from the conflict between two of them.

### `--format=tar --pgdata=-`

`--format=plain` writes a directory tree. With `--pgdata=-` it demands
a real disk path, breaking the no-staging constraint; with a staging
path it costs ~1 TB of plain (uncompressed!) data on disk per
concurrent backup. Both rejected upfront.

`--format=tar --pgdata=-` produces one tar that streams to stdout as a
continuous byte sequence, piped directly into the storage
`SaveFile(io.Reader)` interface.
[ADR-0010](./0010-no-support-for-customer-tablespaces.md) already
forbids custom tablespaces, so pg_basebackup emits exactly one tar
(`base.tar`) — no concatenation logic on our side.

### `--wal-method=fetch`

`--wal-method=stream` opens a second replication connection alongside
the data connection and writes WAL to a separate output
(`pg_wal.tar` next to `base.tar` or a `pg_wal/` directory). With
`--pgdata=-` there is no second output channel; pg_basebackup refuses
with `cannot stream write-ahead logs in tar mode to stdout`. The
choice is mechanical, not philosophical — `stream` would have given us
real-time WAL during the backup window, but the cost is breaking the
single-byte-stream invariant.

`--wal-method=fetch` collects WAL through the same replication
connection at end-of-backup and appends segments as `pg_wal/...` tar
entries inside the single tar. Self-contained artifact preserved.

Fetch carries one weakness — the source must retain WAL from
`start_lsn` to `stop_lsn` until pg_basebackup asks for it at the end
of the backup. PG default `wal_keep_size=0` does not guarantee that on
busy clusters. The per-backup replication slot helper
(`backup_slot.go`) pins WAL on the source for the duration of the
backup and drops it in defer; fetch never sees `requested WAL segment
X has already been removed`.

### `--compress=server-zstd:5`

On 1-3 TB databases the dominant cost is the PG→Databasus link.
`--compress=client-zstd:5` sends 3 TB **uncompressed** over the wire
and compresses on Databasus. `--compress=server-zstd:5` compresses on
the PG host and sends ~1 TB. Concrete impact at zstd:5's typical ~3x
ratio on PG data:

| Link              | client-zstd  | server-zstd |
|-------------------|--------------|-------------|
| 1 Gbit/s LAN      | ~7 hours     | ~2.3 hours  |
| 10 Gbit/s LAN     | ~40 minutes  | ~14 minutes |
| 100 Mbit/s WAN    | ~3 days      | ~22 hours   |

The trade is "more PG-side CPU during the backup window" for "less
wire time". On a window that spans hours, that is the right direction.
If the source is heat-sensitive, `--compress=server-zstd:N` with a
lower level dials CPU down at a small ratio cost.

### Reconstructing the manifest

`--compress=server-zstd` plus `--format=tar --pgdata=-` together break
manifest emission. pg_basebackup refuses with `cannot inject manifest
into a compressed tar file`. The constraint is mechanical: with stdout
output, the manifest would normally be appended as a final tar entry;
with the tar already compressed by the time pg_basebackup hands bytes
out, it cannot re-open the stream to append. PG offers no flag to
redirect the manifest to a side channel under `--pgdata=-`.

`--no-manifest` removes the conflict but takes manifests away
entirely, which breaks incrementals (ADR-0008).

We reconstruct. Every manifest field is derivable on Databasus from
data the tar stream already carries plus a few values on hand:

| Field                                | Source                                                                |
|--------------------------------------|-----------------------------------------------------------------------|
| `Path`, `Size`, `Last-Modified`      | tar header                                                            |
| `Checksum` (SHA-256 of file content) | `sha256` over `tar.Reader` body                                       |
| `System-Identifier`                  | the `PostgresqlPhysicalDatabase` row                                  |
| `WAL-Ranges`                         | parsed from `pg_basebackup` stderr (already parsed for start/stop LSN)|
| `Manifest-Checksum`                  | SHA-256 of our own serialised output                                  |

`PostgreSQL-Backup-Manifest-Version` has been `1` since PG 13 and the
PG 17 incremental feature did not bump it; the format is stable enough
to target. The byte-exact serialisation risk is mitigated by the
paired-backup CI test above — any drift introduced by a future PG
release fails in PR before reaching production.

## Alternatives considered

- **Stay on `--compress=client-zstd:5`.** Zero new code. Rejected:
  7+ hours per FULL on gigabit LAN is not viable as fleets grow into
  TB-scale databases. WAN is unusable.

- **Local staging (`--pgdata=<dir> --compress=server-zstd:5`).** PG
  emits its own manifest as a side file `backup_manifest`; no
  reconstruction needed. Rejected because it requires ~1 TB of
  staging disk per concurrent backup — exactly the constraint the
  streaming pipeline was designed to avoid. A backup host serving 10
  sources would need tens of TB of staging-only disk sitting cold
  between backups.

- **Speak the replication protocol directly (pgMoneta-style).** Send
  `BASE_BACKUP COMPRESSION 'server-zstd:5' MANIFEST 'yes'` over a
  libpq replication connection; manifest arrives as a separate
  CopyData stream byte-exact from PG. Rejected because it requires
  re-implementing pg_basebackup's full surface — multi-tablespace
  handling, WAL fetch trailer, error recovery, incremental protocol —
  and maintaining that against every PG release. The single benefit
  (no reconstruction) costs 2-3 KLOC of replication-protocol code
  carried for our use only. The spirit of ADR-0008 — use PG's native
  binaries, don't fork them — applies; if we ever need features
  pg_basebackup cannot provide (mid-stream heartbeats for WAN,
  structured progress events) this is the path we revisit.

## Consequences

### Positive

- PG→Databasus traffic compressed ~3x. 1-3 TB backups complete in
  hours, not days; WAN deployments become viable.
- Self-contained tar artifact preserved (base + WAL + reconstructed
  manifest sidecar).
- Incremental chain unblocked — manifest sidecar is consumed by
  `--incremental=` exactly as PG's own would have been.
- No new deployment requirement. pg_basebackup CLI remains the
  workhorse and continues to own retry, error mapping and
  multi-tablespace handling for free.

### Negative

- ~500-700 LOC of manifest reconstruction code (zstd reader + tar
  walker + byte-exact JSON serialiser) to maintain.
- Byte-exact serialisation risk against PG manifest format. Mitigated
  by paired-backup CI test; minor PG releases that drift the format
  are caught at PR-time, not in production.
- Net CPU on Databasus to decompress and SHA-256 the full backup
  contents — roughly 25 minutes on 4 threads at zstd-decompress +
  SHA-256 throughput for a 3 TB backup. Acceptable on a dedicated
  backup host; tuning levers are server-side zstd level and the
  manifest-path worker count on Databasus.