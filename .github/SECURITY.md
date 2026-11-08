# Security

## Reporting a vulnerability

Please report vulnerabilities privately through [GitHub's private vulnerability reporting](https://github.com/myrheimb/pageroom/security/advisories/new) on this repository. Please don't open a public issue for them.

## Threat model

Pageroom edits a website on someone's computer and publishes it to a git host. Its users are often not developers, so safety can't depend on them knowing git or security practice. We try to protect four things: credentials, the user's work, their privacy and their computer.

### Credentials

- Tokens live only in the OS keychain. Pageroom's settings file never holds a secret, and secrets never cross from Go to the UI.
- A token pasted into a repository URL is moved to the keychain.
- Errors and logs pass through a redactor that removes known secrets and token-shaped strings.
- GitHub sign-in uses a GitHub App with the device flow: access is per repository, tokens expire and no client secret is in the binary. The Cloudflare token needs one permission, Workers Scripts: Edit, and is checked before it is saved.
- The preview process gets the user's environment minus variables named like token, secret, password or key.
- Before every commit, changed files are scanned. Publishing stops if one looks like a private key, an `.env` file or a service API key.

### The user's work

- Pushes are never forced. Each push is compare-and-swap, and on a clash Pageroom fetches, merges and retries.
- Overlapping edits from two people become a conflict the user resolves. Nothing on disk changes until they decide.
- Saves are atomic, and every save carries the hash of the version it started from, so a change made elsewhere is noticed.
- Deleted pages and images go to a trash folder, and published versions can be restored from history.

### Privacy

- Image metadata, including GPS, is removed on import.
- Fonts come from the site itself, so visitors' browsers contact no font service.

### The user's computer

- All file access from the UI goes through `fsutil.SafeJoin`, which rejects `..`, absolute paths and symlinks that leave the site.
- The site file server serves only images and fonts from the open site, with `nosniff` and a sandboxing CSP. SVGs with scripts, event handlers or embeds are refused on upload.
- Preview JavaScript runs in an iframe on `127.0.0.1` with a different origin, and the app rejects runtime calls from any origin but its own.
- Hugo listens on `127.0.0.1` only and renders to memory.
- Pageroom uses a Hugo already installed when the version fits. Otherwise it asks before downloading its own copy into its cache. The download is checked against the release's SHA-256 checksums, which guards against corruption, not a replaced release. Nothing is installed system-wide.
- Gallery themes are pinned to a tested commit and checked against its SHA-256 before use. Theme code is otherwise as trusted as on themes.gohugo.io.
- WordPress imports don't resolve XML external entities, accept only a short list of file types, and refuse private, loopback and link-local addresses.
- Content from a git repository is treated as data. Raw HTML shows as text in locked blocks. Opening an untrusted Hugo site is still as risky as running `hugo` on it.
- Production builds use a strict CSP: scripts only from the app, frames only from the local preview.
