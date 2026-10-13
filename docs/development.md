# Development contract

## Instruction scope

The repository instruction set is `AGENTS.md` and this document. Parent workspace and active harness instructions remain controlling. README, changelog, dated reports and historical verification are references, not approval or proof of current acceptance.

## Fork boundary

The pinned upstream revision and allowed fork paths are in [the boundary manifest](../scripts/fork-boundary.json). Upstream-identical files remain byte-identical. Existing modified upstream files may receive fixes in their fork layer, not wholesale cleanup. The pinned `backend/internal/voicewire` port retains its [license](../backend/internal/voicewire/LICENSE) and [patch provenance](../backend/internal/voicewire/OMACORD-PATCH.md); do not format or purge it for hygiene. Update the boundary deliberately when taking an upstream version, not to hide an unrelated diff.

`python3 scripts/check-repository.py` validates upstream bytes, upstream-file hook limits, documentation links, the instruction set and the bundled source fingerprint. `--upstream-ref` allows an immutable locally verified reference when the pinned commit is unavailable in a downloaded archive. A Git checkout is required for the upstream comparison; an archive alone cannot establish upstream preservation.

## Source and application

Develop in the checkout, not the independent installed plugin. Read installers before running them: mirroring can delete installed local changes, restart the backend and reload shared UI. Preserve profiles, credentials, drafts, staged attachments, pending sends and active jobs. Stage checked candidates outside watched trees when applying an update. Prefer supported bounded reloads over process replacement. Never launch a second Quickshell or terminate the desktop session to test a plugin.

For implementation requests, compare installed hashes and actual loaded component identity with the checked candidate. For documentation-only work, do not interrupt a live call merely to deploy prose. Keep current user state rather than restoring a stale baseline over a concurrent session.

## Verification

Use [the executable checks](checks.md). Native UI changes require an actual-consumer inert offscreen fixture, explicit imports/readiness/locale, dependency hashes and inspected images. Rendering, geometry, interaction and physical user acceptance are separate evidence. A mocked backend, decoded packet or `Image.Ready` alone does not prove real Discord media or audible playback. Do not capture the desktop, activate devices, send test messages or mutate the current account merely for a hygiene check.

Keep architectural, packaging, security and delayed-state source-text tests unless their contract is demonstrably obsolete. QML type names, signals and native entry points can be implicit consumers; one textual occurrence is not proof of dead code. Preserve license headers, tool directives and comments explaining real protocol or lifecycle invariants. Remove only demonstrated obsolete noise with a syntax-aware equality check.

Use a finite isolated test worker or existing CI for builds and full test suites. Local native offscreen verification can cover host-specific imports; record which checks CI cannot run. Missing tooling or failed review remains a gap, never PASS.

## Changes and publication

Define intent, invariants and executable checks before a non-trivial change. Record actual outcomes outside the repository rather than adding raw journals or copied reports. Preserve the active branch and index; isolate checked task-owned changes from concurrent work. Use English project documentation, comments and commit messages. Follow the current user's publication policy; do not create PRs, merge, rewrite history or release merely because a reference procedure suggests it.

Review independent findings against exact source citations and executable evidence. A configured route or family-looking alias is not backing-model attestation. If no verified independent review ran, say so.
