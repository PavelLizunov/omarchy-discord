# Removal

Preserve drafts, then disable the plugin before removing its runtime:

```sh
omarchy plugin disable quickshell.discord
bash scripts/remove-runtime.sh
```

Runtime removal requires a successful backend stop, removes the user unit/executable and moves matching configuration into a dated backup. It does not remove the plugin source copy. `--purge` additionally deletes plugin configuration/cache and attempts to clear matching keyring entries; use it only when you intend those losses and verify keyring removal independently.
