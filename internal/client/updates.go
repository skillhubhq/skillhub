package client

import (
	"context"
	"encoding/json"
	"errors"

	"connectrpc.com/connect"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	"golang.org/x/mod/semver"
)

var ErrUpdateConnection = errors.New("installation connection is unavailable; reconnect or choose an account for this Hub")

var ErrInstalledVersionUnavailable = errors.New("installed version is no longer readable or published; local files are preserved but restore is unavailable")

var ErrNoUpdate = errors.New("no newer stable version is available")

type UpdateCheck struct {
	VersionUnavailable bool         `json:"version_unavailable"`
	Installation       Installation `json:"installation"`
	Source             string       `json:"source,omitempty"`
	AvailableVersion   string       `json:"available_version,omitempty"`
	AvailableDigest    string       `json:"available_digest,omitempty"`
	Error              string       `json:"error,omitempty"`
	ConnectionRequired bool         `json:"connection_required,omitempty"`
}

func (c *Client) installationSource(ctx context.Context, installation Installation, alias string) (Source, error) {
	if alias != "" {
		source, err := c.Source(ctx, alias)
		if err != nil {
			return Source{}, err
		}
		if source.RegistryID != installation.RegistryID || source.URL != installation.RegistryURL {
			return Source{}, ErrRegistryChanged
		}
		return source, nil
	}
	// 连接引用区分删除后重建的同名别名，不能悄悄换用另一个身份。
	if installation.Source != "" {
		source, err := c.Source(ctx, installation.Source)
		if err != nil || source.RegistryID != installation.RegistryID || source.URL != installation.RegistryURL || source.CredentialRef != installation.SourceRef {
			return Source{}, ErrUpdateConnection
		}
		return source, nil
	}
	source, err := c.lockedSource(ctx, LockedSkill{RegistryID: installation.RegistryID, RegistryURL: installation.RegistryURL})
	if err != nil {
		return Source{}, ErrUpdateConnection
	}
	return source, nil
}

// 最新发布可能是旧分支补丁；更新检查遍历可见版本，仅选择高于当前版本的稳定版。
func stableUpdateState(ctx context.Context, remote *Remote, installation Installation) (string, string, bool, error) {
	current := "v" + installation.Version
	if !semver.IsValid(current) {
		return "", "", false, errors.New("installed version is not a semantic version")
	}
	version, digest, token := "", "", ""
	installedAvailable := false
	seen := map[string]bool{}
	for page := 0; page < 100; page++ {
		response, err := remote.Catalog.ListReleases(ctx, connect.NewRequest(&catalogv1.ListReleasesRequest{Namespace: installation.Namespace, Name: installation.Name, Page: &commonv1.PageRequest{PageSize: 100, PageToken: token}}))
		if err != nil {
			return "", "", false, err
		}
		for _, release := range response.Msg.Releases {
			if release == nil || release.Namespace != installation.Namespace || release.Name != installation.Name || !semver.IsValid("v"+release.Version) || !validDigest(release.Digest) {
				return "", "", false, errors.New("invalid release in source response")
			}
			if release.Version == installation.Version && release.Digest == installation.Digest {
				installedAvailable = true
			}
			candidate := "v" + release.Version
			if semver.Prerelease(candidate) != "" || semver.Compare(candidate, current) <= 0 {
				continue
			}
			if version == "" || semver.Compare(candidate, "v"+version) > 0 {
				version, digest = release.Version, release.Digest
			}
		}
		token = response.Msg.GetPage().GetNextPageToken()
		if token == "" {
			return version, digest, installedAvailable, nil
		}
		if seen[token] {
			return "", "", false, errors.New("source repeated its release cursor")
		}
		seen[token] = true
	}
	return "", "", false, errors.New("update discovery exceeded 100 release pages; choose an explicit version")
}

func newerStable(ctx context.Context, remote *Remote, installation Installation) (string, string, error) {
	version, digest, available, err := stableUpdateState(ctx, remote, installation)
	if err == nil && !available && version == "" {
		err = ErrInstalledVersionUnavailable
	}
	return version, digest, err
}

func (c *Client) CheckUpdates(ctx context.Context, target string) ([]UpdateCheck, error) {
	return c.CheckUpdatesFrom(ctx, target, "")
}

// 沿用安装连接；旧记录或显式改选只在成功验证读取权限后记住连接。
func (c *Client) CheckUpdatesFrom(ctx context.Context, target, alias string) ([]UpdateCheck, error) {
	installs, err := c.store.ListInstallations(ctx)
	if err != nil {
		return nil, err
	}
	if target != "" {
		target, err = canonicalTarget(target)
		if err != nil {
			return nil, err
		}
	}
	checks := []UpdateCheck{}
	for _, installation := range installs {
		if target != "" && installation.Target != target {
			continue
		}
		check := UpdateCheck{Installation: installation}
		err := func() error {
			source, err := c.installationSource(ctx, installation, alias)
			if err != nil {
				return err
			}
			check.Source = source.Alias
			remote, err := c.Remote(ctx, source.Alias)
			if err != nil {
				return err
			}
			if remote.Source.RegistryID != installation.RegistryID || remote.Source.URL != installation.RegistryURL || remote.Source.CredentialRef != source.CredentialRef {
				return ErrRegistryChanged
			}
			var available bool
			check.AvailableVersion, check.AvailableDigest, available, err = stableUpdateState(ctx, remote, installation)
			check.VersionUnavailable = err == nil && !available
			if err != nil {
				return err
			}
			updated, err := c.store.rememberInstallationSource(ctx, installation, remote.Source)
			if err == nil {
				check.Installation = updated
			}
			return err
		}()
		if err != nil {
			check.Error = err.Error()
			check.ConnectionRequired = errors.Is(err, ErrUpdateConnection)
		}
		checks = append(checks, check)
	}
	if target != "" && len(checks) == 0 {
		return nil, errors.New("installation not found")
	}
	return checks, nil
}

// 空版本表示选择更高的稳定版本；显式版本允许用户审阅计划后降级或选择预发行版。
func (c *Client) PlanRemoteUpdate(ctx context.Context, target, version string) (*InstallPlan, []byte, error) {
	return c.PlanRemoteUpdateFrom(ctx, target, version, "")
}

// 账号只用于下载授权，不能改变安装记录绑定的 Hub 身份。
func (c *Client) PlanRemoteUpdateFrom(ctx context.Context, target, version, alias string) (*InstallPlan, []byte, error) {
	target, err := canonicalTarget(target)
	if err != nil {
		return nil, nil, err
	}
	original, err := c.store.installation(ctx, target)
	if err != nil {
		return nil, nil, err
	}
	if original == nil {
		return nil, nil, errors.New("installation not found")
	}
	source, err := c.installationSource(ctx, *original, alias)
	if err != nil {
		return nil, nil, err
	}
	expectedDigest := ""
	if version == "" {
		remote, err := c.Remote(ctx, source.Alias)
		if err != nil {
			return nil, nil, err
		}
		if remote.Source.RegistryID != original.RegistryID || remote.Source.URL != original.RegistryURL || remote.Source.CredentialRef != source.CredentialRef {
			return nil, nil, ErrRegistryChanged
		}
		version, expectedDigest, err = newerStable(ctx, remote, *original)
		if err != nil {
			return nil, nil, err
		}
		if version == "" {
			return nil, nil, ErrNoUpdate
		}
	}
	plan, archive, err := c.PlanRemoteInstall(ctx, Coordinate{Source: source.Alias, Namespace: original.Namespace, Name: original.Name, Version: version}, original.Tool, original.Scope, original.Project)
	if err != nil {
		return nil, nil, err
	}
	if plan.Target != original.Target || !sameInstallation(plan.Before, original) {
		return nil, nil, ErrStalePlan
	}
	if plan.After.RegistryID != original.RegistryID || plan.After.RegistryURL != original.RegistryURL || plan.After.SourceRef != source.CredentialRef {
		return nil, nil, ErrRegistryChanged
	}
	if expectedDigest != "" && plan.After.Digest != expectedDigest {
		return nil, nil, errors.New("source version changed during update discovery")
	}
	return plan, archive, nil
}

// 仅更新本机来源元数据；目标锁和旧记录比较防止覆盖并发安装或卸载。
func (s *Store) rememberInstallationSource(ctx context.Context, original Installation, source Source) (Installation, error) {
	if original.Source == source.Alias && original.SourceRef == source.CredentialRef {
		return original, nil
	}
	updated := original
	updated.Source, updated.SourceRef = source.Alias, source.CredentialRef
	err := s.withTarget(ctx, original.Target, func() error {
		if err := s.recoverTarget(ctx, original.Target); err != nil {
			return err
		}
		current, err := s.installation(ctx, original.Target)
		if err != nil {
			return err
		}
		if !sameInstallation(current, &original) {
			return ErrStalePlan
		}
		raw, err := json.Marshal(updated)
		if err != nil {
			return err
		}
		_, err = s.db.ExecContext(ctx, "UPDATE installations SET record=? WHERE target=?", string(raw), original.Target)
		return err
	})
	return updated, err
}
