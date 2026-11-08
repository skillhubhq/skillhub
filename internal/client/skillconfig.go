package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/skillconfig"
)

const SkillConfigFile = skillconfig.FileName

type ConfigVariable = skillconfig.Variable
type ConfigDeclaration = skillconfig.Declaration
type ConfigVariableStatus struct {
	ConfigVariable
	Mode   string `json:"mode"`
	Status string `json:"status"`
}
type SkillConfiguration struct {
	Installation Installation           `json:"installation"`
	Installed    bool                   `json:"installed"`
	Declared     bool                   `json:"declared"`
	Changed      bool                   `json:"changed"`
	Ready        bool                   `json:"ready"`
	Variables    []ConfigVariableStatus `json:"variables"`
}
type ConfigTarget struct {
	Installation Installation `json:"installation"`
	Installed    bool         `json:"installed"`
	Saved        bool         `json:"saved"`
}
type configValue struct {
	Mode  string `json:"mode"`
	Value string `json:"value,omitempty"`
}
type configState struct {
	Installation Installation           `json:"installation"`
	Binding      string                 `json:"binding"`
	Values       map[string]configValue `json:"values"`
}

func decodeConfig(raw []byte, target any, limit int) error {
	if len(raw) > limit || !utf8.Valid(raw) {
		return errors.New("invalid local configuration document")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return errors.New("invalid local configuration document")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("invalid local configuration document")
	}
	return nil
}
func ParseConfigDeclaration(raw []byte) (ConfigDeclaration, error) {
	return skillconfig.Parse(raw)
}
func configHash(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func configBinding(i Installation) string {
	return configHash([]string{i.ID, i.RegistryID, i.RegistryURL, i.SourceRef, i.Namespace, i.Name, i.Version, i.Digest, i.Tool, i.Scope, i.Project, i.Target, i.Fingerprint})
}
func (c *Client) configRef(s configState, name string) string {
	return "skill-config:" + configHash([]string{c.store.root, s.Binding, name})
}
func (c *Client) withConfig(ctx context.Context, fn func(*os.Root) error) error {
	return c.store.withTarget(ctx, "local-configuration", func() error {
		root, err := os.OpenRoot(c.store.root)
		if err != nil {
			return err
		}
		defer root.Close()
		if err = root.MkdirAll("local-configuration", 0700); err != nil {
			return err
		}
		info, err := root.Lstat("local-configuration")
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("invalid local configuration directory")
		}
		dir, err := root.OpenRoot("local-configuration")
		if err != nil {
			return err
		}
		defer dir.Close()
		return fn(dir)
	})
}
func readConfigState(dir *os.Root, target string) (configState, error) {
	s := configState{Values: map[string]configValue{}}
	name := configHash(target) + ".json"
	info, err := dir.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if !info.Mode().IsRegular() || info.Size() > 256<<10 {
		return s, errors.New("invalid saved configuration")
	}
	f, err := dir.Open(name)
	if err != nil {
		return s, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (256<<10)+1))
	if err != nil {
		return s, err
	}
	if err = decodeConfig(raw, &s, 256<<10); err != nil {
		return s, err
	}
	if s.Installation.Target != target || s.Binding != configBinding(s.Installation) || len(s.Values) > 32 {
		return s, errors.New("invalid saved configuration binding")
	}
	for key, v := range s.Values {
		if !skillconfig.ValidName(key) || (v.Mode != "value" && v.Mode != "keychain" && v.Mode != "environment") || (v.Mode != "value" && v.Value != "") || len(v.Value) > 4096 {
			return s, errors.New("invalid saved configuration value")
		}
	}
	if s.Values == nil {
		s.Values = map[string]configValue{}
	}
	return s, nil
}
func writeConfigState(dir *os.Root, s configState) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	name := ".pending-" + uuid.NewString()
	f, err := dir.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer dir.Remove(name)
	_, err = f.Write(raw)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err = errors.Join(err, syncErr, closeErr); err != nil {
		return err
	}
	if err = dir.Rename(name, configHash(s.Installation.Target)+".json"); err != nil {
		return err
	}
	return syncDirectory(dir.Name())
}
func (c *Client) configInstallation(ctx context.Context, target string) (Installation, ConfigDeclaration, bool, error) {
	i, err := c.store.checkedInstallation(ctx, target)
	if err != nil {
		return Installation{}, ConfigDeclaration{}, false, errors.New("check installation integrity before configuring this skill")
	}
	if i == nil {
		return Installation{}, ConfigDeclaration{}, false, errors.New("installation not found")
	}
	files, err := packageformat.ReadDirectory(target)
	if err != nil {
		return *i, ConfigDeclaration{}, false, errors.New("cannot read configuration declaration")
	}
	for _, f := range files {
		if f.Path == SkillConfigFile {
			d, e := ParseConfigDeclaration(f.Data)
			return *i, d, true, e
		}
	}
	return *i, ConfigDeclaration{}, false, nil
}

// 诊断只返回状态；不把普通值、密钥值或凭据库引用返回给 UI/JSON 输出。
func (c *Client) SkillConfiguration(ctx context.Context, target string) (SkillConfiguration, error) {
	target, err := canonicalTarget(target)
	if err != nil {
		return SkillConfiguration{}, err
	}
	r := SkillConfiguration{Variables: []ConfigVariableStatus{}}
	err = c.store.withTarget(ctx, target, func() error {
		return c.withConfig(ctx, func(dir *os.Root) error {
			s, err := readConfigState(dir, target)
			if err != nil {
				return err
			}
			stored, err := c.store.installation(ctx, target)
			if err != nil {
				return err
			}
			if stored == nil {
				if s.Binding == "" {
					return errors.New("installation not found")
				}
				r.Installation = s.Installation
				return nil
			}
			i, d, declared, err := c.configInstallation(ctx, target)
			if err != nil {
				return err
			}
			r.Installation = i
			r.Installed = true
			r.Declared = declared
			r.Ready = true
			r.Changed = s.Binding != "" && s.Binding != configBinding(i)
			for _, v := range d.Variables {
				value, ok := s.Values[v.Name]
				status := ConfigVariableStatus{ConfigVariable: v, Mode: value.Mode, Status: "missing"}
				if r.Changed {
					status.Status = "review-required"
				} else if ok {
					switch value.Mode {
					case "environment":
						if env, found := os.LookupEnv(v.Name); found && env != "" {
							status.Status = "available"
						}
					case "value":
						if !v.Secret && value.Value != "" {
							status.Status = "available"
						}
					case "keychain":
						secret, e := c.credentials.Get(c.configRef(s, v.Name))
						if e == nil && secret != "" {
							status.Status = "available"
						} else if e != nil && !errors.Is(e, ErrCredentialMissing) {
							status.Status = "store-unavailable"
						}
					}
				}
				if (v.Required && status.Status != "available") || status.Status == "store-unavailable" || r.Changed {
					r.Ready = false
				}
				r.Variables = append(r.Variables, status)
			}
			if r.Changed {
				r.Ready = false
			}
			return nil
		})
	})
	r.Installation.SourceRef = ""
	return r, err
}

// 显式设置绑定本次安装版本；版本更新后先重新授权。
func (c *Client) SetSkillConfiguration(ctx context.Context, target, name, mode, value string) error {
	target, err := canonicalTarget(target)
	if err != nil {
		return err
	}
	if !skillconfig.ValidName(name) || len(value) > 4096 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return errors.New("invalid configuration input")
	}
	return c.store.withTarget(ctx, target, func() error {
		return c.withConfig(ctx, func(dir *os.Root) error {
			i, d, _, err := c.configInstallation(ctx, target)
			if err != nil {
				return err
			}
			var variable *ConfigVariable
			for _, v := range d.Variables {
				if v.Name == name {
					copy := v
					variable = &copy
				}
			}
			if variable == nil {
				return errors.New("variable is not declared by this skill")
			}
			if (mode != "value" && mode != "keychain" && mode != "environment") || (variable.Secret && mode == "value") || (mode == "environment" && value != "") || (mode != "environment" && value == "") {
				return errors.New("choose a valid storage mode; secrets require the system store or an environment reference")
			}
			s, err := readConfigState(dir, target)
			if err != nil {
				return err
			}
			if s.Binding != "" && s.Binding != configBinding(i) {
				return errors.New("skill changed; review and renew authorization for its current requirements")
			}
			s.Installation = i
			s.Binding = configBinding(i)
			old := s.Values[name]
			if old.Mode == "keychain" && mode != "keychain" {
				if err = c.credentials.Delete(c.configRef(s, name)); err != nil {
					return ErrCredentialStore
				}
			}
			entry := configValue{Mode: mode}
			if mode == "value" {
				entry.Value = value
			}
			s.Values[name] = entry
			// 先持久化无密钥的引用，再写系统凭据库；崩溃后仍能诊断、重试和清理。
			if err = writeConfigState(dir, s); err != nil {
				return errors.New("could not save local configuration")
			}
			if mode == "keychain" {
				if err = c.credentials.Set(c.configRef(s, name), value); err != nil {
					return ErrCredentialStore
				}
			}
			return nil
		})
	})
}
func (c *Client) ClearSkillConfiguration(ctx context.Context, target string) error {
	return c.removeSkillConfiguration(ctx, target, "")
}
func (c *Client) UnsetSkillConfiguration(ctx context.Context, target, name string) error {
	if !skillconfig.ValidName(name) {
		return errors.New("invalid configuration variable name")
	}
	return c.removeSkillConfiguration(ctx, target, name)
}
func (c *Client) removeSkillConfiguration(ctx context.Context, target, key string) error {
	target, err := canonicalTarget(target)
	if err != nil {
		return err
	}
	return c.store.withTarget(ctx, target, func() error {
		return c.withConfig(ctx, func(dir *os.Root) error {
			s, err := readConfigState(dir, target)
			if err != nil || s.Binding == "" {
				return err
			}
			for name, v := range s.Values {
				if key != "" && name != key {
					continue
				}
				if v.Mode == "keychain" {
					if err = c.credentials.Delete(c.configRef(s, name)); err != nil {
						return ErrCredentialStore
					}
				}
				delete(s.Values, name)
			}
			if len(s.Values) > 0 {
				return writeConfigState(dir, s)
			}
			if err = dir.Remove(configHash(target) + ".json"); err != nil {
				return err
			}
			return syncDirectory(dir.Name())
		})
	})
}
func (c *Client) ConfigurationTargets(ctx context.Context) ([]ConfigTarget, error) {
	items, err := c.store.ListInstallations(ctx)
	if err != nil {
		return nil, err
	}
	byTarget := map[string]ConfigTarget{}
	for _, i := range items {
		byTarget[i.Target] = ConfigTarget{Installation: i, Installed: true}
	}
	err = c.withConfig(ctx, func(dir *os.Root) error {
		f, err := dir.Open(".")
		if err != nil {
			return err
		}
		defer f.Close()
		entries, err := f.ReadDir(2049)
		if err != nil && err != io.EOF {
			return err
		}
		if len(entries) > 2048 {
			return errors.New("too many saved configurations")
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".json") || !validCacheKey(strings.TrimSuffix(entry.Name(), ".json")) {
				continue
			}
			if !entry.Type().IsRegular() {
				return errors.New("invalid saved configuration file")
			}
			file, err := dir.Open(entry.Name())
			if err != nil {
				return err
			}
			raw, e := io.ReadAll(io.LimitReader(file, (256<<10)+1))
			file.Close()
			if e != nil {
				return e
			}
			var s configState
			if e = decodeConfig(raw, &s, 256<<10); e != nil {
				return e
			}
			if configHash(s.Installation.Target)+".json" != entry.Name() {
				return errors.New("invalid saved configuration path")
			}
			if _, e = readConfigState(dir, s.Installation.Target); e != nil {
				return e
			}
			t := byTarget[s.Installation.Target]
			if !t.Installed {
				t.Installation = s.Installation
			}
			t.Saved = true
			byTarget[s.Installation.Target] = t
		}
		return nil
	})
	result := []ConfigTarget{}
	for _, i := range byTarget {
		i.Installation.SourceRef = ""
		result = append(result, i)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Installation.Target < result[j].Installation.Target })
	return result, err
}

// 用户确认当前包摘要后重新授权；来源、账号或安装目标不同必须独立配置。
func (c *Client) RenewSkillConfiguration(ctx context.Context, target, expectedDigest string) error {
	target, err := canonicalTarget(target)
	if err != nil {
		return err
	}
	return c.store.withTarget(ctx, target, func() error {
		return c.withConfig(ctx, func(dir *os.Root) error {
			i, d, _, err := c.configInstallation(ctx, target)
			if err != nil {
				return err
			}
			if i.Digest != expectedDigest {
				return errors.New("installed skill changed; review its requirements again")
			}
			old, err := readConfigState(dir, target)
			if err != nil {
				return err
			}
			a, b := old.Installation, i
			if a.RegistryID != b.RegistryID || a.RegistryURL != b.RegistryURL || a.SourceRef != b.SourceRef || a.Namespace != b.Namespace || a.Name != b.Name || a.Tool != b.Tool || a.Scope != b.Scope || a.Project != b.Project || a.Target != b.Target {
				return errors.New("configuration belongs to a different source, account or installation")
			}
			if old.Binding == configBinding(i) {
				return nil
			}
			next := configState{Installation: i, Binding: configBinding(i), Values: map[string]configValue{}}
			var created []string
			committed := false
			defer func() {
				if !committed {
					for _, ref := range created {
						_ = c.credentials.Delete(ref)
					}
				}
			}()
			for _, v := range d.Variables {
				entry, ok := old.Values[v.Name]
				if !ok || (v.Secret && entry.Mode == "value") {
					continue
				}
				if entry.Mode == "keychain" {
					secret, e := c.credentials.Get(c.configRef(old, v.Name))
					if errors.Is(e, ErrCredentialMissing) {
						continue
					}
					if e != nil {
						return ErrCredentialStore
					}
					ref := c.configRef(next, v.Name)
					if e = c.credentials.Set(ref, secret); e != nil {
						return ErrCredentialStore
					}
					created = append(created, ref)
				}
				next.Values[v.Name] = entry
			}
			if err = writeConfigState(dir, next); err != nil {
				return err
			}
			committed = true
			for name, v := range old.Values {
				if v.Mode == "keychain" {
					_ = c.credentials.Delete(c.configRef(old, name))
				}
			}
			return nil
		})
	})
}
