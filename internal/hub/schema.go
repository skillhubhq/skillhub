package hub

// 当前完整结构；初始化在同一事务中完成。
const initialSchema = `
CREATE TABLE instance (id TEXT PRIMARY KEY, secret BLOB NOT NULL);
CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT NOT NULL UNIQUE, password_hash BLOB NOT NULL, administrator INTEGER NOT NULL, disabled INTEGER NOT NULL DEFAULT 0);
CREATE TABLE sessions (token_hash TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id), expires_at INTEGER NOT NULL);
CREATE INDEX sessions_user ON sessions(user_id);
CREATE TABLE devices (device_hash TEXT PRIMARY KEY, user_code TEXT NOT NULL UNIQUE, expires_at INTEGER NOT NULL, user_id TEXT REFERENCES users(id), last_poll INTEGER NOT NULL DEFAULT 0);
CREATE TABLE audit (sequence INTEGER PRIMARY KEY AUTOINCREMENT, actor_id TEXT NOT NULL, action TEXT NOT NULL, resource TEXT NOT NULL, created_at INTEGER NOT NULL, detail TEXT NOT NULL DEFAULT '');

CREATE TABLE groups (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE);
CREATE TABLE group_members (group_id TEXT NOT NULL REFERENCES groups(id), user_id TEXT NOT NULL REFERENCES users(id), PRIMARY KEY(group_id,user_id));
CREATE TABLE namespaces (name TEXT PRIMARY KEY, review_required INTEGER NOT NULL, revision INTEGER NOT NULL, archived INTEGER NOT NULL DEFAULT 0);
CREATE TABLE namespace_tombstones (name TEXT PRIMARY KEY, deleted_at INTEGER NOT NULL);
CREATE TABLE release_tombstones (id TEXT PRIMARY KEY, namespace TEXT NOT NULL, name TEXT NOT NULL, version TEXT NOT NULL, digest TEXT NOT NULL, deleted_at INTEGER NOT NULL, UNIQUE(namespace,name,version));
CREATE TABLE package_cleanup (digest TEXT PRIMARY KEY);
CREATE TABLE namespace_grants (namespace TEXT NOT NULL REFERENCES namespaces(name), kind INTEGER NOT NULL CHECK(kind IN (1,2)), subject_id TEXT NOT NULL, role INTEGER NOT NULL CHECK(role IN (1,2,3,4)), PRIMARY KEY(namespace,kind,subject_id,role));
CREATE TABLE categories (name TEXT PRIMARY KEY, title TEXT NOT NULL, archived INTEGER NOT NULL, revision INTEGER NOT NULL);
CREATE TABLE releases (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT NOT NULL UNIQUE,
 namespace TEXT NOT NULL REFERENCES namespaces(name), name TEXT NOT NULL, version TEXT NOT NULL,
 digest TEXT NOT NULL, archive_bytes INTEGER NOT NULL DEFAULT 0,
 display_name_en TEXT NOT NULL DEFAULT '', display_name_zh TEXT NOT NULL DEFAULT '', icon INTEGER NOT NULL DEFAULT 0,
 description TEXT NOT NULL, markdown TEXT NOT NULL, provenance TEXT NOT NULL DEFAULT 'null',
 status INTEGER NOT NULL CHECK(status IN (1,2,3,4)), visibility INTEGER NOT NULL CHECK(visibility IN (1,2,3)),
 submitted_by TEXT NOT NULL REFERENCES users(id), created_at INTEGER NOT NULL,
 review_reason TEXT NOT NULL DEFAULT '', category TEXT NOT NULL DEFAULT '', published_order INTEGER NOT NULL DEFAULT 0,
 UNIQUE(namespace,name,version)
);
CREATE INDEX releases_catalog ON releases(status,namespace,name,published_order DESC);
CREATE TABLE release_groups (release_id TEXT NOT NULL REFERENCES releases(id), group_id TEXT NOT NULL REFERENCES groups(id), PRIMARY KEY(release_id,group_id));
CREATE TABLE release_tags (release_id TEXT NOT NULL REFERENCES releases(id), tag TEXT NOT NULL, PRIMARY KEY(release_id,tag));
CREATE INDEX release_tags_name ON release_tags(tag,release_id);
CREATE TABLE mutations (actor TEXT NOT NULL REFERENCES users(id), method TEXT NOT NULL, key TEXT NOT NULL, fingerprint TEXT NOT NULL, result BLOB NOT NULL, PRIMARY KEY(actor,method,key));

CREATE TABLE profiles (user_id TEXT PRIMARY KEY REFERENCES users(id), display_name TEXT NOT NULL DEFAULT '', bio TEXT NOT NULL DEFAULT '', avatar TEXT NOT NULL DEFAULT '', revision INTEGER NOT NULL DEFAULT 0);
CREATE TABLE team_profiles (namespace TEXT PRIMARY KEY REFERENCES namespaces(name), display_name TEXT NOT NULL DEFAULT '', bio TEXT NOT NULL DEFAULT '', revision INTEGER NOT NULL DEFAULT 0);
CREATE TABLE comments (sequence INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT NOT NULL UNIQUE, release_id TEXT NOT NULL REFERENCES releases(id), parent_id TEXT REFERENCES comments(id), author_id TEXT NOT NULL REFERENCES users(id), body TEXT NOT NULL, created_at INTEGER NOT NULL, deleted INTEGER NOT NULL DEFAULT 0);
CREATE INDEX comments_thread ON comments(release_id,parent_id,sequence);
CREATE TABLE reactions (user_id TEXT NOT NULL REFERENCES users(id), release_id TEXT NOT NULL REFERENCES releases(id), bookmark INTEGER NOT NULL CHECK(bookmark IN (0,1)), PRIMARY KEY(user_id,release_id,bookmark));
CREATE INDEX reactions_release ON reactions(release_id,bookmark);

CREATE TABLE access_requests (user_id TEXT PRIMARY KEY REFERENCES users(id), created_at INTEGER NOT NULL);

CREATE TABLE platform_roles (user_id TEXT NOT NULL REFERENCES users(id), role INTEGER NOT NULL CHECK(role IN (1,2)), PRIMARY KEY(user_id,role));
CREATE TABLE release_scans (sequence INTEGER PRIMARY KEY AUTOINCREMENT, release_id TEXT NOT NULL REFERENCES releases(id), digest TEXT NOT NULL, report TEXT NOT NULL, created_at INTEGER NOT NULL);
CREATE INDEX release_scans_release ON release_scans(release_id,sequence);
CREATE TABLE review_notifications (sequence INTEGER PRIMARY KEY AUTOINCREMENT, recipient TEXT NOT NULL REFERENCES users(id), release_id TEXT NOT NULL REFERENCES releases(id), approved INTEGER NOT NULL, is_read INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL, UNIQUE(recipient,release_id));
CREATE INDEX review_notifications_recipient ON review_notifications(recipient,sequence);

CREATE TABLE featured_skills (namespace TEXT NOT NULL REFERENCES namespaces(name), name TEXT NOT NULL, PRIMARY KEY(namespace,name));
CREATE INDEX releases_review_queue ON releases(status,sequence);

CREATE TABLE evaluation_evidence (sequence INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT NOT NULL UNIQUE, release_id TEXT NOT NULL REFERENCES releases(id), digest TEXT NOT NULL, metadata TEXT NOT NULL, document TEXT NOT NULL, retracted INTEGER NOT NULL, UNIQUE(release_id,digest));
CREATE INDEX evaluation_release ON evaluation_evidence(release_id,sequence);

CREATE TABLE personal_namespaces (user_id TEXT PRIMARY KEY REFERENCES users(id), namespace TEXT NOT NULL UNIQUE REFERENCES namespaces(name));

CREATE INDEX releases_author_time ON releases(submitted_by,created_at);
CREATE TABLE community_reports (sequence INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT NOT NULL UNIQUE, reporter TEXT NOT NULL REFERENCES users(id), release_id TEXT NOT NULL REFERENCES releases(id), comment_id TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL, resolved INTEGER NOT NULL DEFAULT 0, resolution TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, resolved_at INTEGER NOT NULL DEFAULT 0, UNIQUE(reporter,release_id,comment_id));
CREATE INDEX community_reports_queue ON community_reports(resolved,sequence);

CREATE TABLE external_identities (issuer TEXT NOT NULL, subject TEXT NOT NULL, user_id TEXT NOT NULL UNIQUE REFERENCES users(id), PRIMARY KEY(issuer,subject));
PRAGMA user_version=12;
`
