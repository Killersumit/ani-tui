# Security policy

## Supported versions

Until ani-tui reaches 1.0, security fixes are made on the latest released minor version only. Users should upgrade to the newest GitHub or npm release before reporting a problem.

## Reporting a vulnerability

Please **do not open a public issue** for a suspected vulnerability. Use GitHub's private advisory form:

<https://github.com/Killersumit/ani-tui/security/advisories/new>

Include the affected ani-tui version (`ani-tui --version`), platform, impact, reproduction steps, and any suggested mitigation. Remove credentials, private filesystem paths, anime history, and other personal data from attachments.

You should receive an acknowledgement within seven days. A validated report will be investigated privately, fixed on a coordinated timeline, and credited in the advisory unless you prefer to remain anonymous.

## Security boundaries

- ani-tui launches the separately installed `ani-cli` backend and the player/package-manager commands explicitly shown in its UI.
- The dependency doctor asks before installing anything, uses curated official sources, executes direct argument vectors, and retries only explicit transient network failures.
- Release installers verify SHA-256 checksums. GitHub release binaries also receive build-provenance attestations.
- The loopback menu bridge uses a per-run authentication token, but it is local IPC—not a network service intended for remote exposure.
