package hub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/evaluation"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/skillscan"
)

type EvaluationEvidence struct {
	ID           string `json:"id"`
	ReleaseID    string `json:"release_id"`
	Digest       string `json:"digest"`
	SuiteName    string `json:"suite_name"`
	SuiteVersion string `json:"suite_version"`
	SuiteDigest  string `json:"suite_digest"`
	Runner       string `json:"runner"`
	Agent        string `json:"agent"`
	Model        string `json:"model"`
	ExecutedAt   string `json:"executed_at"`
	ImportedAt   string `json:"imported_at"`
	evaluation.Summary
	Retracted    bool   `json:"retracted"`
	DocumentJSON string `json:"document_json"`
}
type EvaluationPage struct {
	Evidence  []EvaluationEvidence `json:"evidence"`
	CanManage bool                 `json:"can_manage"`
}

// 发布版本沿用内容受众；未发布或撤下版本仅允许履职角色访问。
func evaluationAccess(ctx context.Context, tx *hubdb.Tx, u Identity, id string) (Release, error) {
	args := append(principalArgs(u), sql.Named("release", id))
	r, err := scanRelease(tx.QueryRowContext(ctx, `SELECT `+releaseColumns+` FROM releases r WHERE r.id=@release AND ((r.status=2 AND `+readableSQL+`) OR (r.status<>2 AND `+manageableSQL+`))`, args...))
	return r, assertExists(err)
}
func evaluationManager(ctx context.Context, tx *hubdb.Tx, u Identity, id string) (Release, error) {
	r, err := releaseByID(ctx, tx, id)
	if err != nil {
		return r, err
	}
	if err = requireRole(ctx, tx, u, r.Namespace, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER); err != nil {
		return Release{}, ErrNotFound
	}
	return r, nil
}
func (s *Store) ImportEvaluation(ctx context.Context, actor Identity, key, id string, raw []byte) (string, error) {
	report, err := evaluation.Parse(raw)
	if err != nil {
		return "", ErrInvalid
	}
	executed, _ := time.Parse(time.RFC3339, report.ExecutedAt)
	if executed.After(s.now().Add(5 * time.Minute)) {
		return "", ErrInvalid
	}
	canonical := report.Canonical()
	if len(canonical) > evaluation.MaxBytes {
		return "", ErrInvalid
	}
	digest := packageformat.Digest(canonical)
	scan, err := skillscan.Scan(ctx, &packageformat.Bundle{Digest: digest, Files: []packageformat.File{{Path: "evaluation.json", Data: canonical}}})
	if err != nil {
		return "", err
	}
	if scan.Blocking {
		return "", ErrInvalid
	}
	cmd := struct{ ReleaseID, Digest string }{id, digest}
	return mutate(s, ctx, actor, "evaluation.import", key, cmd, func(tx *hubdb.Tx, u Identity) error {
		r, e := evaluationManager(ctx, tx, u, id)
		if e != nil {
			return e
		}
		if r.Digest != report.SkillDigest {
			return ErrConflict
		}
		return nil
	}, func(tx *hubdb.Tx, u Identity) (string, error) {
		var existing string
		err := tx.QueryRowContext(ctx, "SELECT id FROM evaluation_evidence WHERE release_id=? AND digest=?", id, digest).Scan(&existing)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		var count int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM evaluation_evidence WHERE release_id=?", id).Scan(&count); err != nil {
			return "", err
		}
		if count >= 100 {
			return "", ErrConflict
		}
		e := EvaluationEvidence{ID: uuid.NewString(), ReleaseID: id, Digest: digest, SuiteName: report.Suite.Name, SuiteVersion: report.Suite.Version, SuiteDigest: report.SuiteDigest(), Runner: report.Runner, Agent: report.Agent, Model: report.Model, ExecutedAt: report.ExecutedAt, ImportedAt: s.now().UTC().Format(time.RFC3339), Summary: report.Summary()}
		metadata, _ := json.Marshal(e)
		if _, err = tx.ExecContext(ctx, "INSERT INTO evaluation_evidence(id,release_id,digest,metadata,document,retracted) VALUES(?,?,?,?,?,0)", e.ID, id, digest, string(metadata), string(canonical)); err != nil {
			return "", err
		}
		return e.ID, s.auditChange(ctx, tx, u.ID, "evaluation.import", e.ID, cmd)
	})
}
func scanEvidence(row scanner) (EvaluationEvidence, error) {
	var e EvaluationEvidence
	var metadata string
	if err := row.Scan(&metadata, &e.DocumentJSON, &e.Retracted); err != nil {
		return e, assertExists(err)
	}
	document, retracted := e.DocumentJSON, e.Retracted
	if err := json.Unmarshal([]byte(metadata), &e); err != nil {
		return e, err
	}
	e.DocumentJSON = document
	e.Retracted = retracted
	return e, nil
}
func (s *Store) ListEvaluations(ctx context.Context, actor Identity, id string) (EvaluationPage, error) {
	out := EvaluationPage{Evidence: []EvaluationEvidence{}}
	tx, u, err := s.read(ctx, actor)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err = evaluationAccess(ctx, tx, u, id); err != nil {
		return out, err
	}
	_, err = evaluationManager(ctx, tx, u, id)
	out.CanManage = err == nil
	rows, err := tx.QueryContext(ctx, "SELECT metadata,'' AS document,retracted FROM evaluation_evidence WHERE release_id=? ORDER BY sequence DESC LIMIT 100", id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		e, err := scanEvidence(rows)
		if err != nil {
			return out, err
		}
		out.Evidence = append(out.Evidence, e)
	}
	return out, rows.Err()
}
func (s *Store) GetEvaluation(ctx context.Context, actor Identity, id string) (EvaluationEvidence, error) {
	tx, u, err := s.read(ctx, actor)
	if err != nil {
		return EvaluationEvidence{}, err
	}
	defer tx.Rollback()
	var release string
	if err = tx.QueryRowContext(ctx, "SELECT release_id FROM evaluation_evidence WHERE id=?", id).Scan(&release); err != nil {
		return EvaluationEvidence{}, assertExists(err)
	}
	if _, err = evaluationAccess(ctx, tx, u, release); err != nil {
		return EvaluationEvidence{}, err
	}
	return scanEvidence(tx.QueryRowContext(ctx, "SELECT metadata,document,retracted FROM evaluation_evidence WHERE id=?", id))
}
func (s *Store) RetractEvaluation(ctx context.Context, actor Identity, key, id string) error {
	_, err := mutate(s, ctx, actor, "evaluation.retract", key, struct{ ID string }{id}, func(tx *hubdb.Tx, u Identity) error {
		var release string
		if err := tx.QueryRowContext(ctx, "SELECT release_id FROM evaluation_evidence WHERE id=?", id).Scan(&release); err != nil {
			return assertExists(err)
		}
		_, err := evaluationManager(ctx, tx, u, release)
		return err
	}, func(tx *hubdb.Tx, u Identity) (struct{}, error) {
		e, err := scanEvidence(tx.QueryRowContext(ctx, "SELECT metadata,'' AS document,retracted FROM evaluation_evidence WHERE id=?", id))
		if err != nil {
			return struct{}{}, err
		}
		if e.Retracted {
			return struct{}{}, nil
		}
		// 正文及其派生文字一并清除；幂等记录仅持有标识和摘要。
		tombstone := EvaluationEvidence{ID: e.ID, ReleaseID: e.ReleaseID, Digest: e.Digest, ImportedAt: e.ImportedAt, Retracted: true}
		metadata, _ := json.Marshal(tombstone)
		if _, err = tx.ExecContext(ctx, "UPDATE evaluation_evidence SET document='',metadata=?,retracted=1 WHERE id=?", string(metadata), id); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, audit(ctx, tx, u.ID, "evaluation.retract", id, s.now())
	})
	return err
}
