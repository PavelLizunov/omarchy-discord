# Workspace entry

Read `/home/slovn/Work/omarchy-plugins/AGENTS.md` and its README before work.
Develop in this checkout; the installed copy is a separate deployment target.
This location rule applies in every harness. Historical paths below do not
authorize editing installed files or performing runtime operations.

# Local text-only Discord

- Work in this checkout. Omarchy loads a separate ordinary copy at
  `/home/slovn/.config/omarchy/plugins/quickshell.discord`.
- Preserve user session/keyring, voice, shell theme and unrelated plugins.
  Do not restart the shell or backend without explicit authorization.
- `ClientView.qml`, `SwitcherView.qml`, `components/` and `ui/` must remain
  portable Qt Quick consumers: no Quickshell imports, process execution,
  network requests or desktop queries. Only the local login QR uses Image.
- Keep remote media disabled before request admission; an empty image source
  alone does not prove that no download was scheduled.
- Use actual-consumer `tests/visual` fixtures with QML Preview MCP 0.3.0.
  Inspect PNGs, record hashes/state/locale/DPR and keep interaction tests separate.
- Run `node tests/no-media.cjs`, the existing Node suites and offscreen
  `qmltestrunner -input tests/visual` before syncing the installed copy.
- Preserve native theme zero-radius behavior and inherited font/spacing tokens.
- Measure computational changes using the same synthetic data and workload.
  Shared-shell RSS/CPU includes unrelated plugins and is not Discord-only cost.
