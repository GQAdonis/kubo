# UAR settings fixtures for check-uar

Recorded shapes for `mcp-agent-principal-enforcement` task 1.3 (ADR-0007). Each `settings-*.json` validates against `../../agentic/uar-effective-settings.schema.json`; its `$schema` key is a test aid that `check-uar` ignores.

| File | Expected verdict (principal with `egress_ack: []`, listen `127.0.0.1:5050`, `check: verify`) |
|------|------|
| `settings-shipped-profile.json` | `ok` |
| `settings-uar-defaults.json` | `refuse` (`remote_model_not_acknowledged`, `remote_embedder_not_acknowledged`, plus `governance_disabled`) |
| `settings-shell-enabled.json` | `read_only` (`shell_tool_enabled`, `non_kubo_mcp_server`) |
| `settings-no-inventory.json` | `read_only` (`tool_inventory_unreadable`) |
| `observed-records-a7cb9729.json` | `refuse` (`settings_unreadable`): the per-key record array UAR serves at `GET /api/uar/settings` (`src/uar/api/settings.rs:616`, HEAD `a7cb9729`) does not validate. Values are illustrative; field names are from `SettingsWithMetaResponse` (`settings.rs:522`). |

These are recorded from UAR source, not from a published UAR release. `uar-integration-spike` refreshes them when a release is evaluated.
