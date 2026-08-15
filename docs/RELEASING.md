# Release process

Releases are tag-driven. A valid `vMAJOR.MINOR.PATCH` tag runs tests, builds all native targets, creates checksums and attestations, publishes the GitHub Release, and publishes the matching npm package when `NPM_TOKEN` is configured.

## One-time repository setup

1. Enable GitHub Actions with read/write workflow permissions.
2. For the first npm publication, add an Actions secret named `NPM_TOKEN` containing a short-lived npm granular access token with **Read and write** and **Bypass 2FA** enabled. Legacy/automation tokens are not supported. After the package exists, migrate to npm trusted publishing before removing this secret.
3. Keep GitHub's artifact attestations enabled for the public repository.
4. Protect `main` and require the **CI** workflow before merging.

## Prepare a release

1. Update user-facing documentation and the npm package's baseline version if needed. The workflow always replaces the npm version with the tag, so the checked-in value primarily describes the next intended release.
2. Ensure the release is clean:

   ```sh
   make check
   go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
   shellcheck install.sh uninstall.sh scripts/*.sh
   sh ./scripts/test-distribution.sh
   python3 ./scripts/test-installer-pty.py
   (cd npm && npm test && npm pack --dry-run --ignore-scripts)
   VERSION=v0.1.0 sh ./scripts/build-release.sh
   ./release/ani-tui-linux-x64 --version
   ```

3. Review `release/checksums.txt` and smoke-test at least the host binary against a current ani-cli.
4. Merge the release commit to `main`.

## Publish

Create and push an annotated tag from the exact `main` commit:

```sh
git switch main
git pull --ff-only
git tag -a v0.1.0 -m 'ani-tui v0.1.0'
git push origin v0.1.0
```

The release workflow builds:

- Linux x64, ARM64, and ARMv7;
- macOS x64 and ARM64;
- Windows x64 and ARM64;
- `checksums.txt` covering every binary.

Every Go binary embeds the tag, commit SHA, and deterministic tag-commit UTC time. GitHub's build-provenance action attests the native assets. npm receives the same semantic version and its postinstall downloader fetches the same-tag GitHub asset.

## Verify publication

```sh
gh release view v0.1.0
gh release download v0.1.0 --pattern 'ani-tui-linux-x64' --pattern checksums.txt
sha256sum --check checksums.txt --ignore-missing
gh attestation verify ./ani-tui-linux-x64 --repo Killersumit/ani-tui
npm view ani-tui@0.1.0 version
```

Then exercise both documented install paths in clean environments. If npm was intentionally skipped because `NPM_TOKEN` was absent, configure the secret and publish from the exact release commit; do not rebuild or reuse a different binary tag.

Never move or overwrite a published version tag. Fix a bad release with a new patch version.
