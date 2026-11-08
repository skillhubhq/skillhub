# Local configuration and secrets

The CLI and App share a device-local configuration store. Hubs never receive saved values, credential-store references or environment values. This feature manages configuration requirements; it does not run agents, inject values into running processes, generate `.env` files or provide a security sandbox.

## Declare requirements

A skill author may include `skillhub.config.json` beside `SKILL.md`:

```json
{
  "version": 1,
  "variables": [
    { "name": "SERVICE_TOKEN", "description": "Token for the service used by this skill", "secret": true, "required": true },
    { "name": "SERVICE_REGION", "description": "Service region", "secret": false, "required": false }
  ]
}
```

This optional SkillHub declaration is not part of the universal Agent Skills specification. The skill remains usable with ordinary environment variables and the tool's own configuration. Declarations contain names, descriptions and requirements only: no values, defaults, commands or executable providers. Version 1 allows up to 32 unique uppercase variable names and a 32 KiB document. Unknown fields are rejected. Published declarations are reviewed as part of the immutable skill archive.

## Configure an installation

In the App, use **My library → Installed → Configure**, or **My library → Configuration**. Each installation has independent settings, even when the same skill appears in multiple projects or tools. Choose:

| Storage | Behavior |
| --- | --- |
| System credential store | Stores a value in macOS Keychain, Windows Credential Manager or Linux Secret Service; unavailable stores fail without a plaintext fallback |
| Environment reference | Checks the variable with the declared name in the current process; never copies its value into SkillHub |
| Local non-secret value | Stores ordinary configuration in the private client data directory; unavailable for variables declared secret |

CLI equivalents:

```sh
skillhub config list
skillhub config doctor /path/to/installed/skill
skillhub config set /path/to/installed/skill SERVICE_TOKEN --mode environment
skillhub config set /path/to/installed/skill SERVICE_TOKEN --mode keychain --value-stdin
skillhub config unset /path/to/installed/skill SERVICE_TOKEN --yes
skillhub config clear /path/to/installed/skill --yes
```

For `--value-stdin`, pipe output from a trusted credential tool rather than entering a secret literally into a command or shell history. Diagnostics and JSON output show availability only, never saved values or keychain references. `doctor` exits unsuccessfully when required configuration is missing, the store is unavailable or the installed version needs review. Optional missing variables do not block readiness.

## Runtime and security boundaries

Stored values are not automatically available to Codex, Claude Code or another process. Configure the tool's environment separately using its supported startup mechanism or an external credential tool. An environment reference validates the environment of the process running SkillHub: an App launched from Finder may see a different environment from a terminal. Restart existing processes when changing their startup environment.

No vault CLI commands are executed by SkillHub. External password managers remain usable through their normal environment workflows. Giving an entire agent a variable does not isolate it to one skill. Processes running as the same OS user may have access to the same credential store; this is not protection against a compromised user account.

Settings bind the registry identity, source origin, installation, project/tool, exact version and digest. Upgrades require clearing previous settings and reviewing the current declaration before saving again. Modified installed files cannot redefine trusted requirements. Uninstalling retains configuration so users can deliberately clear it from the configuration list; it never transfers automatically to a new installation.

Client backups include non-secret configuration and opaque bindings under `local-configuration/`, but not OS-store secret values. OS credentials need the operating system's own backup/recovery mechanism. If metadata and system-store updates are interrupted, diagnostics report missing/unavailable values; retry or clear the retained entry. Keep the client data directory at its existing path when replacing the App. Restoring metadata on a different device or path does not copy or rebind secrets automatically.
