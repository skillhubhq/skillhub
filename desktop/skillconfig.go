package main

import "github.com/skillhubhq/skillhub/internal/client"

func (s *DesktopService) ConfigurationTargets() ([]client.ConfigTarget, error) {
	return s.core.ConfigurationTargets(s.ctx)
}
func (s *DesktopService) SkillConfiguration(target string) (client.SkillConfiguration, error) {
	return s.core.SkillConfiguration(s.ctx, target)
}
func (s *DesktopService) SetSkillConfiguration(target, name, mode, value string) error {
	return s.core.SetSkillConfiguration(s.ctx, target, name, mode, value)
}
func (s *DesktopService) ClearSkillConfiguration(target string) error {
	return s.core.ClearSkillConfiguration(s.ctx, target)
}
func (s *DesktopService) UnsetSkillConfiguration(target, name string) error {
	return s.core.UnsetSkillConfiguration(s.ctx, target, name)
}

func (s *DesktopService) EnvironmentInstructions(target, shell string) (string, error) {
	return s.core.EnvironmentInstructions(s.ctx, target, shell)
}

func (s *DesktopService) RenewSkillConfiguration(target, digest string) error {
	return s.core.RenewSkillConfiguration(s.ctx, target, digest)
}
