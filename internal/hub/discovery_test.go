package hub

import (
	"context"
	"fmt"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestDiscoverySchema8Upgrade(t *testing.T) {
	s, session, _, r := backupFixture(t)
	ctx := context.Background()
	root := s.root
	identity := s.RegistryID()
	_, e := s.db.Exec("DROP TABLE evaluation_evidence; DROP TABLE featured_skills; DROP INDEX releases_review_queue; PRAGMA user_version=8;")
	mustGovernance(t, e)
	mustGovernance(t, s.Close())
	s, e = Open(root)
	mustGovernance(t, e)
	defer s.Close()
	if s.RegistryID() != identity {
		t.Fatal("registry identity changed")
	}
	mustGovernance(t, s.SetFeaturedSkill(ctx, session.Identity, "feature-test", r.Namespace, r.Name, true))
	backup := filepath.Join(t.TempDir(), "schema9")
	info, e := s.Backup(ctx, backup)
	mustGovernance(t, e)
	if info.Schema != currentSchema {
		t.Fatal(info)
	}
	dest := filepath.Join(t.TempDir(), "restored")
	_, e = RestoreBackup(ctx, backup, dest)
	mustGovernance(t, e)
	restored, e := Open(dest)
	mustGovernance(t, e)
	defer restored.Close()
	p, e := restored.SearchSkills(ctx, session.Identity, Search{FeaturedOnly: true}, Page{})
	mustGovernance(t, e)
	if len(p.Items) != 1 {
		t.Fatal(p)
	}
}

// 合成大目录仅测试查询/分页，不执行包、不写入用户数据；延迟记录用于比较，不冒充 SLA。
func TestDiscoveryCapacity(t *testing.T) {
	var s *Store
	var e error
	if os.Getenv("SKILLHUB_TEST_POSTGRES_DSN") != "" {
		cfg, _ := postgresFixture(t)
		s, e = OpenDatabase(t.TempDir(), cfg)
		mustGovernance(t, e)
		_, e = s.Bootstrap(context.Background(), "admin", "a-strong-test-password")
		mustGovernance(t, e)
		t.Cleanup(func() { s.Close() })
	} else {
		s = testStore(t)
	}
	ctx := context.Background()
	session, e := s.Login(ctx, "admin", "a-strong-test-password")
	mustGovernance(t, e)
	admin := session.Identity
	reader := governanceUser(t, s, admin, "capacity-reader")
	reviewer := governanceUser(t, s, admin, "capacity-reviewer")
	_, e = s.PutNamespace(ctx, admin, "namespace", PutNamespace{Name: "capacity", ReviewRequired: true})
	mustGovernance(t, e)
	mustGovernance(t, s.SetNamespaceGrant(ctx, admin, "reviewer", SetNamespaceGrant{Namespace: "capacity", Kind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, SubjectID: reviewer.ID, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER, Granted: true}))
	tx, e := s.db.BeginTx(ctx, nil)
	mustGovernance(t, e)
	// 每个技能三版，偶数技能公开，奇数技能仅维护者可读；另有 200 条待审。
	const skills = 2000
	for i := 0; i < skills; i++ {
		for v := 1; v <= 3; v++ {
			visibility := 1
			if i%2 == 1 {
				visibility = 3
			}
			seq := i*3 + v
			_, e = tx.ExecContext(ctx, `INSERT INTO releases(id,namespace,name,version,digest,description,markdown,status,visibility,submitted_by,created_at,published_order) VALUES(?,'capacity',?,?,?,'Capacity fixture','Fixture',2,?,?,?,?)`, fmt.Sprintf("capacity-%d", seq), fmt.Sprintf("skill-%04d", i), fmt.Sprintf("%d.0.0", v), "sha256:fixture", visibility, admin.ID, seq, seq)
			mustGovernance(t, e)
		}
		if i%10 == 0 {
			_, e = tx.ExecContext(ctx, "INSERT INTO featured_skills(namespace,name) VALUES('capacity',?)", fmt.Sprintf("skill-%04d", i))
			mustGovernance(t, e)
		}
	}
	for i := 0; i < 200; i++ {
		_, e = tx.ExecContext(ctx, `INSERT INTO releases(id,namespace,name,version,digest,description,markdown,status,visibility,submitted_by,created_at) VALUES(?,'capacity',?,'1.0.0','fixture','Pending','Pending',1,2,?,?)`, fmt.Sprintf("pending-%d", i), fmt.Sprintf("pending-%d", i), admin.ID, i+1)
		mustGovernance(t, e)
	}
	mustGovernance(t, tx.Commit())
	for _, order := range []taxonomy.CatalogOrder{0, taxonomy.CatalogOrder_CATALOG_ORDER_RECENT} {
		seen := map[string]bool{}
		token := ""
		for {
			p, e := s.SearchSkills(ctx, reader, Search{Order: order}, Page{Size: 73, Token: token})
			mustGovernance(t, e)
			for _, v := range p.Items {
				if seen[v.Name] || v.LatestVersion != "3.0.0" {
					t.Fatal("duplicate or stale", v)
				}
				seen[v.Name] = true
			}
			token = p.Next
			if token == "" {
				break
			}
		}
		if len(seen) != skills/2 {
			t.Fatal("permission-filtered pagination", len(seen))
		}
	}
	var mu sync.Mutex
	var durations []time.Duration
	var wg sync.WaitGroup
	failures := make(chan error, 100)
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				start := time.Now()
				p, e := s.SearchSkills(ctx, reader, Search{Order: taxonomy.CatalogOrder_CATALOG_ORDER_RECENT, FeaturedOnly: i%2 == 0}, Page{Size: 30})
				if e != nil {
					failures <- e
					return
				}
				if len(p.Items) != 30 {
					failures <- fmt.Errorf("unexpected page size %d", len(p.Items))
					return
				}
				q, e := s.ListReviewTasks(ctx, reviewer, Page{Size: 20})
				if e != nil {
					failures <- e
					return
				}
				if q.Total != 200 || len(q.Tasks) != 20 {
					failures <- fmt.Errorf("unexpected task counts")
					return
				}
				mu.Lock()
				durations = append(durations, time.Since(start))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	close(failures)
	for e := range failures {
		t.Error(e)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	if len(durations) > 0 {
		t.Logf("driver=%s skills=%d releases=6200 clients=8 samples=%d search+tasks p50=%s p95=%s max=%s", s.db.Driver, skills, len(durations), durations[len(durations)/2], durations[len(durations)*95/100], durations[len(durations)-1])
	}
}
