# Melodex

Melodex is a desktop music library for importing, organizing, enriching, and
playing the music you keep on your own computer.

It gives your collection one calm place to live: predictable folders, useful
metadata, album art, lyrics, track profiles, playlists, queues, and a player
that stays close to the library.

## Use and rights

Melodex is for music you own or are authorized to use. You are responsible for
following copyright law and the terms that apply to your files, sources, and
optional tools.

Melodex does not ship copyrighted music, bypass DRM, or provide circumvention
tools. It is a local-first library manager with optional metadata enrichment
and import helpers.

## What it does

- Organizes a local music library.
- Cleans up metadata with optional AI assistance.
- Adds lyrics where the source and rights allow it.
- Keeps album art and track profiles alongside the music.
- Builds playlists and queues for everyday listening.
- Imports local files and authorized sources.

## Quick start

From the repository root:

```bash
./install.sh  # check prerequisites and install frontend dependencies
./launch.sh   # build and open Melodex
```

Use `./dev.sh` when you are working on the app and `./build.sh` when you want
to create a release build. Run `make help` for the complete command list.

By default, Melodex stores the library in:

`~/Music/Melodex Music`

You can choose a different library folder from Settings.

## Troubleshooting

For more detail while diagnosing a problem, enable debug logging:

```bash
MELODEX_DEBUG=1 ./dev.sh
```

To also write the log to a specific file:

```bash
MELODEX_DEBUG=1 MELODEX_LOG_FILE=/tmp/melodex-dev.log ./dev.sh
```

## Checks

The repository includes Go backend tests, frontend helper tests, and a
production frontend build. The same checks run in CI.

```bash
go test ./...
cd frontend
npm ci
npm test
npm run build
```

## More information

See [LEGAL.md](LEGAL.md) for usage and licensing notes and
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for dependency attribution.
