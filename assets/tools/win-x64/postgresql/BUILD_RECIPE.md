# Windows `pg_basebackup.exe` — self-built notes

## Why

Upstream Windows `pg_basebackup.exe` (EnterpriseDB, MSYS2, all known builds) writes
stdout in CRT text mode. Every `0x0A` byte gets rewritten to `0x0D 0x0A`, which
corrupts the binary tar/zstd payload produced by `--pgdata=-`. We rely on
`--pgdata=-` to stream backups through Go without a host-side scratch directory,
so the upstream binary is unusable for us on Windows.

The patched binaries in `postgresql-17/bin/pg_basebackup.exe` and
`postgresql-18/bin/pg_basebackup.exe` add one call to `_setmode(_fileno(stdout),
_O_BINARY)` early in `main()` and are otherwise identical to upstream
PostgreSQL 17.9 / 18.4.

## When to rebuild

- New major PostgreSQL release we want to support (PG 19, PG 20, ...).
- Reported pg_basebackup CVE on the minor we ship (rare — fixes usually land server-side).
- `--incremental` wire-protocol change (unlikely within a major version).

`pg_combinebackup`, `psql`, `pg_restore`, `pg_dump`, `pg_receivewal` and `libpq.dll`
do NOT have this problem (they write to files, not stdout) and are kept from the
upstream EDB-style bundle.

## Patch

Identical for every supported major. Apply to `src/bin/pg_basebackup/pg_basebackup.c`:

1. After the existing `#ifdef HAVE_LIBZ ... #endif` include block near the top,
   add:

   ```c
   #ifdef WIN32
   #include <fcntl.h>
   #include <io.h>
   #endif
   ```

2. Right after `pg_logging_init(argv[0]);` in `main()`, add:

   ```c
   #ifdef WIN32
       _setmode(_fileno(stdout), _O_BINARY);
   #endif
   ```

## Build

### Toolchain

MSYS2 (https://www.msys2.org/) — open the **UCRT64** shell.

```
pacman -Syu --noconfirm
pacman -S --noconfirm --needed \
  mingw-w64-ucrt-x86_64-gcc \
  mingw-w64-ucrt-x86_64-meson \
  mingw-w64-ucrt-x86_64-ninja \
  mingw-w64-ucrt-x86_64-pkgconf \
  mingw-w64-ucrt-x86_64-zstd \
  mingw-w64-ucrt-x86_64-lz4 \
  mingw-w64-ucrt-x86_64-zlib \
  mingw-w64-ucrt-x86_64-openssl \
  mingw-w64-ucrt-x86_64-icu \
  mingw-w64-ucrt-x86_64-libxml2 \
  mingw-w64-ucrt-x86_64-libxslt \
  bison flex perl python make diffutils
```

### Source

Download the latest minor for each major from https://ftp.postgresql.org/pub/source/:

```
curl -O https://ftp.postgresql.org/pub/source/vX.Y/postgresql-X.Y.tar.bz2
tar -xjf postgresql-X.Y.tar.bz2
```

### Configure + build

```
cd postgresql-X.Y
meson setup build --buildtype=release \
  -Dssl=openssl -Dzstd=enabled -Dlz4=enabled -Dicu=enabled \
  -Dreadline=disabled -Dtap_tests=disabled
ninja -C build src/bin/pg_basebackup/pg_basebackup.exe
```

Output: `build/src/bin/pg_basebackup/pg_basebackup.exe`.

## Install into assets

```
cp build/src/bin/pg_basebackup/pg_basebackup.exe \
   assets/tools/win-x64/postgresql/postgresql-<major>/bin/pg_basebackup.exe
```

MSYS2 UCRT64 builds link `libintl-8.dll`, while the EDB-bundled siblings link
`libintl-9.dll`. Both must live side-by-side in `bin/`:

```
cp /c/msys64/ucrt64/bin/libintl-8.dll \
   assets/tools/win-x64/postgresql/postgresql-<major>/bin/libintl-8.dll
```

All other DLLs (`libpq.dll`, `liblz4.dll`, `libzstd.dll`, `zlib1.dll`,
`libiconv-2.dll`, `libcrypto-3-x64.dll`, `libssl-3-x64.dll`,
`libwinpthread-1.dll`) are already in `bin/` from the upstream bundle.

Verify with `objdump -p pg_basebackup.exe | grep "DLL Name"` — every listed
DLL must exist next to the binary.

## Versions in tree today

- PG 17 → built from PostgreSQL 17.9 source.
- PG 18 → built from PostgreSQL 18.4 source.

Bump these when you rebuild and update this file.
