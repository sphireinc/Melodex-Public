# Melodex third-party notices

This file identifies the direct dependencies and optional external tools used
by the current repository. Exact versions come from `go.mod` and
`frontend/package-lock.json`; the lockfiles remain authoritative when a
dependency is upgraded. Run `node scripts/check-third-party-notices.mjs` to
verify that every direct dependency declared by those manifests is represented
below.

This file is an attribution index, not a replacement for the license text
distributed by each dependency. The upstream license and notice files should
be preserved in dependency distributions and must be reviewed before creating
a redistributable bundle. Transitive npm dependencies are intentionally not
hand-copied into this document: their exact graph is recorded by the lockfile,
and their package metadata/license files are the source of truth after `npm
ci`. The checker prevents the direct-dependency index from silently falling
behind the manifests.

## Direct Go dependencies

| Module | Version | License / notice action | Upstream |
| --- | --- | --- | --- |
| `github.com/faiface/beep@v1.1.0` | `v1.1.0` | MIT; retain the upstream `LICENSE` | <https://github.com/faiface/beep> |
| `github.com/wailsapp/wails/v3@v3.0.0-beta.2` | `v3.0.0-beta.2` | MIT; retain the upstream `LICENSE` | <https://github.com/wailsapp/wails> |

The Go module graph also contains indirect modules. Their exact versions are
resolved by `go.sum` and the Go module graph used for a build; do not remove
their upstream license files when vendoring or packaging dependencies.

## Direct frontend dependencies

The versions below are the versions resolved in the current lockfile.

| Package | Version | License / notice action | Upstream |
| --- | --- | --- | --- |
| `@dnd-kit/core@6.3.1` | `6.3.1` | MIT; retain upstream notices | <https://github.com/clauderic/dnd-kit> |
| `@dnd-kit/sortable@10.0.0` | `10.0.0` | MIT; retain upstream notices | <https://github.com/clauderic/dnd-kit> |
| `@dnd-kit/utilities@3.2.2` | `3.2.2` | MIT; retain upstream notices | <https://github.com/clauderic/dnd-kit> |
| `@wailsio/runtime@3.0.0-beta.1` | `3.0.0-beta.1` | MIT; retain upstream notices | <https://github.com/wailsapp/wails> |
| `react@18.3.1` | `18.3.1` | MIT; retain upstream notices | <https://github.com/facebook/react> |
| `react-dom@18.3.1` | `18.3.1` | MIT; retain upstream notices | <https://github.com/facebook/react> |
| `react-icons@5.6.0` | `5.6.0` | MIT; retain upstream notices and icon-set attribution terms | <https://github.com/react-icons/react-icons> |
| `video.js@8.23.7` | `8.23.7` | Apache-2.0; retain upstream notices | <https://github.com/videojs/video.js> |

## Direct development and build dependencies

These packages reproduce the frontend checks and build. They are not
necessarily bundled in the packaged application, but their notices still need
to remain available to developers and release builders.

| Package | Version | License / notice action | Upstream |
| --- | --- | --- | --- |
| `@playwright/test@1.62.1` | `1.62.1` | Apache-2.0; retain upstream notices | <https://github.com/microsoft/playwright> |
| `@types/react@18.3.29` | `18.3.29` | MIT; retain upstream notices | <https://github.com/DefinitelyTyped/DefinitelyTyped> |
| `@types/react-dom@18.3.7` | `18.3.7` | MIT; retain upstream notices | <https://github.com/DefinitelyTyped/DefinitelyTyped> |
| `@typescript-eslint/eslint-plugin@8.33.1` | `8.33.1` | MIT; retain upstream notices | <https://github.com/typescript-eslint/typescript-eslint> |
| `@typescript-eslint/parser@8.33.1` | `8.33.1` | MIT; retain upstream notices | <https://github.com/typescript-eslint/typescript-eslint> |
| `@vitejs/plugin-react@6.0.2` | `6.0.2` | MIT; retain upstream notices | <https://github.com/vitejs/vite-plugin-react> |
| `eslint@8.57.1` | `8.57.1` | MIT; retain upstream notices | <https://github.com/eslint/eslint> |
| `eslint-import-resolver-typescript@3.6.1` | `3.6.1` | ISC; retain upstream notices | <https://github.com/import-js/eslint-import-resolver-typescript> |
| `eslint-plugin-import@2.32.0` | `2.32.0` | MIT; retain upstream notices | <https://github.com/import-js/eslint-plugin-import> |
| `eslint-plugin-react-hooks@4.6.2` | `4.6.2` | MIT; retain upstream notices | <https://github.com/facebook/react> |
| `eslint-plugin-react-refresh@0.4.24` | `0.4.24` | MIT; retain upstream notices | <https://github.com/ArnaudBarre/eslint-plugin-react-refresh> |
| `prettier@3.8.3` | `3.8.3` | MIT; retain upstream notices | <https://github.com/prettier/prettier> |
| `typescript@5.8.3` | `5.8.3` | Apache-2.0; retain upstream notices | <https://github.com/microsoft/TypeScript> |
| `vite@8.0.16` | `8.0.16` | MIT; retain upstream notices | <https://github.com/vitejs/vite> |

The exact transitive dependency graph is recorded in
`frontend/package-lock.json`. The checker verifies the direct declarations;
release builders must preserve the license metadata and notice files supplied
by transitive packages when they are redistributed.

## Optional external programs

| Program | Repository role | Notice requirement |
| --- | --- | --- |
| `yt-dlp` | Optional user-configured import helper | The repository does not bundle it. Review the license and terms of the exact installed release and preserve its notices when redistributing it. |
| `ffmpeg` | Optional user-configured media tool | License obligations depend on the exact build configuration and whether optional GPL components are enabled. Review the exact binary's notices before redistribution. |

## Generated assets and application packaging

Wails-generated bindings, platform metadata, and application packaging output
are generated from this repository and its declared dependencies. They do not
replace the upstream notices for bundled third-party code. A release review
must inspect any newly added font, icon, image, codec, or platform asset and
add its attribution here before distribution. Do not assume that an asset is
free to redistribute solely because it was found online.

## License text and source availability

Melodex's project-level licensing status is recorded in [`LICENSE`](LICENSE).
That status does not change the separate licenses of the dependencies above.
For a source checkout, the pinned manifests and lockfiles are:

- [`go.mod`](go.mod) and [`go.sum`](go.sum)
- [`frontend/package.json`](frontend/package.json) and
  [`frontend/package-lock.json`](frontend/package-lock.json)

When packaging a dependency or its license text into a release, use the
upstream artifact for the exact resolved version rather than copying a license
from a different release.
