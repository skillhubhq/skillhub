package hub

import (
	"context"
	"errors"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/evaluation"
	"path/filepath"
	"strings"
	"testing"
)

func evaluationFixture(digest string) evaluation.Report {
	return evaluation.Report{SchemaVersion: "skillhub.evaluation.v1", SkillDigest: digest, Suite: evaluation.Suite{SchemaVersion: "skillhub.cases.v1", Name: "review-check", Version: "1", Cases: []evaluation.Case{{ID: "missing-input", Input: "No change provided", Expected: "Ask for the change"}}}, Agent: "example-agent/1", Model: "example-model/1", Runner: "manual/1", Settings: "tools disabled", ExecutedAt: "2026-01-01T00:00:00Z", Results: []evaluation.Result{{CaseID: "missing-input", Outcome: evaluation.Outcome{Status: "pass", Observed: "Asked for input"}, Baseline: &evaluation.Outcome{Status: "fail", Observed: "Guessed"}}}}
}
func TestEvaluationVersionPermissionsRetractionAndBackup(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	login, e := s.Login(ctx, "admin", "a-strong-test-password")
	mustGovernance(t, e)
	admin := login.Identity
	outsider := governanceUser(t, s, admin, "outsider")
	maintainer := governanceUser(t, s, admin, "maintainer")
	reviewer := governanceUser(t, s, admin, "reviewer")
	_, e = s.PutNamespace(ctx, admin, "namespace", PutNamespace{Name: "team", ReviewRequired: true})
	mustGovernance(t, e)
	for name, grant := range map[string]SetNamespaceGrant{"maintain": {Namespace: "team", Kind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, SubjectID: maintainer.ID, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, Granted: true}, "reviewer": {Namespace: "team", Kind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, SubjectID: reviewer.ID, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER, Granted: true}} {
		mustGovernance(t, s.SetNamespaceGrant(ctx, admin, "grant-"+name, grant))
	}
	r, e := s.SubmitRelease(ctx, maintainer, "publish-test", SubmitRelease{Namespace: "team", Version: "1.0.0", Archive: governanceArchive(t, "Review inputs."), Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC})
	mustGovernance(t, e)
	report := evaluationFixture(r.Digest)
	unsafeReport := evaluationFixture(r.Digest)
	unsafeReport.Results[0].Observed = "ghp_" + strings.Repeat("A", 40)
	if _, err := s.ImportEvaluation(ctx, maintainer, "unsafe-report", r.ID, unsafeReport.Canonical()); !errors.Is(err, ErrInvalid) {
		t.Fatal("credential-shaped evidence was accepted", err)
	}
	empty, err := s.ListEvaluations(ctx, maintainer, r.ID)
	mustGovernance(t, err)
	if len(empty.Evidence) != 0 {
		t.Fatal("rejected evidence was persisted")
	}
	raw := report.Canonical()
	for _, u := range []Identity{outsider, reviewer, {}} {
		if _, err := s.ImportEvaluation(ctx, u, "invalid-import", r.ID, raw); err == nil {
			t.Fatal("unauthorized import")
		}
	}
	wrong := report
	wrong.SkillDigest = "sha256:" + strings.Repeat("0", 64)
	if _, e = s.ImportEvaluation(ctx, maintainer, "wrong-digest", r.ID, wrong.Canonical()); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	id, e := s.ImportEvaluation(ctx, maintainer, "import-test", r.ID, raw)
	mustGovernance(t, e)
	duplicate, e := s.ImportEvaluation(ctx, maintainer, "different-key", r.ID, raw)
	mustGovernance(t, e)
	if duplicate != id {
		t.Fatal("duplicate evidence")
	}
	for _, u := range []Identity{outsider, {}} {
		if _, e = s.GetEvaluation(ctx, u, id); !errors.Is(e, ErrNotFound) {
			t.Fatal("pending evidence leaked", e)
		}
	}
	p, e := s.ListEvaluations(ctx, reviewer, r.ID)
	mustGovernance(t, e)
	if p.CanManage || len(p.Evidence) != 1 || p.Evidence[0].DocumentJSON != "" {
		t.Fatal("invalid reviewer summary")
	}
	if _, e = s.ReviewRelease(ctx, maintainer, "self-review", ReviewRelease{ReleaseID: r.ID, Approve: true}); !errors.Is(e, ErrForbidden) {
		t.Fatal("evidence bypassed independent review")
	}
	_, e = s.ReviewRelease(ctx, reviewer, "review-release", ReviewRelease{ReleaseID: r.ID, Approve: true})
	mustGovernance(t, e)
	full, e := s.GetEvaluation(ctx, Identity{}, id)
	mustGovernance(t, e)
	if full.DocumentJSON == "" || full.Passed != 1 || full.BaselineTotal != 1 {
		t.Fatal("missing public evidence")
	}
	mustGovernance(t, s.SetNamespaceGrant(ctx, admin, "revoke-maintainer", SetNamespaceGrant{Namespace: "team", Kind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, SubjectID: maintainer.ID, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, Granted: false}))
	if _, e = s.ImportEvaluation(ctx, maintainer, "import-test", r.ID, raw); e == nil {
		t.Fatal("replay bypassed revocation")
	}
	backup := filepath.Join(t.TempDir(), "backup")
	_, e = s.Backup(ctx, backup)
	mustGovernance(t, e)
	dest := filepath.Join(t.TempDir(), "restored")
	_, e = RestoreBackup(ctx, backup, dest)
	mustGovernance(t, e)
	restored, e := Open(dest)
	mustGovernance(t, e)
	defer restored.Close()
	copy, e := restored.GetEvaluation(ctx, Identity{}, id)
	mustGovernance(t, e)
	if copy.DocumentJSON != full.DocumentJSON {
		t.Fatal("backup lost evidence")
	}
	mustGovernance(t, s.RetractEvaluation(ctx, admin, "retract-report", id))
	full, e = s.GetEvaluation(ctx, Identity{}, id)
	mustGovernance(t, e)
	if !full.Retracted || full.DocumentJSON != "" || full.SuiteName != "" || full.Agent != "" {
		t.Fatal("retracted content leaked")
	}
	replay, e := s.ImportEvaluation(ctx, admin, "import-admin", r.ID, raw)
	mustGovernance(t, e)
	if replay != id {
		t.Fatal("resurrected report")
	}
	_, e = s.YankRelease(ctx, admin, "yank-release", YankRelease{ReleaseID: r.ID, Reason: "Fixture withdrawal"})
	mustGovernance(t, e)
	if _, e = s.GetEvaluation(ctx, Identity{}, id); !errors.Is(e, ErrNotFound) {
		t.Fatal("withdrawn evidence leaked", e)
	}
}
