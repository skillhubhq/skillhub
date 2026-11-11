package client

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/skillhubhq/skillhub/internal/packageformat"
	"go.yaml.in/yaml/v3"
)

// 本地清单独立于安装记录：发现文件不授予覆盖权限，也不推断 Hub 身份。
type LocalLocation struct {
	Path    string `json:"path"`
	Tool    string `json:"tool"`
	Scope   string `json:"scope"`
	Project string `json:"project"`
	Linked  bool   `json:"linked"`
}
type LocalSkill struct {
	ID           string          `json:"id"`
	Target       string          `json:"target"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Locations    []LocalLocation `json:"locations"`
	Installation *Installation   `json:"installation,omitempty"`
	Association  *Installation   `json:"association,omitempty"`
	Manager      string          `json:"manager"`
	State        string          `json:"state"`
	Problem      string          `json:"problem"`
	Hidden       bool            `json:"hidden"`
	Fingerprint  string          `json:"fingerprint"`
}
type LocalInventory struct {
	Skills   []LocalSkill `json:"skills"`
	Roots    []string     `json:"roots"`
	Warnings []string     `json:"warnings"`
}
type localPreference struct {
	Hidden      bool          `json:"hidden"`
	Association *Installation `json:"association,omitempty"`
}

func (s *Store) localPreference(ctx context.Context, target string) (localPreference, error) {
	var p localPreference
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT record FROM local_preferences WHERE target=?", target).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal([]byte(raw), &p)
	return p, err
}
func (c *Client) LocalRoots(ctx context.Context) ([]string, error) {
	rows, err := c.store.db.QueryContext(ctx, "SELECT root FROM local_roots ORDER BY root")
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
func (c *Client) SetLocalRoot(ctx context.Context, root string, add bool) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if add {
		abs, err = filepath.EvalSymlinks(abs)
		if err != nil {
			return err
		}
		info, e := os.Stat(abs)
		if e != nil {
			return e
		}
		if !info.IsDir() || filepath.Dir(abs) == abs {
			return errors.New("choose a skill folder or a folder containing skills")
		}
		var count int
		if err = c.store.db.QueryRowContext(ctx, "SELECT count(*) FROM local_roots").Scan(&count); err != nil {
			return err
		}
		if count >= 64 {
			return errors.New("too many local folders")
		}
		_, err = c.store.db.ExecContext(ctx, "INSERT OR IGNORE INTO local_roots(root) VALUES(?)", abs)
	} else {
		_, err = c.store.db.ExecContext(ctx, "DELETE FROM local_roots WHERE root=?", filepath.Clean(abs))
	}
	return err
}
func localID(target string) string { return "local-" + configHash(target) }
func boundedSkillHeader(path string) (packageformat.Metadata, error) {
	var m packageformat.Metadata
	root, err := os.OpenRoot(path)
	if err != nil {
		return m, err
	}
	defer root.Close()
	info, err := root.Lstat("SKILL.md")
	if err != nil {
		return m, err
	}
	if !info.Mode().IsRegular() || info.Size() > 256<<10 {
		return m, errors.New("SKILL.md is not a bounded regular text file")
	}
	f, err := root.Open("SKILL.md")
	if err != nil {
		return m, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (256<<10)+1))
	if err != nil {
		return m, err
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if len(raw) > 256<<10 || !utf8.Valid(raw) || !strings.HasPrefix(text, "---\n") {
		return m, errors.New("invalid SKILL.md frontmatter")
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return m, errors.New("invalid SKILL.md frontmatter")
	}
	if err = yaml.Unmarshal([]byte(text[4:4+end]), &m); err != nil {
		return m, errors.New("invalid SKILL.md frontmatter")
	}
	if m.Name == "" {
		m.Name = filepath.Base(path)
	}
	if !packageformat.ValidName(m.Name) {
		return m, errors.New("invalid skill name")
	}
	if m.Description == "" {
		return m, errors.New("skill description is missing")
	}
	return m, nil
}
func localManager(path string) string {
	for _, v := range []struct{ path, name string }{{".git", "git"}, {".clawhub/origin.json", "clawhub"}, {".claude-plugin/plugin.json", "plugin"}} {
		if _, e := os.Lstat(filepath.Join(path, v.path)); e == nil {
			return v.name
		}
	}
	// 锁文件仅用于标识更新归属，不采信其中的地址或凭据。
	parent := filepath.Dir(filepath.Dir(path))
	for _, name := range []string{".skill-lock.json", "skills-lock.json"} {
		f, e := os.Open(filepath.Join(parent, name))
		if e != nil {
			continue
		}
		raw, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
		f.Close()
		if e != nil || len(raw) > 1<<20 {
			continue
		}
		var lock struct {
			Skills map[string]json.RawMessage `json:"skills"`
		}
		if json.Unmarshal(raw, &lock) == nil && lock.Skills[filepath.Base(path)] != nil {
			return "skills-cli"
		}
	}
	if strings.Contains(filepath.ToSlash(path), "/plugins/cache/") {
		return "plugin"
	}
	return "local"
}
func (c *Client) LocalInventory(ctx context.Context) (LocalInventory, error) {
	out := LocalInventory{Skills: []LocalSkill{}, Warnings: []string{}}
	roots, err := c.LocalRoots(ctx)
	if err != nil {
		return out, err
	}
	out.Roots = roots
	installed, err := c.store.ListInstallations(ctx)
	if err != nil {
		return out, err
	}
	byPath := map[string]*LocalSkill{}
	add := func(loc LocalLocation, force bool) {
		if ctx.Err() != nil {
			return
		}
		if len(byPath) >= 4096 {
			return
		}
		path, e := filepath.Abs(loc.Path)
		if e != nil {
			return
		}
		loc.Path = filepath.Clean(path)
		info, e := os.Lstat(loc.Path)
		if e != nil && !force {
			return
		}
		resolved := loc.Path
		if e == nil {
			loc.Linked = info.Mode()&os.ModeSymlink != 0
			if real, re := filepath.EvalSymlinks(loc.Path); re == nil {
				resolved = real
			} else {
				if !force && !loc.Linked {
					return
				}
			}
		}
		if _, e := os.Stat(filepath.Join(resolved, "SKILL.md")); e != nil && !force && !loc.Linked {
			return
		}
		item := byPath[resolved]
		if item == nil {
			pref, pe := c.store.localPreference(ctx, resolved)
			if pe != nil {
				out.Warnings = append(out.Warnings, pe.Error())
				return
			}
			item = &LocalSkill{ID: localID(resolved), Target: resolved, Name: filepath.Base(resolved), Hidden: pref.Hidden, Association: pref.Association, Manager: localManager(resolved), State: "local", Locations: []LocalLocation{}}
			meta, me := boundedSkillHeader(resolved)
			if me != nil {
				item.State = "unavailable"
				item.Problem = me.Error()
			} else {
				item.Name = meta.Name
				item.Description = meta.Description
			}
			byPath[resolved] = item
		}
		for _, existing := range item.Locations {
			if existing == loc {
				return
			}
		}
		item.Locations = append(item.Locations, loc)
	}
	scan := func(root string, loc LocalLocation) {
		if _, e := os.Stat(filepath.Join(root, "SKILL.md")); e == nil {
			loc.Path = root
			add(loc, false)
			return
		}
		f, e := os.Open(root)
		if errors.Is(e, os.ErrNotExist) {
			return
		}
		if e != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("Cannot scan %s: %v", root, e))
			return
		}
		entries, e := f.ReadDir(4097)
		f.Close()
		if e != nil && e != io.EOF {
			out.Warnings = append(out.Warnings, fmt.Sprintf("Cannot scan %s: %v", root, e))
			return
		}
		if len(entries) > 4096 {
			out.Warnings = append(out.Warnings, "Folder exceeds the 4096 entry scan limit: "+root)
			entries = entries[:4096]
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			loc.Path = filepath.Join(root, entry.Name())
			add(loc, false)
		}
	}
	home := c.home
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return out, err
		}
	}
	for _, tool := range Tools() {
		scan(filepath.Join(home, tool.GlobalDirectory), LocalLocation{Tool: tool.ID, Scope: "global"})
	}
	scan(filepath.Join(home, ".codex", "skills"), LocalLocation{Tool: "codex", Scope: "global"})
	projects, err := c.ListProjects(ctx)
	if err != nil {
		return out, err
	}
	for _, project := range projects {
		for _, tool := range Tools() {
			scan(filepath.Join(project, tool.ProjectDirectory), LocalLocation{Tool: tool.ID, Scope: "project", Project: project})
		}
	}
	for _, root := range roots {
		scan(root, LocalLocation{Scope: "folder"})
	}
	for _, record := range installed {
		add(LocalLocation{Path: record.Target, Tool: record.Tool, Scope: record.Scope, Project: record.Project}, true)
		target := record.Target
		if real, e := filepath.EvalSymlinks(target); e == nil {
			target = real
		}
		if item := byPath[target]; item != nil {
			r := record
			item.Installation = &r
			item.Association = &r
			item.Manager = "skillhub"
		}
	}
	for _, item := range byPath {
		if item.State != "unavailable" && (item.Association != nil || item.Installation != nil) {
			fp, _, e := fingerprintAt(item.Target)
			if e != nil {
				item.State = "unavailable"
				item.Problem = e.Error()
			} else {
				item.Fingerprint = fp
				item.State = "modified"
				if fp == item.Association.Fingerprint {
					item.State = "matched"
				}
			}
		}
		out.Skills = append(out.Skills, *item)
	}
	sort.Slice(out.Skills, func(i, j int) bool {
		if out.Skills[i].Name == out.Skills[j].Name {
			return out.Skills[i].Target < out.Skills[j].Target
		}
		return out.Skills[i].Name < out.Skills[j].Name
	})
	return out, ctx.Err()
}

// 按用户选定的路径读取；网络来源不参与本地内容解析。
func (c *Client) LocalSkill(ctx context.Context, target string) (LocalSkill, error) {
	abs, err := filepath.Abs(target)
	if err != nil {
		return LocalSkill{}, err
	}
	if real, e := filepath.EvalSymlinks(abs); e == nil {
		abs = real
	}
	inv, err := c.LocalInventory(ctx)
	if err != nil {
		return LocalSkill{}, err
	}
	for _, i := range inv.Skills {
		if i.Target == abs {
			return i, nil
		}
	}
	meta, err := boundedSkillHeader(abs)
	if err != nil {
		return LocalSkill{}, err
	}
	pref, err := c.store.localPreference(ctx, abs)
	if err != nil {
		return LocalSkill{}, err
	}
	record, err := c.store.installation(ctx, abs)
	if err != nil {
		return LocalSkill{}, err
	}
	return LocalSkill{ID: localID(abs), Target: abs, Name: meta.Name, Description: meta.Description, Hidden: pref.Hidden, Association: pref.Association, Installation: record, Manager: localManager(abs), State: "local", Locations: []LocalLocation{{Path: abs, Scope: "folder"}}}, nil
}
func (c *Client) SetLocalHidden(ctx context.Context, target string, hidden bool) error {
	item, err := c.LocalSkill(ctx, target)
	if err != nil {
		return err
	}
	return c.store.withTarget(ctx, item.Target, func() error {
		p, e := c.store.localPreference(ctx, item.Target)
		if e != nil {
			return e
		}
		p.Hidden = hidden
		raw, _ := json.Marshal(p)
		_, e = c.store.db.ExecContext(ctx, "INSERT INTO local_preferences(target,record) VALUES(?,?) ON CONFLICT(target) DO UPDATE SET record=excluded.record", item.Target, string(raw))
		return e
	})
}

type LocalDocument struct {
	Target      string         `json:"target"`
	Fingerprint string         `json:"fingerprint"`
	Files       []PreviewFile  `json:"files"`
	File        PreviewContent `json:"file"`
}

func (c *Client) ReadLocalSkill(ctx context.Context, target, path string) (LocalDocument, error) {
	item, err := c.LocalSkill(ctx, target)
	if err != nil {
		return LocalDocument{}, err
	}
	files, err := packageformat.ReadDirectory(item.Target)
	if err != nil {
		return LocalDocument{}, err
	}
	out := LocalDocument{Target: item.Target, Fingerprint: packageformat.Fingerprint(files), Files: []PreviewFile{}}
	found := false
	for _, f := range files {
		out.Files = append(out.Files, PreviewFile{Path: f.Path, Size: len(f.Data), Executable: f.Executable})
		if f.Path == path {
			found = true
			out.File = PreviewContent{Path: path, Text: utf8.Valid(f.Data), Content: string(f.Data)}
			if !out.File.Text {
				out.File.Content = ""
			}
		}
	}
	if !found {
		return out, errors.New("skill file not found")
	}
	return out, nil
}

func (c *Client) localConfigurationInstallation(ctx context.Context, target string) (*Installation, error) {
	record, err := c.store.installation(ctx, target)
	if err != nil {
		return nil, err
	}
	if record != nil {
		return record, nil
	}
	meta, err := boundedSkillHeader(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pref, err := c.store.localPreference(ctx, target)
	if err != nil {
		return nil, err
	}
	fp, _, e := fingerprintAt(target)
	if e != nil {
		return nil, e
	}
	i := Installation{ID: localID(target), Name: meta.Name, Target: target, Scope: "folder", Digest: fp, Fingerprint: fp}
	if pref.Association != nil {
		i = *pref.Association
		i.ID = localID(target)
		i.Target = target
		i.Fingerprint = fp
		i.Digest = fp
		i.Version = ""
	}
	return &i, nil
}

func (c *Client) ExistingLocalTarget(tool, scope, project, name string) (string, error) {
	target, _, err := ResolveTarget(tool, scope, project, c.home, name)
	if err != nil {
		return "", err
	}
	if _, err = os.Lstat(target); errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return target, err
}
