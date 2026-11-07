//go:build native_smoke

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/packageformat"
)

type upgradeAccount struct {
	Alias    string `json:"alias"`
	Password string `json:"password"`
}
type upgradeSnapshot struct {
	Sources           []client.Source       `json:"sources"`
	Installed         []client.Installation `json:"installed"`
	Projects          []string              `json:"projects"`
	Draft             client.DraftWorkspace `json:"draft"`
	Incomplete        client.DraftWorkspace `json:"incomplete"`
	CredentialDigests map[string]string     `json:"credential_digests"`
	Files             map[string]string     `json:"files"`
}

func (r *NativeSmokeReporter) UpgradeMode() string {
	switch mode := os.Getenv("SKILLHUB_UPGRADE_PHASE"); mode {
	case "seed", "baseline", "candidate", "restart", "rollback":
		return mode
	}
	return ""
}
func (r *NativeSmokeReporter) upgradeClient() (*client.Store, *client.Client, error) {
	if r.UpgradeMode() == "" {
		return nil, nil, errors.New("upgrade fixture is not enabled")
	}
	store, err := client.OpenStore(r.root)
	if err != nil {
		return nil, nil, err
	}
	return store, client.NewClient(store, client.ClientOptions{Home: filepath.Join(r.root, "home")}), nil
}
func upgradeFiles(root string, snapshot upgradeSnapshot) (map[string]string, error) {
	files := map[string]string{}
	dirs := []string{snapshot.Draft.Directory, snapshot.Incomplete.Directory}
	for _, i := range snapshot.Installed {
		dirs = append(dirs, i.Target)
	}
	for _, directory := range dirs {
		rel, err := filepath.Rel(root, directory)
		if err != nil || !filepath.IsLocal(rel) {
			return nil, errors.New("fixture path escaped its root")
		}
		entries, err := packageformat.ReadDirectory(directory)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			files[filepath.Join(rel, entry.Path)] = packageformat.Digest(entry.Data)
		}
	}
	for _, project := range snapshot.Projects {
		for _, name := range []string{"skillhub.json", "skillhub.lock.json"} {
			rel, err := filepath.Rel(root, filepath.Join(project, name))
			if err != nil || !filepath.IsLocal(rel) {
				return nil, errors.New("fixture project escaped its root")
			}
			data, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				return nil, err
			}
			files[rel] = packageformat.Digest(data)
		}
	}
	return files, nil
}
func (r *NativeSmokeReporter) SeedUpgrade() error {
	if r.UpgradeMode() != "seed" {
		return errors.New("not the seed phase")
	}
	store, core, err := r.upgradeClient()
	if err != nil {
		return err
	}
	defer store.Close()
	raw, err := os.ReadFile(filepath.Join(r.root, "upgrade-accounts.json"))
	if err != nil {
		return err
	}
	var accounts []upgradeAccount
	if err = json.Unmarshal(raw, &accounts); err != nil {
		return err
	}
	ctx := context.Background()
	for _, account := range accounts {
		if _, present := os.LookupEnv("SKILLHUB_TOKEN_" + strings.ToUpper(account.Alias)); present {
			return errors.New("injected credential bypasses acceptance")
		}
		if _, err = core.LoginPassword(ctx, account.Alias, "admin", account.Password); err != nil {
			return errors.New("system credential fixture login failed")
		}
	}
	project := filepath.Join(r.root, "project")
	if err = os.Mkdir(project, 0700); err != nil {
		return err
	}
	for _, scope := range []string{"project", "global"} {
		for _, tool := range []string{"codex", "claude-code"} {
			location := ""
			if scope == "project" {
				location = project
			}
			plan, archive, err := core.PlanRemoteInstall(ctx, client.Coordinate{Source: "company", Namespace: "team", Name: "upgrade-check", Version: "1.0.0"}, tool, scope, location)
			if err != nil {
				return err
			}
			if err = store.Apply(ctx, *plan, archive); err != nil {
				return err
			}
		}
	}
	if _, err = core.InitProject(ctx, project); err != nil {
		return err
	}
	for _, tool := range []string{"codex", "claude-code"} {
		if _, err = core.SetProjectSkill(ctx, project, client.ProjectSkill{Source: "company", Namespace: "team", Name: "upgrade-check", Version: "1.0.0", Tool: tool}); err != nil {
			return err
		}
	}
	if _, err = core.LockProject(ctx, project); err != nil {
		return err
	}
	draft, err := core.CreateDraft(ctx, "", client.PreviewTarget{})
	if err != nil {
		return err
	}
	file, err := store.ReadDraftFile(draft.ID, "SKILL.md")
	if err != nil {
		return err
	}
	if _, err = store.SaveDraftFile(ctx, draft.ID, "SKILL.md", file.Digest, strings.Replace(file.Text, "my-skill", "upgrade-draft", 1)+"\nSaved before application replacement.\n"); err != nil {
		return err
	}
	incomplete, err := core.CreateDraft(ctx, "", client.PreviewTarget{})
	if err != nil {
		return err
	}
	file, err = store.ReadDraftFile(incomplete.ID, "SKILL.md")
	if err != nil {
		return err
	}
	if _, err = store.SaveDraftFile(ctx, incomplete.ID, "SKILL.md", file.Digest, "---\nname: [unfinished\n"); err != nil {
		return err
	}
	snapshot := upgradeSnapshot{Draft: draft, Incomplete: incomplete, CredentialDigests: map[string]string{}}
	snapshot.Sources, err = core.ListSources(ctx)
	if err != nil {
		return err
	}
	snapshot.Installed, err = store.ListInstallations(ctx)
	if err != nil {
		return err
	}
	if len(snapshot.Installed) != 4 {
		return errors.New("fixture must contain four installations")
	}
	snapshot.Projects, err = core.ListProjects(ctx)
	if err != nil {
		return err
	}
	for _, source := range snapshot.Sources {
		token, err := (client.SystemCredentials{}).Get(source.CredentialRef)
		if err != nil {
			return err
		}
		snapshot.CredentialDigests[source.CredentialRef] = packageformat.Digest([]byte(token))
	}
	// 验证覆盖安装也保留用户对已安装技能的本地修改。
	if err = os.WriteFile(filepath.Join(snapshot.Installed[0].Target, "local-note.txt"), []byte("local edit must survive"), 0600); err != nil {
		return err
	}
	snapshot.Files, err = upgradeFiles(r.root, snapshot)
	if err != nil {
		return err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.root, "upgrade-snapshot.json"), data, 0600)
}
func (r *NativeSmokeReporter) VerifyUpgrade() (client.DraftWorkspace, error) {
	store, core, err := r.upgradeClient()
	if err != nil {
		return client.DraftWorkspace{}, err
	}
	defer store.Close()
	data, err := os.ReadFile(filepath.Join(r.root, "upgrade-snapshot.json"))
	if err != nil {
		return client.DraftWorkspace{}, err
	}
	var snapshot upgradeSnapshot
	if err = json.Unmarshal(data, &snapshot); err != nil {
		return client.DraftWorkspace{}, err
	}
	ctx := context.Background()
	sources, err := core.ListSources(ctx)
	if err != nil {
		return client.DraftWorkspace{}, err
	}
	installed, err := store.ListInstallations(ctx)
	if err != nil {
		return client.DraftWorkspace{}, err
	}
	projects, err := core.ListProjects(ctx)
	if err != nil {
		return client.DraftWorkspace{}, err
	}
	if !reflect.DeepEqual(sources, snapshot.Sources) || !reflect.DeepEqual(installed, snapshot.Installed) || !reflect.DeepEqual(projects, snapshot.Projects) {
		return client.DraftWorkspace{}, errors.New("upgrade changed source, installation or project records")
	}
	for _, source := range sources {
		token, err := (client.SystemCredentials{}).Get(source.CredentialRef)
		if err != nil || packageformat.Digest([]byte(token)) != snapshot.CredentialDigests[source.CredentialRef] {
			return client.DraftWorkspace{}, errors.New("upgrade changed or lost a system credential")
		}
	}
	files, err := upgradeFiles(r.root, snapshot)
	if err != nil {
		return client.DraftWorkspace{}, err
	}
	if !reflect.DeepEqual(files, snapshot.Files) {
		return client.DraftWorkspace{}, errors.New("upgrade changed draft, project or installed skill files")
	}
	if _, err = store.PlanRemove(ctx, installed[0].Target); !errors.Is(err, client.ErrModified) {
		return client.DraftWorkspace{}, errors.New("local edits were not protected")
	}
	return snapshot.Draft, nil
}
