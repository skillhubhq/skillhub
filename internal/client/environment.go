package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"
)

// 环境交接只面向用户主动选择的本地安装，不执行包命令或访问 Hub。
type EnvironmentStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}
type environmentValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Keep  bool   `json:"keep"`
}

// 运行时控制变量不能从技能声明进入 Shell 或改变 Agent 的登录身份。
func safeEnvironmentName(name string) bool {
	for _, prefix := range []string{"BASH", "ZSH", "SHELLOPT", "LD_", "DYLD_", "PYTHON", "NODE_", "NPM_", "RUBY", "PERL", "GIT_", "SKILLHUB", "CODEX", "CLAUDE", "OPENAI", "ANTHROPIC", "SSL_", "CURL_", "LC_", "DOTNET_", "COMPLUS_", "GLIBC_", "GCONV_", "GTK_", "QT_", "SSH_", "SUDO_", "LESS"} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	switch name {
	case "ZDOTDIR", "LOCPATH", "NLSPATH", "GIO_EXTRA_MODULES", "LANG", "LANGUAGE", "EDITOR", "VISUAL", "PAGER", "BROWSER", "PROMPT", "RPROMPT", "PS1", "PS2", "PS3", "HISTFILE", "HISTCONTROL", "RANDOM", "SECONDS", "LINENO", "OPTIND", "PATH", "HOME", "USER", "LOGNAME", "SHELL", "ENV", "IFS", "CDPATH", "FPATH", "PS4", "PROMPT_COMMAND", "TMPDIR", "TMP", "TEMP", "PWD", "OLDPWD", "SHLVL", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "JAVA_TOOL_OPTIONS", "JDK_JAVA_OPTIONS", "CLASSPATH":
		return false
	}
	return true
}

func (c *Client) resolveEnvironment(ctx context.Context, targets []string) ([]environmentValue, error) {
	if len(targets) == 0 || len(targets) > 32 {
		return nil, errors.New("select between 1 and 32 installed skills")
	}
	values := map[string]environmentValue{}
	seen := map[string]bool{}
	for _, target := range targets {
		target, err := canonicalTarget(target)
		if err != nil {
			return nil, err
		}
		if seen[target] {
			continue
		}
		seen[target] = true
		err = c.store.withTarget(ctx, target, func() error {
			return c.withConfig(ctx, func(dir *os.Root) error {
				i, d, declared, e := c.configInstallation(ctx, target)
				if e != nil {
					return e
				}
				if !declared {
					return errors.New("selected skill does not declare configuration requirements")
				}
				s, e := readConfigState(dir, target)
				if e != nil {
					return e
				}
				if s.Binding != "" && s.Binding != configBinding(i) {
					return errors.New("skill version or origin changed; review configuration first")
				}
				for _, v := range d.Variables {
					if !safeEnvironmentName(v.Name) {
						return fmt.Errorf("%s is reserved for runtime configuration", v.Name)
					}
					saved, ok := s.Values[v.Name]
					value := ""
					if ok {
						switch saved.Mode {
						case "environment":
							value = os.Getenv(v.Name)
						case "value":
							if !v.Secret {
								value = saved.Value
							}
						case "keychain":
							value, e = c.credentials.Get(c.configRef(s, v.Name))
							if e != nil && !errors.Is(e, ErrCredentialMissing) {
								return errors.New("unlock the system credential store before loading configuration")
							}
						}
					}
					if value == "" {
						if v.Required {
							return fmt.Errorf("%s is not configured", v.Name)
						}
						continue
					}
					if len(value) > 4096 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
						return fmt.Errorf("%s cannot be represented in a process environment", v.Name)
					}
					if _, exists := values[v.Name]; exists {
						return fmt.Errorf("%s is declared by multiple selected installations; select one owner", v.Name)
					}
					old, exists := os.LookupEnv(v.Name)
					values[v.Name] = environmentValue{Name: v.Name, Value: value, Keep: exists && old == value}
				}
				return nil
			})
		})
		if err != nil {
			return nil, err
		}
	}
	result := make([]environmentValue, 0, len(values))
	for _, v := range values {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	if len(result) == 0 {
		return nil, errors.New("no configured variables to load")
	}
	return result, nil
}

// 诊断仅比较当前进程与本地保存值，不返回值，也不声称下游服务已认证。
func (c *Client) CheckEnvironment(ctx context.Context, targets []string) ([]EnvironmentStatus, error) {
	values, err := c.resolveEnvironment(ctx, targets)
	if err != nil {
		return nil, err
	}
	result := make([]EnvironmentStatus, 0, len(values))
	for _, v := range values {
		status := "missing"
		if _, ok := os.LookupEnv(v.Name); ok {
			status = "different"
			if v.Keep {
				status = "present"
			}
		}
		result = append(result, EnvironmentStatus{Name: v.Name, Status: status})
	}
	return result, nil
}

// WriteEnvironment 是 CLI 到本地 Shell 管道的敏感传输接口，不能绑定给 Web/App 或诊断输出。
func (c *Client) WriteEnvironment(ctx context.Context, targets []string, format string, out io.Writer) error {
	if format != "nul" && format != "json" {
		return errors.New("unsupported environment transport")
	}
	values, err := c.resolveEnvironment(ctx, targets)
	if err != nil {
		return err
	}
	for _, v := range values {
		if _, ok := os.LookupEnv(v.Name); ok && !v.Keep {
			return fmt.Errorf("%s already has a different value; use a clean terminal or resolve the conflict", v.Name)
		}
	}
	var buffer bytes.Buffer
	if format == "json" {
		err = json.NewEncoder(&buffer).Encode(values)
	} else {
		for _, v := range values {
			action := "set"
			if v.Keep {
				action = "keep"
			}
			fmt.Fprintf(&buffer, "%s\x00%s\x00%s\x00", v.Name, v.Value, action)
		}
		buffer.WriteString("__SKILLHUB_COMPLETE__\x00")
	}
	if err != nil {
		return errors.New("could not prepare local environment")
	}
	if _, err = out.Write(buffer.Bytes()); err != nil {
		return errors.New("local environment transfer failed")
	}
	return nil
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

// 使用说明只包含路径与命令，不包含配置值或凭据库引用。
func (c *Client) EnvironmentInstructions(ctx context.Context, target, shell string) (string, error) {
	target, err := canonicalTarget(target)
	if err != nil {
		return "", err
	}
	if _, _, _, err = c.configInstallation(ctx, target); err != nil {
		return "", err
	}
	prefix := "skillhub --data " + shellQuote(c.store.root)
	switch shell {
	case "bash", "zsh":
		return "source <(" + prefix + " env init " + shell + ")\nskillhub_env use " + shellQuote(target) + "\nskillhub_env doctor " + shellQuote(target), nil
	case "powershell":
		q := func(v string) string { return "'" + strings.ReplaceAll(v, "'", "''") + "'" }
		return "skillhub --data " + q(c.store.root) + " env init powershell | Out-String | Invoke-Expression\nskillhub_env use " + q(target) + "\nskillhub_env doctor " + q(target), nil
	default:
		return "", errors.New("supported shells: bash, zsh, powershell")
	}
}
