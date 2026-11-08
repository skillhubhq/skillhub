package client

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/provenancewire"
	"golang.org/x/mod/semver"
)

const ManifestFile = "skillhub.json"
const ProjectLockFile = "skillhub.lock.json"
const maxProjectEntries = 100
const maxProjectFile = 1 << 20

var ErrProjectChanged = errors.New("project manifest or lock changed; plan again")

// ProjectSkill 使用逻辑源别名表达期望；锁文件另行绑定实际 Hub 身份。
type ProjectSkill struct {
	Source    string `json:"source"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Tool      string `json:"tool"`
}
type ProjectManifest struct {
	Schema int            `json:"schema"`
	Skills []ProjectSkill `json:"skills"`
}
type LockedSkill struct {
	ProjectSkill
	RegistryID  string `json:"registry_id"`
	RegistryURL string `json:"registry_url"`
	Digest      string `json:"digest"`
	Target      string `json:"target"`
}
type ProjectLock struct {
	Schema         int           `json:"schema"`
	ManifestDigest string        `json:"manifest_digest"`
	Skills         []LockedSkill `json:"skills"`
}
type ProjectState struct {
	Root      string          `json:"root"`
	Manifest  ProjectManifest `json:"manifest"`
	Lock      *ProjectLock    `json:"lock,omitempty"`
	NeedsLock bool            `json:"needs_lock"`
}
type ProjectRestorePlan struct {
	Project string        `json:"project"`
	Plans   []InstallPlan `json:"plans"`
	// 快照只留在内核中；前端保存计划句柄，不能改写已确认的目标。
	manifestDigest string
	lockDigest     string
}
type ProjectRestoreResult struct {
	Target  string `json:"target"`
	Applied bool   `json:"applied"`
	Error   string `json:"error,omitempty"`
}

func projectRoot(path string) (string, error) {
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("project must be an existing directory")
	}
	return physicalSpelling(root)
}

// 项目锁放在项目中，协调使用不同客户端数据库的 CLI/App 进程。
func withProject(ctx context.Context, root string, fn func() error) error {
	path := filepath.Join(root, ".skillhub-project.lock")
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("project operation lock must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	lock := flock.New(path)
	ok, err := lock.TryLockContext(ctx, 25*time.Millisecond)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("cannot acquire project lock")
	}
	defer lock.Unlock()
	return fn()
}

func readProjectJSON(path string, value any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxProjectFile {
		return errors.New("project metadata must be a regular file of at most 1 MiB")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxProjectFile+1))
	if err != nil {
		return err
	}
	if len(data) > maxProjectFile {
		return errors.New("project metadata exceeds 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(value); err != nil {
		return errors.New("invalid project metadata JSON")
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return errors.New("project metadata must contain one JSON document")
	}
	return nil
}

func writeProjectJSON(root, name string, value any) error {
	path := filepath.Join(root, name)
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("project metadata must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if len(data)+1 > maxProjectFile {
		return errors.New("project metadata exceeds 1 MiB")
	}
	f, err := os.CreateTemp(root, ".skillhub-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(append(data, '\n'))
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	return syncDirectory(root)
}
func metadataDigest(value any) string {
	data, _ := json.Marshal(value)
	return packageformat.Digest(data)
}

func relativeSkillTarget(skill ProjectSkill) (string, error) {
	for _, value := range []string{skill.Source, skill.Namespace, skill.Name} {
		if !packageformat.ValidName(value) {
			return "", errors.New("invalid project skill coordinate")
		}
	}
	if len(skill.Version) > 128 || !semver.IsValid("v"+skill.Version) || strings.TrimPrefix(semver.Canonical("v"+skill.Version), "v") != strings.SplitN(skill.Version, "+", 2)[0] {
		return "", errors.New("project requires exact semantic versions")
	}
	switch skill.Tool {
	case "codex":
		return ".agents/skills/" + skill.Name, nil
	case "claude-code":
		if skill.Name == "synced" || skill.Name == "anthropic-skills" {
			return "", errors.New("skill name is reserved by Claude Code")
		}
		return ".claude/skills/" + skill.Name, nil
	default:
		return "", errors.New("supported tools: codex, claude-code")
	}
}
func validateManifest(m ProjectManifest) error {
	if m.Schema != 1 || m.Skills == nil || len(m.Skills) > maxProjectEntries {
		return errors.New("unsupported project schema or invalid skill count (maximum 100)")
	}
	targets := map[string]bool{}
	for _, skill := range m.Skills {
		target, err := relativeSkillTarget(skill)
		if err != nil {
			return err
		}
		if targets[target] {
			return errors.New("multiple project skills claim the same tool target")
		}
		targets[target] = true
	}
	return nil
}
func readManifest(root string) (ProjectManifest, error) {
	var m ProjectManifest
	if err := readProjectJSON(filepath.Join(root, ManifestFile), &m); err != nil {
		return m, err
	}
	return m, validateManifest(m)
}
func validDigest(value string) bool {
	if len(value) != 71 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(value[7:])
	return err == nil && value == strings.ToLower(value)
}
func validateLock(m ProjectManifest, lock ProjectLock) error {
	if lock.Schema != 1 || lock.ManifestDigest != metadataDigest(m) || lock.Skills == nil || len(lock.Skills) != len(m.Skills) {
		return errors.New("project lock does not match manifest; run project lock")
	}
	for i, skill := range lock.Skills {
		target, err := relativeSkillTarget(skill.ProjectSkill)
		if err != nil {
			return err
		}
		address, err := canonicalURL(skill.RegistryURL)
		if err != nil || address != skill.RegistryURL || len(skill.RegistryID) < 1 || len(skill.RegistryID) > 128 || !validDigest(skill.Digest) || skill.Target != target || skill.ProjectSkill != m.Skills[i] {
			return errors.New("invalid locked skill identity or target")
		}
	}
	return nil
}

func (c *Client) ReadProject(ctx context.Context, path string) (ProjectState, error) {
	root, err := projectRoot(path)
	if err != nil {
		return ProjectState{}, err
	}
	state := ProjectState{Root: root}
	err = withProject(ctx, root, func() error {
		var err error
		state.Manifest, err = readManifest(root)
		if err != nil {
			return err
		}
		var lock ProjectLock
		err = readProjectJSON(filepath.Join(root, ProjectLockFile), &lock)
		if errors.Is(err, os.ErrNotExist) {
			state.NeedsLock = true
			return nil
		}
		if err != nil {
			return err
		}
		state.Lock = &lock
		state.NeedsLock = validateLock(state.Manifest, lock) != nil
		return nil
	})
	return state, err
}

func (c *Client) InitProject(ctx context.Context, path string) (ProjectState, error) {
	root, err := projectRoot(path)
	if err != nil {
		return ProjectState{}, err
	}
	err = withProject(ctx, root, func() error {
		if _, err := os.Lstat(filepath.Join(root, ManifestFile)); !errors.Is(err, os.ErrNotExist) {
			return errors.New("project manifest already exists or cannot be inspected")
		}
		return writeProjectJSON(root, ManifestFile, ProjectManifest{Schema: 1, Skills: []ProjectSkill{}})
	})
	if err != nil {
		return ProjectState{}, err
	}
	return c.OpenProject(ctx, root)
}

func (c *Client) SetProjectSkill(ctx context.Context, path string, skill ProjectSkill) (ProjectState, error) {
	if _, err := relativeSkillTarget(skill); err != nil {
		return ProjectState{}, err
	}
	root, err := projectRoot(path)
	if err != nil {
		return ProjectState{}, err
	}
	err = withProject(ctx, root, func() error {
		m, err := readManifest(root)
		if err != nil {
			return err
		}
		replaced := false
		for i, old := range m.Skills {
			if old.Tool == skill.Tool && old.Name == skill.Name {
				m.Skills[i] = skill
				replaced = true
				break
			}
		}
		if !replaced {
			m.Skills = append(m.Skills, skill)
		}
		sort.Slice(m.Skills, func(i, j int) bool {
			return m.Skills[i].Tool+"/"+m.Skills[i].Name < m.Skills[j].Tool+"/"+m.Skills[j].Name
		})
		if err = validateManifest(m); err != nil {
			return err
		}
		return writeProjectJSON(root, ManifestFile, m)
	})
	if err != nil {
		return ProjectState{}, err
	}
	return c.OpenProject(ctx, root)
}
func (c *Client) RemoveProjectSkill(ctx context.Context, path, tool, name string) (ProjectState, error) {
	root, err := projectRoot(path)
	if err != nil {
		return ProjectState{}, err
	}
	err = withProject(ctx, root, func() error {
		m, err := readManifest(root)
		if err != nil {
			return err
		}
		skills := make([]ProjectSkill, 0, len(m.Skills))
		for _, s := range m.Skills {
			if s.Tool != tool || s.Name != name {
				skills = append(skills, s)
			}
		}
		if len(skills) == len(m.Skills) {
			return errors.New("project skill not found")
		}
		m.Skills = skills
		return writeProjectJSON(root, ManifestFile, m)
	})
	if err != nil {
		return ProjectState{}, err
	}
	return c.OpenProject(ctx, root)
}

// LockProject 从已配置的源解析精确版本，不信任项目文件提供的新地址，也不隐式登录。
func (c *Client) LockProject(ctx context.Context, path string) (ProjectLock, error) {
	root, err := projectRoot(path)
	if err != nil {
		return ProjectLock{}, err
	}
	lock := ProjectLock{Schema: 1, Skills: []LockedSkill{}}
	err = withProject(ctx, root, func() error {
		m, err := readManifest(root)
		if err != nil {
			return err
		}
		lock.ManifestDigest = metadataDigest(m)
		for _, skill := range m.Skills {
			remote, err := c.Remote(ctx, skill.Source)
			if err != nil {
				return fmt.Errorf("%s: %w", skill.Source, err)
			}
			archive, digest, err := c.downloadExact(ctx, remote, Coordinate{skill.Source, skill.Namespace, skill.Name, skill.Version})
			if err != nil {
				return fmt.Errorf("%s/%s: %w", skill.Source, skill.Name, err)
			}
			bundle, err := packageformat.Inspect(archive)
			if err != nil {
				return err
			}
			if bundle.Metadata.Name != skill.Name {
				return errors.New("package does not match skill name")
			}
			target, _ := relativeSkillTarget(skill)
			lock.Skills = append(lock.Skills, LockedSkill{ProjectSkill: skill, RegistryID: remote.Source.RegistryID, RegistryURL: remote.Source.URL, Digest: digest, Target: target})
		}
		// 编辑器不遵守进程锁；写锁文件前再检查期望清单。
		current, err := readManifest(root)
		if err != nil {
			return err
		}
		if metadataDigest(current) != lock.ManifestDigest {
			return ErrProjectChanged
		}
		return writeProjectJSON(root, ProjectLockFile, lock)
	})
	if err == nil {
		err = c.rememberProject(ctx, root)
	}
	return lock, err
}

// 只在用户已配置的源中精确匹配身份与地址；别名可因机器而异，锁文件不会触发任意网络连接。
func (c *Client) lockedSource(ctx context.Context, skill LockedSkill) (Source, error) {
	sources, err := c.ListSources(ctx)
	if err != nil {
		return Source{}, err
	}
	var candidates []Source
	for _, source := range sources {
		if source.RegistryID == skill.RegistryID && source.URL == skill.RegistryURL {
			if source.Alias == skill.Source {
				return source, nil
			}
			candidates = append(candidates, source)
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) > 1 {
		return Source{}, errors.New("multiple source identities match this Hub; select an explicit source alias")
	}
	return Source{}, errors.New("locked Hub is not configured; add its source and log in if required")
}
func (c *Client) lockedDownload(ctx context.Context, skill LockedSkill) ([]byte, Source, *catalogv1.Release, error) {
	source, err := c.lockedSource(ctx, skill)
	if err != nil {
		return nil, Source{}, nil, err
	}
	remote, err := c.Remote(ctx, source.Alias)
	if err != nil {
		return nil, Source{}, nil, err
	}
	if remote.Source.RegistryID != skill.RegistryID || remote.Source.URL != skill.RegistryURL {
		return nil, Source{}, nil, ErrRegistryChanged
	}
	release, archive, err := c.downloadRelease(ctx, remote, Coordinate{source.Alias, skill.Namespace, skill.Name, skill.Version})
	if err != nil {
		return nil, Source{}, nil, err
	}
	if release.Digest != skill.Digest {
		return nil, Source{}, nil, errors.New("download digest differs from project lock")
	}
	return archive, remote.Source, release, nil
}
func projectTarget(root string, skill LockedSkill) (string, error) {
	target, _, err := ResolveTarget(skill.Tool, "project", root, "", skill.Name)
	if err != nil {
		return "", err
	}
	expected := filepath.Join(root, filepath.FromSlash(skill.Target))
	// 即使适配器支持符号链接，项目恢复也不允许相对目标映射到其他位置。
	if target != expected {
		return "", errors.New("project target resolves through an alias or outside its locked path")
	}
	return target, nil
}
func (c *Client) PlanProjectRestore(ctx context.Context, path string) (*ProjectRestorePlan, error) {
	state, err := c.ReadProject(ctx, path)
	if err != nil {
		return nil, err
	}
	if state.NeedsLock || state.Lock == nil {
		return nil, errors.New("project requires a matching lock; run project lock")
	}
	plan := &ProjectRestorePlan{Project: state.Root, Plans: []InstallPlan{}, manifestDigest: metadataDigest(state.Manifest), lockDigest: metadataDigest(state.Lock)}
	targets := map[string]bool{}
	for _, skill := range state.Lock.Skills {
		target, err := projectTarget(state.Root, skill)
		if err != nil {
			return nil, err
		}
		if targets[target] {
			return nil, ErrSourceConflict
		}
		targets[target] = true
		archive, source, release, err := c.lockedDownload(ctx, skill)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", skill.Source, skill.Name, err)
		}
		desired := Installation{Source: source.Alias, SourceRef: source.CredentialRef, RegistryID: skill.RegistryID, RegistryURL: skill.RegistryURL, Namespace: skill.Namespace, Name: skill.Name, Version: skill.Version, Digest: skill.Digest, DisplayNameEN: release.DisplayNameEn, DisplayNameZH: release.DisplayNameZh, Icon: release.Icon, Provenance: provenancewire.Decode(release.Provenance), Tool: skill.Tool, Scope: "project", Project: state.Root, Target: target}
		next, err := c.store.PlanInstall(ctx, desired, archive)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", skill.Target, err)
		}
		plan.Plans = append(plan.Plans, *next)
	}
	current, err := c.ReadProject(ctx, state.Root)
	if err != nil {
		return nil, err
	}
	if metadataDigest(current.Manifest) != plan.manifestDigest || metadataDigest(current.Lock) != plan.lockDigest {
		return nil, ErrProjectChanged
	}
	if err = c.rememberProject(ctx, state.Root); err != nil {
		return nil, err
	}
	return plan, nil
}

// 批量恢复逐目标提交，权限与内容在应用时重新确认；某一目标失败不会假装整体回滚。
func (c *Client) ApplyProjectRestore(ctx context.Context, plan *ProjectRestorePlan) ([]ProjectRestoreResult, error) {
	if plan == nil {
		return nil, errors.New("missing project restore plan")
	}
	results := []ProjectRestoreResult{}
	err := withProject(ctx, plan.Project, func() error {
		m, err := readManifest(plan.Project)
		if err != nil {
			return err
		}
		var lock ProjectLock
		if err = readProjectJSON(filepath.Join(plan.Project, ProjectLockFile), &lock); err != nil {
			return err
		}
		if err = validateLock(m, lock); err != nil {
			return err
		}
		if metadataDigest(m) != plan.manifestDigest || metadataDigest(lock) != plan.lockDigest || len(plan.Plans) != len(lock.Skills) {
			return ErrProjectChanged
		}
		for i, skill := range lock.Skills {
			item := ProjectRestoreResult{Target: skill.Target}
			applyErr := func() error {
				// 手工改清单或移动项目后，不继续写余下目标。
				current, err := readManifest(plan.Project)
				if err != nil {
					return err
				}
				var currentLock ProjectLock
				if err = readProjectJSON(filepath.Join(plan.Project, ProjectLockFile), &currentLock); err != nil {
					return err
				}
				if metadataDigest(current) != plan.manifestDigest || metadataDigest(currentLock) != plan.lockDigest {
					return ErrProjectChanged
				}
				target, err := projectTarget(plan.Project, skill)
				if err != nil {
					return err
				}
				archive, source, release, err := c.lockedDownload(ctx, skill)
				if err != nil {
					return err
				}
				expected := Installation{Source: source.Alias, SourceRef: source.CredentialRef, RegistryID: skill.RegistryID, RegistryURL: skill.RegistryURL, Namespace: skill.Namespace, Name: skill.Name, Version: skill.Version, Digest: skill.Digest, DisplayNameEN: release.DisplayNameEn, DisplayNameZH: release.DisplayNameZh, Icon: release.Icon, Provenance: provenancewire.Decode(release.Provenance), Tool: skill.Tool, Scope: "project", Project: plan.Project, Target: target}
				// 用新计划核对确认过的文件状态；沿用旧计划的安装 ID 由 Apply 保证。
				fresh, err := c.store.PlanInstall(ctx, expected, archive)
				if err != nil {
					return err
				}
				old := plan.Plans[i]
				if old.After == nil || fresh.After == nil {
					return ErrStalePlan
				}
				fresh.After.ID = old.After.ID
				if !reflect.DeepEqual(*fresh, old) {
					return ErrStalePlan
				}
				return c.store.Apply(ctx, old, archive)
			}()
			if applyErr != nil {
				item.Error = applyErr.Error()
			} else {
				item.Applied = true
			}
			results = append(results, item)
		}
		return nil
	})
	return results, err
}

// OpenProject 仅记录用户显式打开的项目；列表不会扫描文件系统或向 Hub 发请求。
func (c *Client) OpenProject(ctx context.Context, path string) (ProjectState, error) {
	state, err := c.ReadProject(ctx, path)
	if err != nil {
		return state, err
	}
	return state, c.rememberProject(ctx, state.Root)
}
func (c *Client) rememberProject(ctx context.Context, root string) error {
	_, err := c.store.db.ExecContext(ctx, "INSERT INTO projects(root) VALUES(?) ON CONFLICT(root) DO NOTHING", root)
	return err
}
func (c *Client) ListProjects(ctx context.Context) ([]string, error) {
	rows, err := c.store.db.QueryContext(ctx, "SELECT root FROM projects ORDER BY root")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roots := []string{}
	for rows.Next() {
		var root string
		if err = rows.Scan(&root); err != nil {
			return nil, err
		}
		roots = append(roots, root)
	}
	return roots, rows.Err()
}

// ForgetProject 只移除本机快捷记录，保留项目文件和已安装技能。
func (c *Client) ForgetProject(ctx context.Context, path string) error {
	// 已删除的目录仍可按列表中的绝对路径移除。
	root, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if canonical, resolveErr := projectRoot(path); resolveErr == nil {
		root = canonical
	}
	_, err = c.store.db.ExecContext(ctx, "DELETE FROM projects WHERE root=?", filepath.Clean(root))
	return err
}
