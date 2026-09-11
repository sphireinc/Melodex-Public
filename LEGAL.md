# Melodex legal and usage notes

These notes explain how the current repository is intended to be used. They
are a product and engineering guide, not legal advice. Users and distributors
should get advice appropriate to their own situation.

## Project license status

Melodex does not currently have a project-level open-source license grant.
The repository's `LICENSE` file intentionally records that status instead of
guessing a license or naming a copyright holder without an approved license
decision. Dependencies retain their own licenses; those terms are summarized
in [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).

Before publishing a release as open-source software, the project owner must
choose a project license, add its complete upstream text, and confirm that the
license is compatible with the dependency and asset obligations documented in
the notices file.

## Intended use and user responsibility

Melodex is a local-first tool for cataloging, organizing, enriching, and
playing music that the user owns or is authorized to use. Users are responsible
for:

- obtaining the rights needed for every imported audio, video, artwork, and
  lyrics file;
- complying with copyright, privacy, contract, and platform terms that apply
  to their sources and tools; and
- complying with the licenses and terms of optional external programs and
  services they install or configure.

Melodex does not ship a copyrighted music catalog, advertise unauthorized
copying, bypass DRM, or provide DRM-circumvention instructions. Optional
import helpers do not change the user's responsibility to use authorized
sources.

## Optional external tools and services

`yt-dlp` and `ffmpeg` are optional external programs. The repository's helper
scripts detect them but do not establish a license for a binary obtained by a
user, package manager, operating-system distribution, or other distributor.
Review the license and distribution terms of the exact binaries used in a
build or installation. The current application repository does not include
those binaries as tracked source artifacts.

The application can be configured to use an AI-compatible metadata enrichment
endpoint. When enabled, the configured API key is stored in the local
application settings and is omitted from exported diagnostics. The current
implementation does not claim that the key is encrypted at rest; users should
protect their settings directory and review the selected provider's terms
before enabling remote enrichment. No API key, cookie, or authorization header
belongs in logs, bug reports, or committed files.

The current repository does not define a hosted Melodex API. Any future hosted
service must be reviewed separately and must remain metadata/enrichment-only;
it must not proxy media downloads or distribute lyrics without an appropriate
rights and product review.

## Lyrics, metadata, and artwork

Metadata, artwork, and lyrics may be obtained from different providers and
may have different terms. A successful lookup is not a determination that a
particular use is licensed. Store, display, export, and redistribute those
materials only when the applicable provider and rights terms permit it.

The application should retain source/provenance information where available,
avoid presenting guesses as facts, and avoid placing full copyrighted text in
diagnostics, logs, or hosted metadata responses.

## Release and review expectations

Before distributing an artifact:

1. run `node scripts/check-third-party-notices.mjs` (or `make legal-check`);
2. verify that the exact lockfiles and `go.mod` used for the build are the
   reviewed versions;
3. review any externally installed `yt-dlp`/`ffmpeg` binaries separately;
4. include `LICENSE`, `LEGAL.md`, and `THIRD_PARTY_NOTICES.md` with the source
   checkout and release review materials; and
5. do not describe an unreleased license decision as an open-source grant.
