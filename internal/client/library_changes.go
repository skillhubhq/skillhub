package client

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/provenancewire"
	"github.com/skillhubhq/skillhub/internal/skillsuite"
)

type LocalDifference struct {
	Path   string `json:"path"`
	Change string `json:"change"`
	Before string `json:"before"`
	After  string `json:"after"`
	Text   bool   `json:"text"`
}
type LocalPlan struct {
	Action           string            `json:"action"`
	Target           string            `json:"target"`
	Expected         string            `json:"expected"`
	Result           string            `json:"result"`
	Locations        []LocalLocation   `json:"locations"`
	Differences      []LocalDifference `json:"differences"`
	Before           *Installation     `json:"before,omitempty"`
	After            *Installation     `json:"after,omitempty"`
	Association      *Installation     `json:"association,omitempty"`
	BeforePreference localPreference   `json:"before_preference"`
	Link             string            `json:"link"`
	BackupID         string            `json:"backup_id"`
	// 字节仅留在客户端内核或桌面计划缓存中，不通过绑定返回。
	files []packageformat.File
	modes map[string]os.FileMode
}
type LocalBackup struct {
	Suite        bool            `json:"suite"`
	ID           string          `json:"id"`
	Target       string          `json:"target"`
	CreatedAt    string          `json:"created_at"`
	Action       string          `json:"action"`
	Fingerprint  string          `json:"fingerprint"`
	Directory    string          `json:"directory"`
	Link         string          `json:"link"`
	Installation *Installation   `json:"installation,omitempty"`
	Preference   localPreference `json:"preference"`
}
type localJournal struct {
	Plan      LocalPlan
	Directory string
	Committed bool
	Backup    LocalBackup
}

func differences(a, b []packageformat.File) []LocalDifference {
	left, right := map[string]packageformat.File{}, map[string]packageformat.File{}
	paths := map[string]bool{}
	for _, f := range a {
		left[f.Path] = f
		paths[f.Path] = true
	}
	for _, f := range b {
		right[f.Path] = f
		paths[f.Path] = true
	}
	out := []LocalDifference{}
	for path := range paths {
		l, le := left[path]
		r, re := right[path]
		if le && re && bytes.Equal(l.Data, r.Data) && l.Executable == r.Executable {
			continue
		}
		change := "modified"
		if !le {
			change = "added"
		}
		if !re {
			change = "removed"
		}
		d := LocalDifference{Path: path, Change: change, Text: utf8.Valid(l.Data) && utf8.Valid(r.Data) && len(l.Data)+len(r.Data) <= 65536}
		if d.Text {
			d.Before = string(l.Data)
			d.After = string(r.Data)
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
func localSnapshot(target string) (string, string, []packageformat.File, error) {
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return "", "", nil, nil
	}
	if err != nil {
		return "", "", nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, e := os.Readlink(target)
		return "link:" + configHash(link), link, nil, e
	}
	files, err := packageformat.ReadDirectory(target)
	if err != nil {
		return "", "", nil, err
	}
	return packageformat.Fingerprint(files), "", files, nil
}
func (c *Client) baseLocalPlan(ctx context.Context, target, action string) (LocalPlan, []packageformat.File, error) {
	item, err := c.LocalSkill(ctx, target)
	if err != nil {
		return LocalPlan{}, nil, err
	}
	target = item.Target
	// 删除工具入口时保留链接目标；其他操作针对清单中的真实内容目录。
	if action == "remove" {
		if abs, e := filepath.Abs(target); e == nil {
			target = abs
		}
	}
	pref, err := c.store.localPreference(ctx, target)
	if err != nil {
		return LocalPlan{}, nil, err
	}
	before, err := c.store.installation(ctx, target)
	if err != nil {
		return LocalPlan{}, nil, err
	}
	fp, link, files, err := localSnapshot(target)
	if err != nil {
		return LocalPlan{}, nil, err
	}
	if fp == "" {
		return LocalPlan{}, nil, errors.New("local skill files are missing")
	}
	p := LocalPlan{Action: action, Target: target, Expected: fp, Result: fp, Locations: item.Locations, Before: before, After: before, Association: pref.Association, BeforePreference: pref, Link: link, Differences: []LocalDifference{}}
	return p, files, nil
}
func (c *Client) PlanLocalSource(ctx context.Context, target string, coordinate Coordinate, replace bool) (LocalPlan, error) {
	p, files, err := c.baseLocalPlan(ctx, target, "associate")
	if err != nil {
		return p, err
	}
	item, err := c.LocalSkill(ctx, target)
	if err != nil {
		return p, err
	}
	if item.Manager != "local" && item.Manager != "skillhub" {
		return p, errors.New("this folder is maintained by " + item.Manager + "; copy it to an independent folder before changing its update source")
	}
	if coordinate.Version == "" {
		return p, errors.New("select an exact version")
	}
	remote, err := c.Remote(ctx, coordinate.Source)
	if err != nil {
		return p, err
	}
	release, archive, err := c.downloadRelease(ctx, remote, coordinate)
	if err != nil {
		return p, err
	}
	bundle, err := packageformat.Inspect(archive)
	if err != nil {
		return p, err
	}
	if runtime.GOOS == "windows" {
		for i := range bundle.Files {
			bundle.Files[i].Executable = false
		}
	}
	suite, err := skillsuite.FromBundle(bundle)
	if err != nil {
		return p, err
	}
	if suite != nil {
		return p, errors.New("install skill suites through the suite workflow")
	}
	if filepath.Base(p.Target) != bundle.Metadata.Name {
		return p, errors.New("source skill name must match the local folder name")
	}
	loc := item.Locations[0]
	desired := Installation{ID: localID(p.Target), Target: p.Target, Source: remote.Source.Alias, SourceRef: remote.Source.CredentialRef, RegistryID: remote.Source.RegistryID, RegistryURL: remote.Source.URL, Namespace: coordinate.Namespace, Name: coordinate.Name, Version: coordinate.Version, Digest: release.Digest, Fingerprint: installationFingerprint(bundle), DisplayNameEN: release.DisplayNameEn, DisplayNameZH: release.DisplayNameZh, Icon: release.Icon, Provenance: provenancewire.Decode(release.Provenance), Tool: loc.Tool, Scope: loc.Scope, Project: loc.Project}
	if p.Before != nil {
		desired.ID = p.Before.ID
	}
	p.Association = &desired
	p.After = nil
	p.Differences = differences(files, bundle.Files)
	if p.Expected == desired.Fingerprint {
		p.After = &desired
	}
	if replace {
		p.Action = "replace"
		p.After = &desired
		p.Result = desired.Fingerprint
		p.files = bundle.Files
		p.Result = packageformat.Fingerprint(p.files)
	}
	return p, nil
}
func (c *Client) PlanLocalDetach(ctx context.Context, target string) (LocalPlan, error) {
	p, _, e := c.baseLocalPlan(ctx, target, "detach")
	p.After = nil
	p.Association = nil
	return p, e
}
func (c *Client) PlanLocalRemove(ctx context.Context, path string) (LocalPlan, error) {
	item, err := c.LocalSkill(ctx, path)
	if err != nil {
		return LocalPlan{}, err
	}
	p, _, err := c.baseLocalPlan(ctx, item.Target, "remove")
	if err != nil {
		return p, err
	}
	abs, err := canonicalTarget(path)
	if err != nil {
		return p, err
	}
	if info, e := os.Lstat(abs); e == nil && info.Mode()&os.ModeSymlink != 0 {
		p.Target = abs
		p.Before, err = c.store.installation(ctx, abs)
		if err != nil {
			return p, err
		}
		p.BeforePreference, err = c.store.localPreference(ctx, abs)
		if err != nil {
			return p, err
		}
		p.Expected, p.Link, _, err = localSnapshot(abs)
		if err != nil {
			return p, err
		}
	} else if item.Manager != "local" && item.Manager != "skillhub" {
		return p, errors.New("remove this skill through " + item.Manager)
	}
	p.After = nil
	p.Association = nil
	p.Result = ""
	return p, nil
}
func (c *Client) PlanLocalEdit(ctx context.Context, target, path, expected, content string) (LocalPlan, error) {
	p, files, err := c.baseLocalPlan(ctx, target, "edit")
	if err != nil {
		return p, err
	}
	item, err := c.LocalSkill(ctx, target)
	if err != nil {
		return p, err
	}
	if item.Manager != "local" && item.Manager != "skillhub" {
		return p, errors.New("edit this skill through " + item.Manager + " or make an independent copy")
	}
	if p.Expected != expected {
		return p, ErrStalePlan
	}
	if !utf8.ValidString(content) || len(content) > 1<<20 {
		return p, errors.New("editable text must be UTF-8 and at most 1 MiB")
	}
	next := append([]packageformat.File(nil), files...)
	found := false
	for i, f := range next {
		if f.Path == path {
			found = true
			if !utf8.Valid(f.Data) {
				return p, errors.New("binary file cannot be edited as text")
			}
			next[i].Data = []byte(content)
		}
	}
	if !found {
		return p, errors.New("file not found")
	}
	p.files = next
	p.modes, err = localFileModes(p.Target, next)
	if err != nil {
		return p, err
	}
	p.Result = packageformat.Fingerprint(next)
	p.Differences = differences(files, next)
	return p, nil
}
func (c *Client) PlanLocalCopy(ctx context.Context, target, destination string) (LocalPlan, error) {
	item, err := c.LocalSkill(ctx, target)
	if err != nil {
		return LocalPlan{}, err
	}
	files, err := packageformat.ReadDirectory(item.Target)
	if err != nil {
		return LocalPlan{}, err
	}
	// 独立副本不继承其他管理器的控制文件。
	filtered := make([]packageformat.File, 0, len(files))
	for _, f := range files {
		first := strings.SplitN(f.Path, "/", 2)[0]
		if first == ".git" || first == ".clawhub" || first == ".claude-plugin" || first == ".skill-lock.json" || first == "skills-lock.json" {
			continue
		}
		filtered = append(filtered, f)
	}
	files = filtered
	dest, err := canonicalTarget(destination)
	if err != nil {
		return LocalPlan{}, err
	}
	if filepath.Base(dest) != item.Name {
		return LocalPlan{}, errors.New("copy destination folder must match the skill name")
	}
	if _, err = os.Lstat(dest); !errors.Is(err, os.ErrNotExist) {
		return LocalPlan{}, errors.New("copy destination already exists")
	}
	pref, err := c.store.localPreference(ctx, dest)
	if err != nil {
		return LocalPlan{}, err
	}
	before, err := c.store.installation(ctx, dest)
	if err != nil {
		return LocalPlan{}, err
	}
	if before != nil {
		return LocalPlan{}, errors.New("destination has an installation record")
	}
	modes, err := localFileModes(item.Target, files)
	if err != nil {
		return LocalPlan{}, err
	}
	return LocalPlan{modes: modes, Action: "copy", Target: dest, Result: packageformat.Fingerprint(files), BeforePreference: pref, Locations: []LocalLocation{{Path: dest, Scope: "folder"}}, Differences: differences(nil, files), files: files}, nil
}
func (s *Store) LocalBackups(ctx context.Context) ([]LocalBackup, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT record FROM local_backups ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LocalBackup{}
	for rows.Next() {
		var raw string
		var b LocalBackup
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &b); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, rows.Err()
}
func (c *Client) PlanLocalRestore(ctx context.Context, id string) (LocalPlan, error) {
	var raw string
	var b LocalBackup
	err := c.store.db.QueryRowContext(ctx, "SELECT record FROM local_backups WHERE id=?", id).Scan(&raw)
	if err != nil {
		return LocalPlan{}, err
	}
	if err = json.Unmarshal([]byte(raw), &b); err != nil {
		return LocalPlan{}, err
	}
	if b.Suite {
		return LocalPlan{}, errors.New("restore an installed suite through its suite workflow")
	}
	if err = validateLocalBackup(b); err != nil {
		return LocalPlan{}, err
	}
	expected, link, current, err := localSnapshot(b.Target)
	if err != nil {
		return LocalPlan{}, err
	}
	if link != "" && b.Link == "" {
		return LocalPlan{}, errors.New("restore destination is now a symbolic link")
	}
	fp, backupLink, files, err := localSnapshot(filepath.Join(b.Directory, "old"))
	if err != nil {
		return LocalPlan{}, err
	}
	if fp != b.Fingerprint || backupLink != b.Link {
		return LocalPlan{}, errors.New("backup contents changed")
	}
	before, err := c.store.installation(ctx, b.Target)
	if err != nil {
		return LocalPlan{}, err
	}
	pref, err := c.store.localPreference(ctx, b.Target)
	if err != nil {
		return LocalPlan{}, err
	}
	var modes map[string]os.FileMode
	if b.Link == "" {
		modes, err = localFileModes(filepath.Join(b.Directory, "old"), files)
		if err != nil {
			return LocalPlan{}, err
		}
	}
	return LocalPlan{modes: modes, Action: "restore", Target: b.Target, Expected: expected, Result: b.Fingerprint, Before: before, After: b.Installation, Association: b.Preference.Association, BeforePreference: pref, BackupID: id, Link: b.Link, files: files, Differences: differences(current, files)}, nil
}
func sameLocalPreference(a, b localPreference) bool {
	return a.Hidden == b.Hidden && sameInstallation(a.Association, b.Association)
}
func (c *Client) ApplyLocalPlan(ctx context.Context, p LocalPlan) error {
	// 在写入前重验源权限和摘要；检查失败不会改变本地内容。
	if p.Action == "associate" || p.Action == "replace" {
		if p.Association == nil {
			return errors.New("source association missing")
		}
		a := p.Association
		r, e := c.Remote(ctx, a.Source)
		if e != nil {
			return e
		}
		if r.Source.RegistryID != a.RegistryID || r.Source.URL != a.RegistryURL || r.Source.CredentialRef != a.SourceRef {
			return ErrSourceConflict
		}
		_, data, e := c.downloadRelease(ctx, r, Coordinate{a.Source, a.Namespace, a.Name, a.Version})
		if e != nil {
			return e
		}
		if packageformat.Digest(data) != a.Digest {
			return ErrStalePlan
		}
	}
	return c.store.applyLocalPlan(ctx, p, nil)
}

// 日志先落盘，再切换目录，最后提交索引；旧文件作为显式恢复点保留。
func (s *Store) applyLocalPlan(ctx context.Context, p LocalPlan, hook func(string) error) error {
	return s.withTarget(ctx, "suite-state", func() error {
		suites, err := s.Suites(ctx)
		if err != nil {
			return err
		}
		for _, suite := range suites {
			if suite.Target == p.Target {
				return errors.New("use the suite workflow for this installation")
			}
			for _, m := range suite.Members {
				if m.Target == p.Target {
					return errors.New("this skill belongs to an installed suite; remove the suite before changing its files or source")
				}
			}
		}
		return s.withTarget(ctx, p.Target, func() error {
			if err := s.recoverTarget(ctx, p.Target); err != nil {
				return err
			}
			if err := s.recoverLocal(ctx, p.Target); err != nil {
				return err
			}
			target, err := canonicalTarget(p.Target)
			if err != nil {
				return err
			}
			if target != p.Target {
				return ErrStalePlan
			}
			before, err := s.installation(ctx, p.Target)
			if err != nil {
				return err
			}
			pref, err := s.localPreference(ctx, p.Target)
			if err != nil {
				return err
			}
			fp, _, _, err := localSnapshot(p.Target)
			if err != nil {
				return err
			}
			if fp != p.Expected || !sameInstallation(before, p.Before) || !sameLocalPreference(pref, p.BeforePreference) {
				return ErrStalePlan
			}
			metadata := p.Action == "associate" || p.Action == "detach"
			if !metadata && p.Action != "replace" && p.Action != "edit" && p.Action != "remove" && p.Action != "restore" && p.Action != "copy" {
				return errors.New("invalid local operation")
			}
			if !metadata && p.Action != "remove" && p.Link == "" && packageformat.Fingerprint(p.files) != p.Result {
				return errors.New("local plan contents changed")
			}
			nextPref := localPreference{Hidden: pref.Hidden, Association: p.Association}
			if metadata {
				return s.commitLocal(ctx, p, nextPref, nil)
			}
			if err = os.MkdirAll(filepath.Dir(p.Target), 0700); err != nil {
				return err
			}
			current, err := canonicalTarget(p.Target)
			if err != nil || current != p.Target {
				return ErrStalePlan
			}
			dir, err := os.MkdirTemp(filepath.Dir(p.Target), ".skillhub-local-")
			if err != nil {
				return err
			}
			j := localJournal{Plan: p, Directory: dir, Backup: LocalBackup{ID: uuid.NewString(), Target: p.Target, CreatedAt: time.Now().UTC().Format(time.RFC3339), Action: p.Action, Fingerprint: p.Expected, Directory: dir, Installation: p.Before, Preference: p.BeforePreference}}
			if p.Expected != "" {
				_, j.Backup.Link, _, err = localSnapshot(p.Target)
				if err != nil {
					return err
				}
			}
			if p.Action != "remove" {
				stage := filepath.Join(dir, "new")
				if p.Action == "restore" && p.Link != "" {
					err = os.Symlink(p.Link, stage)
				} else {
					err = os.Mkdir(stage, 0700)
					if err == nil {
						err = (&packageformat.Bundle{Files: p.files}).Extract(stage)
					}
				}
				if err != nil {
					_ = os.RemoveAll(dir)
					return err
				}
				if p.Link == "" {
					for name, mode := range p.modes {
						if name == "." {
							continue
						}
						if err = os.Chmod(filepath.Join(stage, name), mode); err != nil {
							return err
						}
					}
					if mode, ok := p.modes["."]; ok {
						if err = os.Chmod(stage, mode); err != nil {
							return err
						}
					}
					if err = syncTree(stage); err != nil {
						return err
					}
				}
			}
			raw, _ := json.Marshal(j)
			if _, err = s.db.ExecContext(ctx, "INSERT INTO local_journals(target,record) VALUES(?,?)", p.Target, string(raw)); err != nil {
				return err
			}
			if hook != nil {
				if err = hook("prepared"); err != nil {
					return err
				}
			}
			if p.Expected != "" {
				if err = os.Rename(p.Target, filepath.Join(dir, "old")); err != nil {
					return err
				}
			}
			if hook != nil {
				if err = hook("old-moved"); err != nil {
					return err
				}
			}
			if p.Action != "remove" {
				if err = os.Rename(filepath.Join(dir, "new"), p.Target); err != nil {
					return err
				}
			}
			if err = syncDirectory(filepath.Dir(p.Target)); err != nil {
				return err
			}
			if err = syncDirectory(dir); err != nil {
				return err
			}
			if hook != nil {
				if err = hook("new-moved"); err != nil {
					return err
				}
			}
			j.Committed = true
			if err = s.commitLocal(ctx, p, nextPref, &j); err != nil {
				return err
			}
			if hook != nil {
				if err = hook("committed"); err != nil {
					return err
				}
			}
			return s.recoverLocal(ctx, p.Target)
		})
	})
}
func (s *Store) commitLocal(ctx context.Context, p LocalPlan, pref localPreference, j *localJournal) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if p.After == nil {
		_, err = tx.ExecContext(ctx, "DELETE FROM installations WHERE target=?", p.Target)
	} else {
		raw, _ := json.Marshal(p.After)
		_, err = tx.ExecContext(ctx, "INSERT INTO installations(target,id,record) VALUES(?,?,?) ON CONFLICT(target) DO UPDATE SET id=excluded.id,record=excluded.record", p.Target, p.After.ID, string(raw))
	}
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(pref)
	if _, err = tx.ExecContext(ctx, "INSERT INTO local_preferences(target,record) VALUES(?,?) ON CONFLICT(target) DO UPDATE SET record=excluded.record", p.Target, string(raw)); err != nil {
		return err
	}
	if p.Action == "copy" || p.Action == "associate" || p.Action == "replace" || p.Action == "detach" {
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO local_roots(root) VALUES(?)", p.Target); err != nil {
			return err
		}
	}
	if j != nil {
		if p.Expected != "" {
			raw, _ = json.Marshal(j.Backup)
			if _, err = tx.ExecContext(ctx, "INSERT INTO local_backups(id,record) VALUES(?,?)", j.Backup.ID, string(raw)); err != nil {
				return err
			}
		}
		raw, _ = json.Marshal(j)
		if _, err = tx.ExecContext(ctx, "UPDATE local_journals SET record=? WHERE target=?", string(raw), p.Target); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) recoverLocal(ctx context.Context, target string) error {
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT record FROM local_journals WHERE target=?", target).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var j localJournal
	if err = json.Unmarshal([]byte(raw), &j); err != nil {
		return err
	}
	if j.Plan.Target != target || filepath.Dir(j.Directory) != filepath.Dir(target) || !strings.HasPrefix(filepath.Base(j.Directory), ".skillhub-local-") {
		return errors.New("invalid local recovery journal")
	}
	current, e := canonicalTarget(target)
	if e != nil || current != target {
		return errors.New("local recovery parent changed")
	}
	if info, e := os.Lstat(j.Directory); e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("local recovery directory missing or changed")
	}
	old := filepath.Join(j.Directory, "old")
	oldFP, _, _, err := localSnapshot(old)
	if err != nil {
		return err
	}
	if !j.Committed {
		fp, _, _, err := localSnapshot(target)
		if err != nil {
			return err
		}
		if oldFP != "" {
			if oldFP != j.Plan.Expected {
				return errors.New("local backup changed during recovery")
			}
			if fp != "" {
				if fp != j.Plan.Result {
					return errors.New("local target changed during recovery")
				}
				if err = os.RemoveAll(target); err != nil {
					return err
				}
			}
			if err = os.Rename(old, target); err != nil {
				return err
			}
		} else if j.Plan.Expected == "" {
			if fp != "" {
				if fp != j.Plan.Result {
					return errors.New("local target changed during recovery")
				}
				if err = os.RemoveAll(target); err != nil {
					return err
				}
			}
		} else if fp != j.Plan.Expected {
			return errors.New("local original files unavailable")
		}
		if err = syncDirectory(filepath.Dir(target)); err != nil {
			return err
		}
		if err = os.RemoveAll(j.Directory); err != nil {
			return err
		}
	} else if j.Plan.Expected == "" {
		if err = os.RemoveAll(j.Directory); err != nil {
			return err
		}
	} else if oldFP != j.Plan.Expected {
		return errors.New("saved backup changed")
	}
	_, err = s.db.ExecContext(ctx, "DELETE FROM local_journals WHERE target=?", target)
	return err
}
func (s *Store) recoverLocalChanges(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, "SELECT target FROM local_journals")
	if err != nil {
		return err
	}
	targets := []string{}
	for rows.Next() {
		var t string
		if err = rows.Scan(&t); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, t := range targets {
		if err = s.withTarget(ctx, t, func() error { return s.recoverLocal(ctx, t) }); err != nil {
			return fmt.Errorf("recover local change: %w", err)
		}
	}
	return nil
}

func validateLocalBackup(b LocalBackup) error {
	if filepath.Dir(b.Directory) != filepath.Dir(b.Target) || (!strings.HasPrefix(filepath.Base(b.Directory), ".skillhub-local-") && !strings.HasPrefix(filepath.Base(b.Directory), ".skillhub-txn-")) {
		return errors.New("invalid backup directory")
	}
	canonical, err := canonicalTarget(b.Target)
	if err != nil || canonical != b.Target {
		return errors.New("backup parent changed")
	}
	info, err := os.Lstat(b.Directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("backup directory changed")
	}
	return nil
}

// 用户显式清理恢复点；只删除指纹仍匹配的受管备份。
func (s *Store) DeleteLocalBackup(ctx context.Context, id string) error {
	var raw string
	var b LocalBackup
	if err := s.db.QueryRowContext(ctx, "SELECT record FROM local_backups WHERE id=?", id).Scan(&raw); err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		return err
	}
	return s.withTarget(ctx, b.Target, func() error {
		if err := s.recoverTarget(ctx, b.Target); err != nil {
			return err
		}
		err := validateLocalBackup(b)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil {
			fp, link, _, e := localSnapshot(filepath.Join(b.Directory, "old"))
			if e != nil {
				return e
			}
			if fp != b.Fingerprint || link != b.Link {
				return errors.New("backup contents changed; inspect the folder before deleting")
			}
			if e = os.RemoveAll(b.Directory); e != nil {
				return e
			}
		}
		_, err = s.db.ExecContext(ctx, "DELETE FROM local_backups WHERE id=?", id)
		return err
	})
}

func localFileModes(target string, files []packageformat.File) (map[string]os.FileMode, error) {
	root, err := os.OpenRoot(target)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	modes := map[string]os.FileMode{}
	for _, f := range files {
		for path := f.Path; ; path = filepath.Dir(path) {
			if _, ok := modes[path]; ok {
				break
			}
			info, e := root.Lstat(path)
			if e != nil {
				return nil, e
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil, errors.New("local file changed to symbolic link")
			}
			modes[path] = info.Mode().Perm()
			if path == "." {
				break
			}
		}
	}
	return modes, nil
}
