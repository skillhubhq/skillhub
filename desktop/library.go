package main

import (
	"errors"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func (s *DesktopService) LocalInventory() (client.LocalInventory, error) {
	return s.core.LocalInventory(s.ctx)
}
func (s *DesktopService) SetLocalRoot(path string, add bool) error {
	return s.core.SetLocalRoot(s.ctx, path, add)
}
func (s *DesktopService) SetLocalHidden(target string, hidden bool) error {
	return s.core.SetLocalHidden(s.ctx, target, hidden)
}
func (s *DesktopService) ReadLocalSkill(target, path string) (client.LocalDocument, error) {
	return s.core.ReadLocalSkill(s.ctx, target, path)
}
func (s *DesktopService) LocalBackups() ([]client.LocalBackup, error) {
	return s.store.LocalBackups(s.ctx)
}
func (s *DesktopService) PlanLocalSource(target, coordinate string, replace bool) (ChangePlan, error) {
	coord, e := client.ParseCoordinate(coordinate, true)
	if e != nil {
		return ChangePlan{}, e
	}
	p, e := s.core.PlanLocalSource(s.ctx, target, coord, replace)
	if e != nil {
		return ChangePlan{}, e
	}
	return s.holdPlan(pendingPlan{local: &p})
}
func (s *DesktopService) PlanLocalEdit(target, path, fingerprint, content string) (ChangePlan, error) {
	p, e := s.core.PlanLocalEdit(s.ctx, target, path, fingerprint, content)
	if e != nil {
		return ChangePlan{}, e
	}
	return s.holdPlan(pendingPlan{local: &p})
}
func (s *DesktopService) PlanLocalAction(target, action, destination string) (ChangePlan, error) {
	var p client.LocalPlan
	var e error
	switch action {
	case "detach":
		p, e = s.core.PlanLocalDetach(s.ctx, target)
	case "remove":
		p, e = s.core.PlanLocalRemove(s.ctx, target)
	case "restore":
		p, e = s.core.PlanLocalRestore(s.ctx, target)
	case "copy":
		p, e = s.core.PlanLocalCopy(s.ctx, target, destination)
	default:
		e = errors.New("unknown local action")
	}
	if e != nil {
		return ChangePlan{}, e
	}
	return s.holdPlan(pendingPlan{local: &p})
}

func (s *DesktopService) InspectInstallDestination(tool, scope, project, name string) (string, error) {
	return s.core.ExistingLocalTarget(tool, scope, project, name)
}

func (s *DesktopService) RevealLocalSkill(target string) error {
	item, err := s.core.LocalSkill(s.ctx, target)
	if err != nil {
		return err
	}
	app := application.Get()
	if app == nil {
		return errors.New("desktop application unavailable")
	}
	return app.Env.OpenFileManager(item.Target, false)
}

func (s *DesktopService) DeleteLocalBackup(id string) error {
	return s.store.DeleteLocalBackup(s.ctx, id)
}
