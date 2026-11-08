package main

import (
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/client"
)

func (s *DesktopService) PlatformAccess(source, id string) (client.PlatformAccess, error) {
	return s.core.PlatformAccess(s.ctx, source, id)
}
func (s *DesktopService) SetPlatformRole(source, id string, role taxonomy.PlatformRole, granted bool, key string) error {
	return s.core.SetPlatformRole(s.ctx, source, id, role, granted, key)
}
func (s *DesktopService) ScanReport(source, id string) (client.ScanInfo, error) {
	return s.core.ScanReport(s.ctx, source, id)
}
func (s *DesktopService) ScanRelease(source, id, key string) (client.ScanInfo, error) {
	return s.core.ScanRelease(s.ctx, source, id, key)
}
func (s *DesktopService) Notifications(source, token string) (client.NotificationPage, error) {
	return s.core.Notifications(s.ctx, source, token)
}
func (s *DesktopService) MarkNotificationRead(source, id string) error {
	return s.core.MarkNotificationRead(s.ctx, source, id)
}

func (s *DesktopService) ReviewTasks(source, token string) (client.ReviewTaskPage, error) {
	return s.core.ReviewTasks(s.ctx, source, token)
}
func (s *DesktopService) SetFeaturedSkill(source, namespace, name string, featured bool, key string) error {
	return s.core.SetFeaturedSkill(s.ctx, source, namespace, name, featured, key)
}

func (s *DesktopService) MarkAllNotificationsRead(source string) error {
	return s.core.MarkAllNotificationsRead(s.ctx, source)
}
