# Release

## Build, test, and release

```bash
go build ./...
go test ./... -count=1
go test -race ./... -count=1
make ci   # vet + lint + format + tidy + 500-line cap + build + test + race
```

`make ci` is the full local check CI runs (`.github/workflows/ci.yml`,
reported as `go-ci / ci-complete`). See `CONTRIBUTING.md` for the lane
contract. It needs Go 1.26.10 or newer, and `go.mod` is the version of
record.

Releasing works like this. `.github/workflows/release.yml` runs on
every push to `main` and on every pushed tag
`vMAJOR.MINOR.PATCH[-pre]`. Nothing runs on a pull request, and the
workflow deploys nothing. A push to `main` publishes the moving
`main`, `latest`, and `sha-<full-sha>` tags with the same signature
and attestation hardening as a version release, so use a moving tag
only where floating is intended and keep releases pinned to digests.

The workflow won't publish a version release unless the tag points at
the protected `main` head, so a `v*` tag cut from any other commit
publishes nothing. As defence in depth, restrict who can create `v*`
tags with a tag ruleset limited to maintainers.

1. Set `metadata.version` in `manifest.yaml` to the version you're
   releasing and merge that change to `main`. Then tag the merge
   commit and push the tag. The workflow refuses to publish unless the
   tag equals `v` plus that version (a `-pre-release` suffix on the tag
   is allowed when the manifest version has none), and it checks this
   before it builds or pushes anything.
2. The workflow builds `linux/amd64`, the only platform the manifest
   declares, pushes
   `ghcr.io/eshu-hq/eshu-collector-package-registry:<tag>` (or `:main`,
   `:latest`, and `:sha-<full-sha>` for a `main` push), writes the
   digest to the job summary, signs the digest with cosign (keyless),
   and attaches a SLSA build-provenance attestation and an SPDX SBOM
   attestation. It needs no secret beyond the job's `GITHUB_TOKEN`.
3. Copy the
   `ghcr.io/eshu-hq/eshu-collector-package-registry@sha256:<digest>`
   reference from the job summary into the activation's `seed.image`.
   The deployment stamps it into the manifest with
   `collector seed --image-ref`.
