# Local configuration and secrets

The CLI and App share a device-local configuration store. Hubs never receive saved values, credential-store references or environment values. This feature manages configuration requirements and explicit terminal handoff. It does not run agents, inject values into existing processes, generate `.env` files or provide a security sandbox.

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
| Environment reference | Checks the variable with the declared name in the current process; never persists its value in SkillHub |
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

Stored values are not automatically available to Codex, Claude Code or another process. Use the explicit terminal integration below, or configure the tool through its supported startup mechanism. An environment reference validates the environment of the process running SkillHub: an App launched from Finder may see a different environment from a terminal. Restart existing processes when changing their startup environment.

No vault CLI commands are executed by SkillHub. External password managers remain usable through their normal environment workflows. Giving an entire agent a variable does not isolate it to one skill. Processes running as the same OS user may have access to the same credential store; this is not protection against a compromised user account.

Settings bind the registry identity, source origin, installation, project/tool, exact version and digest. Upgrades require review and explicit authorization for the new digest; matching saved settings are retained. Modified installed files cannot redefine trusted requirements. Uninstalling retains configuration so users can deliberately clear it from the configuration list; it never transfers automatically to a new installation.

Client backups include non-secret configuration and opaque bindings under `local-configuration/`, but not OS-store secret values. OS credentials need the operating system's own backup/recovery mechanism. If metadata and system-store updates are interrupted, diagnostics report missing/unavailable values; retry or clear the retained entry. Keep the client data directory at its existing path when replacing the App. Restoring metadata on a different device or path does not copy or rebind secrets automatically.

## Use saved values in your terminal

Install the SkillHub CLI on the same device and use the same client data directory as the App. In **Configure → Use in terminal**, select Bash, Zsh or PowerShell 7+ and copy the value-free setup commands. The App does not receive saved values through this guide and does not launch a terminal or agent.

For Zsh (use `bash` for Bash):

```sh
source <(skillhub env init zsh)
skillhub_env use /path/to/installed/skill
skillhub_env doctor /path/to/installed/skill
# Start your chosen tool yourself in this terminal.
# After finishing:
skillhub_env deactivate
```

For PowerShell 7+:

```powershell
skillhub env init powershell | Out-String | Invoke-Expression
skillhub_env use 'C:\path\to\installed\skill'
skillhub_env doctor 'C:\path\to\installed\skill'
skillhub_env deactivate
```

`init` prints only the integration code, executable path and client directory; it contains no saved values and does not edit shell profiles. PowerShell evaluates this trusted initialization code, not credential output. `use` transfers configuration through a local pipe, treats values as data and exports them only in the current terminal. The internal transport refuses terminal and regular-file output. This is an accidental-disclosure safeguard, not an access boundary against another process running as your OS user. Use your own trusted, non-recorded terminal. Bash/Zsh reject tracing and DEBUG traps; PowerShell rejects breakpoints and disables expression tracing before reading values. Host-level tracing, debugging and memory inspection remain outside this protection.

Select one or more exact installation paths explicitly. No project-wide or cross-Hub aggregation happens automatically. Missing required values, changed installations, unavailable credential stores, unsupported values and duplicate variable owners stop the transfer before environment changes. Runtime-control and agent-authentication variables such as `PATH`, `BASH_ENV`, `NODE_OPTIONS`, `OPENAI_API_KEY` and `ANTHROPIC_API_KEY` cannot be loaded through skill declarations. Configure agent authentication through the tool itself.

An existing environment variable with a different value causes a conflict. Equal inherited values remain untouched; shell-local variables that would be overwritten also cause a conflict. `deactivate` removes only variables added by the integration. It does not erase stored configuration, revoke service credentials or remove copies held by already-running processes. Deactivate before selecting another set of skills or reinitializing the integration. Closing the terminal also ends its environment.

`config doctor` checks saved configuration. `env doctor` compares the **current process** environment with those saved values and prints only variable names and `present`, `missing` or `different`. A successful check is not proof of a service login or of another process receiving the variables. To verify a tool's shell environment, run the same value-free `skillhub env doctor TARGET` command there, subject to that tool's permissions. Do not print variables into a conversation.

Codex applies its [shell environment policy](https://learn.chatgpt.com/docs/config-file/config-advanced) to spawned commands. Claude Code also applies its own configuration and permissions. Review those rules rather than disabling filtering globally. An already-open desktop App cannot receive changes to a separate terminal; this integration does not inject into it. Environment variables are available to code in the receiving process, not isolated to a single skill. SkillHub does not automatically send values to a Hub or model; downstream tools can still expose values if instructed to print or transmit them.

## Renew authorization

After updating an installed skill, review its requirements and authorize the current digest in App, or run `skillhub config renew TARGET DIGEST`. Matching values are retained, including system-store secrets. Removed variables are discarded; newly required variables need configuration. A different source, account or installation cannot inherit authorization.
