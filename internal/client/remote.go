package client

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"connectrpc.com/connect"
	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/packageformat"
)

type SearchResult struct {
	Source   string             `json:"source"`
	Skills   []*catalogv1.Skill `json:"skills"`
	NextPage string             `json:"next_page,omitempty"`
	Error    string             `json:"error,omitempty"`
}

func (c *Client) Search(ctx context.Context, aliases []string, request *catalogv1.SearchSkillsRequest) ([]SearchResult, error) {
	if len(aliases) == 0 {
		sources, err := c.ListSources(ctx)
		if err != nil {
			return nil, err
		}
		for _, source := range sources {
			aliases = append(aliases, source.Alias)
		}
	}
	if len(aliases) > 1 && request.GetPage().GetPageToken() != "" {
		return nil, errors.New("a cursor belongs to exactly one source")
	}
	result := make([]SearchResult, len(aliases))
	slots := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, alias := range aliases {
		wg.Go(func() {
			result[i] = SearchResult{Source: alias, Skills: []*catalogv1.Skill{}}
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				result[i].Error = "request canceled"
				return
			}
			remote, err := c.Remote(ctx, alias)
			if err != nil {
				result[i].Error = sourceError(err)
				return
			}
			if (request.Order != 0 || request.FeaturedOnly) && !slices.Contains(remote.Capabilities, "catalog.discovery.v1") {
				result[i].Error = "This Hub does not support this catalog order or featured filter. Choose default name order."
				return
			}
			response, err := remote.Catalog.SearchSkills(ctx, connect.NewRequest(request))
			if err != nil {
				result[i].Error = sourceError(err)
				return
			}
			result[i].Skills = response.Msg.Skills
			result[i].NextPage = response.Msg.GetPage().GetNextPageToken()
		})
	}
	wg.Wait()
	return result, nil
}

func sourceError(err error) string {
	if errors.Is(err, ErrRegistryChanged) {
		return ErrRegistryChanged.Error()
	}
	if errors.Is(err, ErrCredentialMissing) {
		return "saved credential is missing; log in again"
	}
	if errors.Is(err, ErrCredentialStore) {
		return ErrCredentialStore.Error()
	}
	if errors.Is(err, ErrSourceNotFound) {
		return ErrSourceNotFound.Error()
	}
	switch connect.CodeOf(err) {
	case connect.CodeUnauthenticated:
		return "login required or credential expired"
	case connect.CodePermissionDenied:
		return "permission denied"
	case connect.CodeInvalidArgument:
		return "invalid request or cursor"
	default:
		return "source request failed"
	}
}

type Coordinate struct {
	Source    string
	Namespace string
	Name      string
	Version   string
}

func ParseCoordinate(value string, versionRequired bool) (Coordinate, error) {
	var coordinate Coordinate
	parts := strings.Split(value, "@")
	if len(parts) > 2 {
		return coordinate, errors.New("use source/namespace/name@version")
	}
	path := strings.Split(parts[0], "/")
	if len(path) != 3 {
		return coordinate, errors.New("use source/namespace/name@version")
	}
	for _, part := range path {
		if !packageformat.ValidName(part) {
			return coordinate, errors.New("invalid skill coordinate")
		}
	}
	coordinate.Source = path[0]
	coordinate.Namespace = path[1]
	coordinate.Name = path[2]
	if len(parts) == 2 {
		coordinate.Version = parts[1]
	}
	if versionRequired && coordinate.Version == "" {
		return coordinate, errors.New("an exact version is required")
	}
	if len(coordinate.Version) > 128 {
		return coordinate, errors.New("invalid version")
	}
	return coordinate, nil
}

// ResolveTarget 只按工具公开的技能目录规则定位，不启动工具或执行技能。
func ResolveTarget(tool, scope, project, home, name string) (string, string, error) {
	if !packageformat.ValidName(name) {
		return "", "", errors.New("invalid skill name")
	}
	directory := ""
	switch tool {
	case "codex":
		directory = ".agents"
	case "claude-code":
		directory = ".claude"
		if name == "synced" || name == "anthropic-skills" {
			return "", "", errors.New("skill name is reserved by Claude Code")
		}
	default:
		return "", "", errors.New("supported tools: codex, claude-code")
	}
	var base string
	var err error
	canonicalProject := ""
	switch scope {
	case "global":
		if project != "" {
			return "", "", errors.New("global installation does not accept a project")
		}
		base = home
		if base == "" {
			base, err = os.UserHomeDir()
		}
	case "project":
		if project == "" {
			project, err = os.Getwd()
		}
		if err == nil {
			base, err = filepath.Abs(project)
		}
		if err == nil {
			base, err = filepath.EvalSymlinks(base)
		}
		if err == nil {
			var info os.FileInfo
			info, err = os.Stat(base)
			if err == nil && !info.IsDir() {
				err = errors.New("project must be an existing directory")
			}
		}
		if err == nil {
			base, err = physicalSpelling(base)
		}
		canonicalProject = base
	default:
		return "", "", errors.New("scope must be global or project")
	}
	if err != nil {
		return "", "", err
	}
	target, err := canonicalTarget(filepath.Join(base, directory, "skills", name))
	if err == nil && scope == "project" {
		// 工具目录可以在项目内复用，但不能由仓库中的符号链接导向项目外。
		relative, relativeErr := filepath.Rel(canonicalProject, target)
		if relativeErr != nil || !filepath.IsLocal(relative) {
			return "", "", errors.New("skill target resolves outside the selected project")
		}
	}
	return target, canonicalProject, err
}

func (c *Client) PlanRemoteInstall(ctx context.Context, coordinate Coordinate, tool, scope, project string) (*InstallPlan, []byte, error) {
	if coordinate.Version == "" {
		return nil, nil, errors.New("an exact version is required")
	}
	remote, err := c.Remote(ctx, coordinate.Source)
	if err != nil {
		return nil, nil, err
	}
	release, archive, err := c.downloadRelease(ctx, remote, coordinate)
	if err != nil {
		return nil, nil, err
	}
	target, canonicalProject, err := ResolveTarget(tool, scope, project, c.home, coordinate.Name)
	if err != nil {
		return nil, nil, err
	}
	desired := Installation{Source: remote.Source.Alias, SourceRef: remote.Source.CredentialRef, RegistryID: remote.Source.RegistryID, RegistryURL: remote.Source.URL, Namespace: coordinate.Namespace, Name: coordinate.Name, Version: coordinate.Version, Digest: release.Digest, DisplayNameEN: release.DisplayNameEn, DisplayNameZH: release.DisplayNameZh, Icon: release.Icon, Tool: tool, Scope: scope, Project: canonicalProject, Target: target}
	plan, err := c.store.PlanInstall(ctx, desired, archive)
	return plan, archive, err
}

func (c *Client) Releases(ctx context.Context, coordinate Coordinate, token string) (*catalogv1.ListReleasesResponse, error) {
	remote, err := c.Remote(ctx, coordinate.Source)
	if err != nil {
		return nil, err
	}
	response, err := remote.Catalog.ListReleases(ctx, connect.NewRequest(&catalogv1.ListReleasesRequest{Namespace: coordinate.Namespace, Name: coordinate.Name, Page: &commonv1.PageRequest{PageSize: 100, PageToken: token}}))
	if err != nil {
		return nil, err
	}
	return response.Msg, nil
}

func (c *Client) downloadExact(ctx context.Context, remote *Remote, coordinate Coordinate) ([]byte, string, error) {
	release, archive, err := c.downloadRelease(ctx, remote, coordinate)
	if err != nil {
		return nil, "", err
	}
	return archive, release.Digest, nil
}

func (c *Client) downloadRelease(ctx context.Context, remote *Remote, coordinate Coordinate) (*catalogv1.Release, []byte, error) {
	key := packageCacheKey(remote.Source, coordinate)
	cached, cacheErr := c.store.cachedPackage(ctx, key)
	knownDigest := ""
	if cacheErr == nil && len(cached) > 0 {
		knownDigest = packageformat.Digest(cached)
	}
	// 无论是否命中缓存，都访问原 Hub 验证当前身份、受众和发布状态。
	response, err := remote.Catalog.DownloadRelease(ctx, connect.NewRequest(&catalogv1.DownloadReleaseRequest{Namespace: coordinate.Namespace, Name: coordinate.Name, Version: coordinate.Version, KnownDigest: knownDigest}))
	if err != nil {
		return nil, nil, err
	}
	release := response.Msg.Release
	if release == nil || release.Namespace != coordinate.Namespace || release.Name != coordinate.Name || release.Version != coordinate.Version || release.Digest != response.Msg.Digest || release.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_PUBLISHED {
		return nil, nil, errors.New("download does not match requested release")
	}
	archive := response.Msg.Archive
	if response.Msg.NotModified {
		if knownDigest == "" || release.Digest != knownDigest || len(archive) != 0 {
			return nil, nil, errors.New("invalid conditional download response")
		}
		archive = cached
	}
	if packageformat.Digest(archive) != release.Digest {
		return nil, nil, errors.New("download does not match requested release")
	}
	bundle, err := packageformat.Inspect(archive)
	if err != nil || bundle.Metadata.Name != coordinate.Name {
		return nil, nil, errors.New("downloaded package is invalid")
	}
	// 缓存只优化传输；缓存不可用不应阻止已经验证的正常下载。
	_ = c.store.cachePackage(ctx, key, archive)
	return release, archive, nil
}
