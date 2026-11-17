# Security policy

This is an independent community project, maintained on a best-effort basis.
There is no commercial support and no guaranteed response time.

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub's private vulnerability
reporting: open the **Security** tab and choose **Report a vulnerability**, or
go to <https://github.com/benemon/openlemm/security/advisories/new>.

Do not open a public issue for a suspected vulnerability.

Include enough detail to reproduce the problem: affected version or commit,
platform, and the impact you observed. A proof of concept helps but is not
required.

The original game's data is not part of this project. Reports about the disc
images themselves are out of scope; please do not attach a copy of the original
disc to a report.

## Supported versions

Fixes land on `main` and ship in the next tagged release. Only the latest
release is supported; there are no backports to earlier tags.

## Verifying a download

Each release has a `checksums.txt` listing the SHA-256 of every archive and
SBOM, a Sigstore bundle `checksums.txt.sigstore.json` signing it, and build
provenance for every archive. Verify the signature with
[cosign](https://docs.sigstore.dev/cosign/system_config/installation/), then
the archive against the checksums:

```sh
version=v0.1.0 os=linux arch=amd64
base=https://github.com/benemon/openlemm/releases/download/$version
archive=openlemm_${version}_${os}_${arch}.tar.gz
curl -fsSL --remote-name-all "$base/$archive" "$base/checksums.txt" "$base/checksums.txt.sigstore.json"
cosign verify-blob checksums.txt --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github\.com/benemon/openlemm/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
grep "  $archive\$" checksums.txt | sha256sum -c -
```

On macOS, use `shasum -a 256 -c -` in place of `sha256sum -c -`. On Windows,
the archive is a `.zip`; in PowerShell, compare
`(Get-FileHash openlemm_v0.1.0_windows_amd64.zip).Hash` with its line in
`checksums.txt` (the case differs; the digits must match).

To check that an archive was built by this repository's release workflow, with
the [GitHub CLI](https://cli.github.com/):

```sh
gh attestation verify "$archive" -R benemon/openlemm
```
