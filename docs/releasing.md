# Maintainer release procedure

The delivery implementation is on a feature branch until reviewed and merged.
No production signing key, release tag or public release is created by this change.

1. Review/merge the code and resolve the project's license decision before public
   distribution. GitHub Releases are prepared from a pushed version tag, for example
   `v0.1.0-alpha.1`. Do not push a tag until ready to run the release workflow.
2. Privately provision a long-lived Android release keystore, keep an offline
   backup, and record its SHA-256 certificate fingerprint. Never commit the key or
   use a disposable fixture/debug key as the public identity. Losing this key
   prevents normal upgrades of installed releases.
3. Create a protected GitHub Actions environment named `release` (restrict it to
   reviewed tags and use required reviewers). Add environment secrets:
   `ANDROID_KEYSTORE_BASE64`, `ANDROID_STORE_PASSWORD`, `ANDROID_KEY_ALIAS`,
   `ANDROID_KEY_PASSWORD`. Add variable `ANDROID_CERT_SHA256` with the certificate
   fingerprint; colon-delimited or plain hex are accepted. No secrets go to PR jobs.
4. After review, push the chosen tag from the intended commit. The workflow pins
   Go 1.23.12, GoReleaser 2.8.2, JDK 21 and Android build-tools 36.0.0. Gradle/AGP
   versions remain pinned in the repository. Native tests, package matrix validation,
   Android tests/lint, APK signing/certificate and package/version checks must pass.
5. The final job downloads all artifacts, includes `install.sh`, `INSTALL.md` and
   `android-version-code.txt`, creates aggregate SHA-256 checksums and opens a
   **draft prerelease**. Inspect/test every platform needed before publishing it
   manually. Native binaries are not OS-code-signed/notarized. Windows is SSH-only;
   see INSTALL.md for WSL hosting limitations.

APK versionName is the tag without `v`; versionCode is this workflow run number
plus 1000. Keep this workflow identity/counter stable. The draft job checks every
prior release/draft's `android-version-code.txt`, refusing duplicate tags or
non-increasing codes (including an older run retried after a newer release).
Existing releases are never overwritten on rerun. To retry a failed draft, inspect
and deliberately remove that draft first, or choose a new tag; never overwrite a
published tag. If migrating workflows, adjust the offset above the highest code.
The release key is deleted from the runner after build, including failure paths.

Local verification without publishing (package checks require `dpkg-deb` and `bsdtar`, provided by dpkg/libarchive-tools on Debian-based build machines):

```sh
go test -race ./...
go vet ./...
python3 -m unittest discover -s scripts -p 'test_*.py'
goreleaser check
goreleaser release --snapshot --clean
python3 scripts/verify_native_packages.py dist
```

To build an actual signed APK locally, provide `MODEX_KEYSTORE`,
`MODEX_STORE_PASSWORD`, `MODEX_KEY_ALIAS`, `MODEX_KEY_PASSWORD`,
`MODEX_RELEASE_VERSION` (without v), and `MODEX_VERSION_CODE` privately in the
process environment. `:app:assembleRelease` fails without them. Verify using
`scripts/verify_release_apk.py` with `MODEX_CERT_SHA256`, expected version/code and
`--build-tools` pointing to Android SDK build-tools 36.0.0. Keep passwords out of
shell history and logs. Debug builds still use the existing local debug keystore.

Checksums confirm file integrity, not independent publisher identity. Reproducible
build attestation, macOS notarization, Authenticode signing and native Windows host
support are not claimed by this initial delivery workflow.
