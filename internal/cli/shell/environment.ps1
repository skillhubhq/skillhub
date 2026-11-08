# Evaluate this value-free integration in PowerShell 7+. No profile is changed.
if ($PSVersionTable.PSVersion.Major -lt 7) { throw 'PowerShell 7 or later is required for UTF-8 environment transfer.' }
if ((Get-Variable -Name SkillHubEnvironmentActive -Scope Global -ErrorAction SilentlyContinue) -and $global:SkillHubEnvironmentActive) { throw 'Deactivate the current SkillHub environment before reinitializing.' }
$global:SkillHubEnvironmentActive = $false
$global:SkillHubEnvironmentNames = @()
function global:skillhub_env {
    param([Parameter(Position=0)][string]$Action, [Parameter(ValueFromRemainingArguments=$true)][string[]]$Targets)
    switch ($Action) {
        'use' {
            if ($global:SkillHubEnvironmentActive) { throw 'Deactivate the current environment first.' }
            if (Get-PSBreakpoint) { throw 'Remove PowerShell breakpoints before loading configuration.' }
            # Disable expression tracing before reading values. No values are emitted to the host.
            Set-PSDebug -Off
            $raw = & @EXECUTABLE@ --data @DATA@ env _read --format json @Targets
            if ($LASTEXITCODE -ne 0) { throw 'Configuration was not loaded. Resolve the error and retry.' }
            try { $entries = ConvertFrom-Json -InputObject ($raw -join "`n") -ErrorAction Stop }
            catch { $raw = $null; throw 'Invalid environment transfer; no values were displayed.' }
            $names = @()
            try {
                foreach ($entry in $entries) {
                    if ($entry.name -cnotmatch '^[A-Z][A-Z0-9_]{0,63}$') { throw 'Invalid environment transfer.' }
                    if (-not $entry.keep) {
                        if (Test-Path -LiteralPath "Env:$($entry.name)") { throw 'An environment variable already exists; resolve the conflict first.' }
                    }
                }
                foreach ($entry in $entries) {
                    if (-not $entry.keep) {
                        [Environment]::SetEnvironmentVariable($entry.name, $entry.value, 'Process')
                        $names += $entry.name
                    }
                }
                $global:SkillHubEnvironmentNames = $names
                $global:SkillHubEnvironmentActive = $true
                Write-Host 'Loaded into this terminal. Start your tool yourself; existing apps are unchanged.'
            } catch {
                foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name, $null, 'Process') }
                throw 'Could not load configuration; no values were displayed.'
            } finally { $raw = $null; $entries = $null }
        }
        'deactivate' {
            foreach ($name in $global:SkillHubEnvironmentNames) { [Environment]::SetEnvironmentVariable($name, $null, 'Process') }
            $global:SkillHubEnvironmentNames = @()
            $global:SkillHubEnvironmentActive = $false
            Write-Host 'Cleared variables added by SkillHub. Running processes retain their copies.'
        }
        'doctor' { & @EXECUTABLE@ --data @DATA@ env doctor @Targets }
        default { throw 'Usage: skillhub_env use TARGET [TARGET...] | doctor TARGET [TARGET...] | deactivate' }
    }
}
