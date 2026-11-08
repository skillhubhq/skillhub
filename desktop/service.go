package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// DesktopService 只适配桌面生命周期与交互；业务状态由共享 Go 内核拥有。
type DesktopService struct {
	store        *client.Store
	core         *client.Client
	ctx          context.Context
	options      client.ClientOptions
	officialOnce sync.Once
	mu           sync.Mutex
	plans        map[string]pendingPlan
	logins       map[string]*pendingLogin
}
type pendingPlan struct {
	install *client.InstallPlan
	project *client.ProjectRestorePlan
	archive []byte
	expires time.Time
}
type ChangePlan struct {
	Handle    string               `json:"handle"`
	Changes   []client.InstallPlan `json:"changes"`
	ExpiresAt string               `json:"expires_at"`
}

func (s *DesktopService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	root, err := client.DataDir()
	if err != nil {
		return err
	}
	store, err := client.OpenStore(root)
	if err != nil {
		return err
	}
	if err = store.Recover(ctx); err != nil {
		_ = store.Close()
		return err
	}
	s.store = store
	s.core = client.NewClient(store, s.options)
	s.ctx = ctx
	s.plans = map[string]pendingPlan{}
	s.logins = map[string]*pendingLogin{}
	return nil
}
func (s *DesktopService) ServiceShutdown() error {
	if s.store != nil {
		return s.store.Close()
	}
	return nil
}
func (s *DesktopService) ListInstalled() ([]client.Installation, error) {
	return s.store.ListInstallations(s.ctx)
}
func (s *DesktopService) CacheInfo() (client.CacheInfo, error)  { return s.store.CacheInfo(s.ctx) }
func (s *DesktopService) ClearCache() (client.CacheInfo, error) { return s.store.ClearCache(s.ctx) }
func (s *DesktopService) NewRequestID() string                  { return uuid.NewString() }
func (s *DesktopService) ChooseDirectory() (string, error) {
	app := application.Get()
	if app == nil {
		return "", errors.New("native application is unavailable")
	}
	return app.Dialog.OpenFile().SetTitle("Choose a skill or project folder").CanChooseFiles(false).CanChooseDirectories(true).PromptForSingleSelection()
}
func (s *DesktopService) holdPlan(p pendingPlan) (ChangePlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, old := range s.plans {
		if time.Now().After(old.expires) {
			delete(s.plans, id)
		}
	}
	if len(s.plans) >= 4 {
		return ChangePlan{}, errors.New("close an existing change preview before creating another")
	}
	id := uuid.NewString()
	p.expires = time.Now().Add(5 * time.Minute)
	s.plans[id] = p
	changes := []client.InstallPlan{}
	if p.install != nil {
		changes = append(changes, *p.install)
	} else {
		changes = p.project.Plans
	}
	return ChangePlan{Handle: id, Changes: changes, ExpiresAt: p.expires.UTC().Format(time.RFC3339)}, nil
}
func (s *DesktopService) DiscardPlan(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.plans, id)
}
func (s *DesktopService) ApplyPlan(id string) ([]client.ProjectRestoreResult, error) {
	s.mu.Lock()
	p, ok := s.plans[id]
	delete(s.plans, id)
	s.mu.Unlock()
	if !ok || time.Now().After(p.expires) {
		return nil, errors.New("change preview expired; create a new plan")
	}
	if p.project != nil {
		return s.core.ApplyProjectRestore(s.ctx, p.project)
	}
	if err := s.store.Apply(s.ctx, *p.install, p.archive); err != nil {
		return nil, err
	}
	return []client.ProjectRestoreResult{{Target: p.install.Target, Applied: true}}, nil
}
func (s *DesktopService) PlanInstall(coordinate, tool, scope, project string) (ChangePlan, error) {
	c, err := client.ParseCoordinate(coordinate, true)
	if err != nil {
		return ChangePlan{}, err
	}
	plan, archive, err := s.core.PlanRemoteInstall(s.ctx, c, tool, scope, project)
	if err != nil {
		return ChangePlan{}, err
	}
	return s.holdPlan(pendingPlan{install: plan, archive: archive})
}
func (s *DesktopService) PlanUpdate(target, version string) (ChangePlan, error) {
	plan, archive, err := s.core.PlanRemoteUpdate(s.ctx, target, version)
	if err != nil {
		return ChangePlan{}, err
	}
	return s.holdPlan(pendingPlan{install: plan, archive: archive})
}
func (s *DesktopService) PlanUpdateFrom(target, version, source string) (ChangePlan, error) {
	plan, archive, err := s.core.PlanRemoteUpdateFrom(s.ctx, target, version, source)
	if err != nil {
		return ChangePlan{}, err
	}
	return s.holdPlan(pendingPlan{install: plan, archive: archive})
}
func (s *DesktopService) CheckUpdatesFrom(target, source string) ([]client.UpdateCheck, error) {
	return s.core.CheckUpdatesFrom(s.ctx, target, source)
}
func (s *DesktopService) PlanRemove(target string) (ChangePlan, error) {
	plan, err := s.store.PlanRemove(s.ctx, target)
	if err != nil {
		return ChangePlan{}, err
	}
	return s.holdPlan(pendingPlan{install: plan})
}
func (s *DesktopService) CheckUpdates() ([]client.UpdateCheck, error) {
	return s.core.CheckUpdates(s.ctx, "")
}
func (s *DesktopService) ListProjects() ([]string, error) { return s.core.ListProjects(s.ctx) }
func (s *DesktopService) OpenProject(path string, create bool) (client.ProjectState, error) {
	if create {
		return s.core.InitProject(s.ctx, path)
	}
	return s.core.OpenProject(s.ctx, path)
}
func (s *DesktopService) ForgetProject(path string) error { return s.core.ForgetProject(s.ctx, path) }
func (s *DesktopService) SetProjectSkill(path string, skill client.ProjectSkill) (client.ProjectState, error) {
	return s.core.SetProjectSkill(s.ctx, path, skill)
}
func (s *DesktopService) RemoveProjectSkill(path, tool, name string) (client.ProjectState, error) {
	return s.core.RemoveProjectSkill(s.ctx, path, tool, name)
}
func (s *DesktopService) LockProject(path string) (client.ProjectState, error) {
	if _, err := s.core.LockProject(s.ctx, path); err != nil {
		return client.ProjectState{}, err
	}
	return s.core.ReadProject(s.ctx, path)
}
func (s *DesktopService) PlanRestore(path string) (ChangePlan, error) {
	plan, err := s.core.PlanProjectRestore(s.ctx, path)
	if err != nil {
		return ChangePlan{}, err
	}
	return s.holdPlan(pendingPlan{project: plan})
}
