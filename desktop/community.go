package main

import taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
import c "github.com/skillhubhq/skillhub/internal/community"

func (s *DesktopService) CommunityGetProfile(source string, id string) (c.Profile, error) {
	return s.core.CommunityGetProfile(s.ctx, source, id)
}
func (s *DesktopService) CommunityUpdateProfile(source string, name, bio string, revision uint64, key string) (c.Profile, error) {
	return s.core.CommunityUpdateProfile(s.ctx, source, name, bio, revision, key)
}
func (s *DesktopService) CommunityGetTeam(source string, namespace string) (c.Team, error) {
	return s.core.CommunityGetTeam(s.ctx, source, namespace)
}
func (s *DesktopService) CommunityUpdateTeam(source string, namespace, name, bio string, revision uint64, key string) (c.Team, error) {
	return s.core.CommunityUpdateTeam(s.ctx, source, namespace, name, bio, revision, key)
}
func (s *DesktopService) CommunityListTeams(source string, token string) (c.Page[c.Team], error) {
	return s.core.CommunityListTeams(s.ctx, source, token)
}
func (s *DesktopService) CommunityListMembers(source string, namespace, token string) (c.Page[c.Member], error) {
	return s.core.CommunityListMembers(s.ctx, source, namespace, token)
}
func (s *DesktopService) CommunityListContributions(source string, id, namespace, token string) (c.Page[c.Contribution], error) {
	return s.core.CommunityListContributions(s.ctx, source, id, namespace, token)
}
func (s *DesktopService) CommunityGetDiscussion(source string, id string) (c.Discussion, error) {
	return s.core.CommunityGetDiscussion(s.ctx, source, id)
}
func (s *DesktopService) CommunityListComments(source string, id, parent, token string) (c.Page[c.Comment], error) {
	return s.core.CommunityListComments(s.ctx, source, id, parent, token)
}
func (s *DesktopService) CommunityCreateComment(source string, id, parent, body, key string) (c.Comment, error) {
	return s.core.CommunityCreateComment(s.ctx, source, id, parent, body, key)
}
func (s *DesktopService) CommunityDeleteComment(source string, id, key string) (bool, error) {
	return s.core.CommunityDeleteComment(s.ctx, source, id, key)
}
func (s *DesktopService) CommunitySetReaction(source string, id string, bookmark, active bool, key string) (c.Discussion, error) {
	return s.core.CommunitySetReaction(s.ctx, source, id, bookmark, active, key)
}
func (s *DesktopService) CommunityListBookmarks(source string, token string) (c.Page[c.Contribution], error) {
	return s.core.CommunityListBookmarks(s.ctx, source, token)
}

func (s *DesktopService) CommunityUpdateAvatar(source string, image []byte, revision uint64, key string) (c.Profile, error) {
	return s.core.CommunityUpdateAvatar(s.ctx, source, image, revision, key)
}

func (s *DesktopService) CommunityParticipation(source string) (c.Participation, error) {
	return s.core.CommunityParticipation(s.ctx, source)
}
func (s *DesktopService) CommunityReport(source, id, comment, reason, key string) (string, error) {
	return s.core.CommunityReport(s.ctx, source, id, comment, reason, key)
}
func (s *DesktopService) CommunityReports(source string, resolved bool, token string) (c.Page[c.Report], error) {
	return s.core.CommunityReports(s.ctx, source, resolved, token)
}
func (s *DesktopService) CommunityResolveReport(source, id string, action taxonomy.ReportResolution, note, key string) (bool, error) {
	return s.core.CommunityResolveReport(s.ctx, source, id, action, note, key)
}
func (s *DesktopService) CommunitySite(source string) (c.Site, error) {
	return s.core.CommunitySite(s.ctx, source)
}
