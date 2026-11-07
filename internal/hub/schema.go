package hub

const registrySchema = `
CREATE TABLE groups (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE);
CREATE TABLE group_members (group_id TEXT NOT NULL REFERENCES groups(id), user_id TEXT NOT NULL REFERENCES users(id), PRIMARY KEY(group_id,user_id));
CREATE TABLE namespaces (name TEXT PRIMARY KEY, review_required INTEGER NOT NULL, revision INTEGER NOT NULL);
CREATE TABLE namespace_grants (namespace TEXT NOT NULL REFERENCES namespaces(name), kind INTEGER NOT NULL CHECK(kind IN (1,2)), subject_id TEXT NOT NULL, role INTEGER NOT NULL CHECK(role IN (1,2,3,4)), PRIMARY KEY(namespace,kind,subject_id,role));
CREATE TABLE categories (name TEXT PRIMARY KEY, title TEXT NOT NULL, archived INTEGER NOT NULL, revision INTEGER NOT NULL);
CREATE TABLE releases (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT NOT NULL UNIQUE,
 namespace TEXT NOT NULL REFERENCES namespaces(name), name TEXT NOT NULL, version TEXT NOT NULL,
 digest TEXT NOT NULL, description TEXT NOT NULL, markdown TEXT NOT NULL,
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
ALTER TABLE audit ADD COLUMN detail TEXT NOT NULL DEFAULT '';
PRAGMA user_version=2;
`

const communitySchema = `
CREATE TABLE profiles (user_id TEXT PRIMARY KEY REFERENCES users(id), display_name TEXT NOT NULL DEFAULT '', bio TEXT NOT NULL DEFAULT '', revision INTEGER NOT NULL DEFAULT 0);
CREATE TABLE team_profiles (namespace TEXT PRIMARY KEY REFERENCES namespaces(name), display_name TEXT NOT NULL DEFAULT '', bio TEXT NOT NULL DEFAULT '', revision INTEGER NOT NULL DEFAULT 0);
CREATE TABLE comments (sequence INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT NOT NULL UNIQUE, release_id TEXT NOT NULL REFERENCES releases(id), parent_id TEXT REFERENCES comments(id), author_id TEXT NOT NULL REFERENCES users(id), body TEXT NOT NULL, created_at INTEGER NOT NULL, deleted INTEGER NOT NULL DEFAULT 0);
CREATE INDEX comments_thread ON comments(release_id,parent_id,sequence);
CREATE TABLE reactions (user_id TEXT NOT NULL REFERENCES users(id), release_id TEXT NOT NULL REFERENCES releases(id), bookmark INTEGER NOT NULL CHECK(bookmark IN (0,1)), PRIMARY KEY(user_id,release_id,bookmark));
CREATE INDEX reactions_release ON reactions(release_id,bookmark);
PRAGMA user_version=3;
`

const avatarSchema = `
ALTER TABLE profiles ADD COLUMN avatar TEXT NOT NULL DEFAULT '';
PRAGMA user_version=4;
`

const presentationSchema = `
ALTER TABLE releases ADD COLUMN display_name_en TEXT NOT NULL DEFAULT '';
ALTER TABLE releases ADD COLUMN display_name_zh TEXT NOT NULL DEFAULT '';
ALTER TABLE releases ADD COLUMN icon INTEGER NOT NULL DEFAULT 0;
PRAGMA user_version=6;
`

const accessRequestSchema = `
CREATE TABLE access_requests (user_id TEXT PRIMARY KEY REFERENCES users(id), created_at INTEGER NOT NULL);
PRAGMA user_version=7;
`

// 平台职责不参与命名空间授权；通知和报告随数据库一并备份。
const governanceSchema = `
CREATE TABLE platform_roles (user_id TEXT NOT NULL REFERENCES users(id), role INTEGER NOT NULL CHECK(role IN (1,2)), PRIMARY KEY(user_id,role));
CREATE TABLE release_scans (sequence INTEGER PRIMARY KEY AUTOINCREMENT, release_id TEXT NOT NULL REFERENCES releases(id), digest TEXT NOT NULL, report TEXT NOT NULL, created_at INTEGER NOT NULL);
CREATE INDEX release_scans_release ON release_scans(release_id,sequence);
CREATE TABLE review_notifications (sequence INTEGER PRIMARY KEY AUTOINCREMENT, recipient TEXT NOT NULL REFERENCES users(id), release_id TEXT NOT NULL REFERENCES releases(id), approved INTEGER NOT NULL, is_read INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL, UNIQUE(recipient,release_id));
CREATE INDEX review_notifications_recipient ON review_notifications(recipient,sequence);
PRAGMA user_version=8;`

// 精选独立于不可变版本，目录始终先过滤受众。
const discoverySchema = `
CREATE TABLE featured_skills (namespace TEXT NOT NULL REFERENCES namespaces(name), name TEXT NOT NULL, PRIMARY KEY(namespace,name));
CREATE INDEX releases_review_queue ON releases(status,sequence);
PRAGMA user_version=9;`
