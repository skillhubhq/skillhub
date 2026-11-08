# Source this value-free integration in Bash or Zsh. No startup files are changed.
if [ -n "${__skillhub_active-}" ]; then
  printf '%s\n' 'Deactivate the current SkillHub environment before reinitializing.' >&2
  return 1
fi
__skillhub_loaded_names=()
skillhub_env() {
  case "$-" in *x*|*v*) printf '%s\n' 'Disable shell tracing and verbose input before using local configuration.' >&2; return 1;; esac
  case "$(trap)" in *DEBUG*) printf '%s\n' 'Disable DEBUG traps before using local configuration.' >&2; return 1;; esac
  local __sh_action="${1-}"
  [ "$#" -gt 0 ] && shift
  case "$__sh_action" in
    use)
      if [ -n "${__skillhub_active-}" ]; then printf '%s\n' 'Deactivate the current environment first.' >&2; return 1; fi
      local __sh_name __sh_value __sh_mode __sh_complete='' __sh_record
      local -a __sh_records __sh_names
      __sh_records=(); __sh_names=()
      while IFS= read -r -d '' __sh_name; do
        if [ "$__sh_name" = '__SKILLHUB_COMPLETE__' ]; then __sh_complete=1; break; fi
        IFS= read -r -d '' __sh_value || break
        IFS= read -r -d '' __sh_mode || break
        case "$__sh_name" in ''|*[!A-Z0-9_]*) printf '%s\n' 'Invalid environment transfer.' >&2; return 1;; esac
        case "$__sh_mode" in
          keep) ;;
          set)
            if typeset -p "$__sh_name" >/dev/null 2>&1; then printf '%s\n' "$__sh_name already exists in this shell; resolve the conflict first." >&2; return 1; fi
            __sh_records+=("$__sh_name=$__sh_value")
            __sh_names+=("$__sh_name") ;;
          *) return 1;;
        esac
      done < <(@EXECUTABLE@ --data @DATA@ env _read "$@")
      if [ "$__sh_complete" != 1 ]; then printf '%s\n' 'Configuration was not loaded. Resolve the error and retry.' >&2; return 1; fi
      for __sh_record in ${__sh_records[@]+"${__sh_records[@]}"}; do
        if ! export "$__sh_record"; then
          for __sh_name in ${__sh_names[@]+"${__sh_names[@]}"}; do unset "$__sh_name"; done
          printf '%s\n' 'Could not load configuration.' >&2; return 1
        fi
      done
      __skillhub_loaded_names=(${__sh_names[@]+"${__sh_names[@]}"}); __skillhub_active=1
      printf '%s\n' 'Loaded into this terminal. Start your tool yourself; existing apps are unchanged.'
      ;;
    deactivate)
      local __sh_name
      for __sh_name in ${__skillhub_loaded_names[@]+"${__skillhub_loaded_names[@]}"}; do
        unset "$__sh_name" || { printf '%s\n' 'Could not clear configuration; close this terminal.' >&2; return 1; }
      done
      __skillhub_loaded_names=(); unset __skillhub_active
      printf '%s\n' 'Cleared variables added by SkillHub. Existing variables were not changed. Running processes retain their copies.'
      ;;
    doctor) @EXECUTABLE@ --data @DATA@ env doctor "$@" ;;
    *) printf '%s\n' 'Usage: skillhub_env use TARGET [TARGET...] | doctor TARGET [TARGET...] | deactivate' >&2; return 1;;
  esac
}
