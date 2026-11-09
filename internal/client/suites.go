package client

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"connectrpc.com/connect"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/skillsuite"
)

type SuiteMemberInstallation struct {
	Target  string `json:"target"`
	Digest  string `json:"digest"`
	Managed bool   `json:"managed"`
}
type SuiteInstallation struct {
	Target     string                    `json:"target"`
	Coordinate Coordinate                `json:"coordinate"`
	Members    []SuiteMemberInstallation `json:"members"`
}
type SuiteInstallPlan struct {
	Plans       []InstallPlan `json:"plans"`
	Retained    []string      `json:"retained"`
	record      SuiteInstallation
	previous    string
	remove      bool
	coordinates map[string]Coordinate
}

func (s *Store) Suites(ctx context.Context) ([]SuiteInstallation, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT record FROM suite_installations ORDER BY target")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SuiteInstallation{}
	for rows.Next() {
		var raw string
		var v SuiteInstallation
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
func suiteSnapshot(records []SuiteInstallation) string { return metadataDigest(records) }
func sharedSuiteTarget(records []SuiteInstallation, parent, target string) bool {
	for _, r := range records {
		if r.Target != parent {
			for _, m := range r.Members {
				if m.Target == target {
					return true
				}
			}
		}
	}
	return false
}
func ownedSuiteTarget(records []SuiteInstallation, target, digest string) bool {
	for _, r := range records {
		for _, m := range r.Members {
			if m.Target == target && m.Digest == digest && m.Managed {
				return true
			}
		}
	}
	return false
}
func (c *Client) PlanSuiteInstall(ctx context.Context, coordinate Coordinate, tool, scope, project string) (*SuiteInstallPlan, error) {
	parent, data, err := c.PlanRemoteInstall(ctx, coordinate, tool, scope, project)
	if err != nil {
		return nil, err
	}
	b, err := packageformat.Inspect(data)
	if err != nil {
		return nil, err
	}
	m, err := skillsuite.FromBundle(b)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errors.New("selected version is not a skill suite")
	}
	records, err := c.store.Suites(ctx)
	if err != nil {
		return nil, err
	}
	result := &SuiteInstallPlan{Plans: []InstallPlan{}, Retained: []string{}, record: SuiteInstallation{Target: parent.Target, Coordinate: coordinate, Members: []SuiteMemberInstallation{}}, previous: suiteSnapshot(records), coordinates: map[string]Coordinate{parent.Target: coordinate}}
	targets := map[string]bool{parent.Target: true}
	for _, member := range m.Members {
		child := Coordinate{Source: coordinate.Source, Namespace: member.Namespace, Name: member.Name, Version: member.Version}
		plan, _, e := c.PlanRemoteInstall(ctx, child, tool, scope, project)
		if e != nil {
			return nil, e
		}
		if targets[plan.Target] || plan.After.Digest != member.Digest {
			return nil, ErrSourceConflict
		}
		targets[plan.Target] = true
		managed := plan.Before == nil
		if plan.Before != nil {
			managed = ownedSuiteTarget(records, plan.Target, plan.Before.Digest)
			if plan.Before.Digest != member.Digest && (!managed || sharedSuiteTarget(records, parent.Target, plan.Target)) {
				return nil, errors.New("suite conflicts with an independently installed or shared version")
			}
		}
		result.record.Members = append(result.record.Members, SuiteMemberInstallation{Target: plan.Target, Digest: member.Digest, Managed: managed})
		result.Plans = append(result.Plans, *plan)
		result.coordinates[plan.Target] = child
	}
	for _, old := range records {
		if old.Target == parent.Target {
			for _, member := range old.Members {
				if !targets[member.Target] {
					if err = c.planSuiteRemoval(ctx, result, records, member); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	result.Plans = append(result.Plans, *parent)
	return result, nil
}
func (c *Client) planSuiteRemoval(ctx context.Context, p *SuiteInstallPlan, records []SuiteInstallation, m SuiteMemberInstallation) error {
	for _, v := range p.Plans {
		if v.Target == m.Target {
			return nil
		}
	}
	for _, v := range p.Retained {
		if v == m.Target {
			return nil
		}
	}

	current, err := c.store.installation(ctx, m.Target)
	if err != nil {
		return err
	}
	if current == nil {
		return nil
	}
	// 中断更新的收据可能同时包含新旧摘要，按磁盘上实际版本确认归属。
	if current.Digest != m.Digest {
		for _, r := range records {
			if r.Target == p.record.Target {
				for _, candidate := range r.Members {
					if candidate.Target == m.Target && candidate.Digest == current.Digest {
						m = candidate
					}
				}
			}
		}
	}
	if !m.Managed || sharedSuiteTarget(records, p.record.Target, m.Target) || current.Digest != m.Digest {
		p.Retained = append(p.Retained, m.Target)
		return nil
	}
	removal, err := c.store.PlanRemove(ctx, m.Target)
	if errors.Is(err, ErrModified) {
		p.Retained = append(p.Retained, m.Target)
		return nil
	}
	if err != nil {
		return err
	}
	p.Plans = append(p.Plans, *removal)
	return nil
}
func (c *Client) PlanSuiteRemove(ctx context.Context, target string) (*SuiteInstallPlan, error) {
	target, err := canonicalTarget(target)
	if err != nil {
		return nil, err
	}
	records, err := c.store.Suites(ctx)
	if err != nil {
		return nil, err
	}
	p := &SuiteInstallPlan{Plans: []InstallPlan{}, Retained: []string{}, previous: suiteSnapshot(records), remove: true, record: SuiteInstallation{Target: target}}
	found := false
	for _, r := range records {
		if r.Target == target {
			found = true
			for _, m := range r.Members {
				if err = c.planSuiteRemoval(ctx, p, records, m); err != nil {
					return nil, err
				}
			}
		}
	}
	if !found {
		return nil, errors.New("suite installation not found")
	}
	current, err := c.store.installation(ctx, target)
	if err != nil {
		return nil, err
	}
	if current != nil {
		parent, err := c.store.PlanRemove(ctx, target)
		if err != nil {
			return nil, err
		}
		p.Plans = append(p.Plans, *parent)
	}
	return p, nil
}

// 逐目标原子写入，失败时保存成员归属以便重试；已修改和共享成员不会被清理。
func (c *Client) ApplySuite(ctx context.Context, p *SuiteInstallPlan) ([]ProjectRestoreResult, error) {
	if p == nil {
		return nil, ErrStalePlan
	}
	results := []ProjectRestoreResult{}
	err := c.store.withTarget(ctx, "suite-state", func() error {
		records, e := c.store.Suites(ctx)
		if e != nil {
			return e
		}
		if suiteSnapshot(records) != p.previous {
			return ErrStalePlan
		}
		// 在第一次写入前重新取得所有成员并检查权限与摘要；总内存有上限。
		archives := map[string][]byte{}
		total := 0
		for _, plan := range p.Plans {
			if plan.Action == "remove" {
				current, e := c.store.PlanRemove(ctx, plan.Target)
				if e != nil {
					return e
				}
				if !reflect.DeepEqual(current.Before, plan.Before) {
					return ErrStalePlan
				}
				continue
			}
			coord := p.coordinates[plan.Target]
			remote, e := c.Remote(ctx, coord.Source)
			if e != nil {
				return e
			}
			_, raw, e := c.downloadRelease(ctx, remote, coord)
			if e != nil {
				return e
			}
			if packageformat.Digest(raw) != plan.After.Digest {
				return ErrStalePlan
			}
			current, e := c.store.PlanInstall(ctx, *plan.After, raw)
			if e != nil {
				return e
			}
			if !reflect.DeepEqual(current.Before, plan.Before) {
				return ErrStalePlan
			}
			total += len(raw)
			if total > 64<<20 {
				return errors.New("suite exceeds the 64 MiB installation budget")
			}
			archives[plan.Target] = raw
		}
		if !p.remove {
			// 同时保留待删除的旧成员；若进程中断，下次计划仍能定位它们。
			pending := p.record
			pending.Members = append([]SuiteMemberInstallation{}, p.record.Members...)
			for _, old := range records {
				if old.Target == p.record.Target {
					for _, m := range old.Members {
						found := false
						for _, n := range pending.Members {
							found = found || (n.Target == m.Target && n.Digest == m.Digest)
						}
						if !found {
							pending.Members = append(pending.Members, m)
						}
					}
				}
			}
			raw, _ := json.Marshal(pending)
			if _, e = c.store.db.ExecContext(ctx, "INSERT INTO suite_installations(target,record) VALUES(?,?) ON CONFLICT(target) DO UPDATE SET record=excluded.record", p.record.Target, string(raw)); e != nil {
				return e
			}
		}
		for _, plan := range p.Plans {
			if e = c.store.apply(ctx, plan, archives[plan.Target], nil); e != nil {
				results = append(results, ProjectRestoreResult{Target: plan.Target, Error: e.Error()})
				return e
			}
			results = append(results, ProjectRestoreResult{Target: plan.Target, Applied: true})
		}
		if p.remove {
			_, e = c.store.db.ExecContext(ctx, "DELETE FROM suite_installations WHERE target=?", p.record.Target)
		} else {
			raw, _ := json.Marshal(p.record)
			_, e = c.store.db.ExecContext(ctx, "UPDATE suite_installations SET record=? WHERE target=?", string(raw), p.record.Target)
		}
		return e
	})
	return results, err
}

type PublishSuite struct {
	Source      string              `json:"source"`
	Namespace   string              `json:"namespace"`
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Description string              `json:"description"`
	Visibility  taxonomy.Visibility `json:"visibility"`
	Members     []skillsuite.Member `json:"members"`
	Entry       string              `json:"entry"`
	RequestID   string              `json:"request_id"`
}

func (c *Client) PublishSuite(ctx context.Context, form PublishSuite) (ReleaseInfo, error) {
	r, err := c.featureRemote(ctx, form.Source, "catalog.suites.v1")
	if err != nil {
		return ReleaseInfo{}, err
	}
	if len(form.Members) < 1 || len(form.Members) > 50 {
		return ReleaseInfo{}, errors.New("a suite requires 1–50 members")
	}
	if !packageformat.ValidName(form.Name) || len(form.Description) < 1 || len(form.Description) > 1000 {
		return ReleaseInfo{}, errors.New("suite name and description are required")
	}
	m := skillsuite.Manifest{Schema: 1, Entry: form.Entry, Members: append([]skillsuite.Member{}, form.Members...)}
	for i, member := range m.Members {
		release, raw, e := c.downloadRelease(ctx, r, Coordinate{Source: form.Source, Namespace: member.Namespace, Name: member.Name, Version: member.Version})
		if e != nil {
			return ReleaseInfo{}, e
		}
		b, e := packageformat.Inspect(raw)
		if e != nil {
			return ReleaseInfo{}, e
		}
		nested, e := skillsuite.FromBundle(b)
		if e != nil || nested != nil {
			return ReleaseInfo{}, errors.New("nested suites are not supported")
		}
		if member.Digest != "" && member.Digest != release.Digest {
			return ReleaseInfo{}, ErrStalePlan
		}
		m.Members[i].Digest = release.Digest
	}
	manifest, _ := json.MarshalIndent(m, "", "  ")
	if _, err = skillsuite.Parse(manifest); err != nil {
		return ReleaseInfo{}, err
	}
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	description, _ := json.Marshal(form.Description)
	markdown := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n# %s\n\n%s\n\nEntry skill: `%s`.\n", form.Name, description, form.Name, form.Description, form.Entry)
	for _, v := range m.Members {
		markdown += fmt.Sprintf("\n- `%s/%s@%s`", v.Namespace, v.Name, v.Version)
	}
	for _, f := range []struct{ name, body string }{{"SKILL.md", markdown}, {skillsuite.Filename, string(manifest)}} {
		file, e := w.Create(f.name)
		if e != nil {
			return ReleaseInfo{}, e
		}
		if _, e = file.Write([]byte(f.body)); e != nil {
			return ReleaseInfo{}, e
		}
	}
	if err = w.Close(); err != nil {
		return ReleaseInfo{}, err
	}
	v, err := r.Publishing.SubmitRelease(ctx, connect.NewRequest(&publishingv1.SubmitReleaseRequest{Mutation: &commonv1.Mutation{IdempotencyKey: form.RequestID}, Namespace: form.Namespace, Version: form.Version, Archive: buffer.Bytes(), Visibility: form.Visibility}))
	if err != nil {
		return ReleaseInfo{}, err
	}
	return ReleaseInformation(v.Msg.Release), nil
}

// 所有交互入口在展示文件计划前展开套装。
func (c *Client) ExpandSuitePlan(ctx context.Context, plan *InstallPlan, data []byte) (*SuiteInstallPlan, error) {
	if plan == nil {
		return nil, ErrStalePlan
	}
	records, err := c.store.Suites(ctx)
	if err != nil {
		return nil, err
	}
	registered := false
	for _, r := range records {
		registered = registered || r.Target == plan.Target
	}
	if plan.Action == "remove" {
		if registered {
			return c.PlanSuiteRemove(ctx, plan.Target)
		}
		return nil, nil
	}
	b, err := packageformat.Inspect(data)
	if err != nil {
		return nil, err
	}
	m, err := skillsuite.FromBundle(b)
	if err != nil {
		return nil, err
	}
	if m == nil {
		if registered {
			return nil, errors.New("remove the installed suite before replacing it with a single skill")
		}
		return nil, nil
	}
	i := plan.After
	return c.PlanSuiteInstall(ctx, Coordinate{Source: i.Source, Namespace: i.Namespace, Name: i.Name, Version: i.Version}, i.Tool, i.Scope, i.Project)
}
