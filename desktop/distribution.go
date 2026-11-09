package main

import (
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/hub"
)

func (s *DesktopService) CompareReleases(coordinate, before, after string) (hub.ReleaseDifference, error) {
	c, err := client.ParseCoordinate(coordinate, false)
	if err != nil {
		return hub.ReleaseDifference{}, err
	}
	return s.core.CompareReleases(s.ctx, c, before, after)
}
func (s *DesktopService) Subscription(coordinate string) (bool, error) {
	c, err := client.ParseCoordinate(coordinate, false)
	if err != nil {
		return false, err
	}
	return s.core.Subscription(s.ctx, c)
}
func (s *DesktopService) Subscribe(coordinate string, active bool, key string) error {
	c, err := client.ParseCoordinate(coordinate, false)
	if err != nil {
		return err
	}
	return s.core.Subscribe(s.ctx, c, active, key)
}
func (s *DesktopService) Subscriptions(source, token string) (client.SubscriptionPage, error) {
	return s.core.Subscriptions(s.ctx, source, token)
}
func (s *DesktopService) CreateAccessToken(source, name, scope, namespace string, days uint32) (client.CreatedAccessToken, error) {
	return s.core.CreateAccessToken(s.ctx, source, name, scope, namespace, days)
}
func (s *DesktopService) AccessTokens(source string) ([]hub.AccessToken, error) {
	return s.core.AccessTokens(s.ctx, source)
}
func (s *DesktopService) RevokeAccessToken(source, id string) error {
	return s.core.RevokeAccessToken(s.ctx, source, id)
}

func (s *DesktopService) PublishSuite(form client.PublishSuite) (client.ReleaseInfo, error) {
	return s.core.PublishSuite(s.ctx, form)
}
func (s *DesktopService) PlanSuiteInstall(value, tool, scope, project string) (ChangePlan, error) {
	c, err := client.ParseCoordinate(value, true)
	if err != nil {
		return ChangePlan{}, err
	}
	p, err := s.core.PlanSuiteInstall(s.ctx, c, tool, scope, project)
	if err != nil {
		return ChangePlan{}, err
	}
	return s.holdPlan(pendingPlan{suite: p})
}
func (s *DesktopService) PlanSuiteRemove(target string) (ChangePlan, error) {
	p, err := s.core.PlanSuiteRemove(s.ctx, target)
	if err != nil {
		return ChangePlan{}, err
	}
	return s.holdPlan(pendingPlan{suite: p})
}
func (s *DesktopService) Suites() ([]client.SuiteInstallation, error) { return s.store.Suites(s.ctx) }
