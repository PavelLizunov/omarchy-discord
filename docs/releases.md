# Testing release: 0.2.0-beta.3

This is a focused sign-in recovery update to beta.2. Download the complete Linux x86_64 plugin archive from the GitHub release, not only a backend executable.

QR login now has separate Back to sign-in and Try again actions, clears cancelled QR state, and reads the encrypted token after decoding the ticket response. Discord CAPTCHA requirements are reported explicitly. CAPTCHA completion and email/password authentication are not implemented in Omacord; the masked input accepts a Discord user token.

The reported password screen on another computer and its HTTP 400 response are not yet identified. This release does not claim to repair that unidentified password flow or establish successful account authentication.

The backend was built for this release. No new tests, previews, account requests or installation on the other computer were performed. Earlier beta.2 verification is historical and is not a pass for this candidate. Voice and video retain beta.2 limitations.

Installation replaces the installed plugin copy, installs the user backend unit, can restart the backend and rescans the shared shell. It does not intentionally log out the account. Keep unsent work and installed customizations before updating. Use the release's complete archive and run `bash scripts/install-local.sh --section right` from its extracted directory.
