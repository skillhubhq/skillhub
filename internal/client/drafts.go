package client

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"go.yaml.in/yaml/v3"
)

// DraftWorkspace 是客户端私有副本，编辑不会覆盖来源目录或已安装内容。
type DraftWorkspace struct {
	ID        string   `json:"id"`
	Directory string   `json:"directory"`
	Files     []string `json:"files"`
}
type DraftFile struct {
	Path   string `json:"path"`
	Text   string `json:"text"`
	Digest string `json:"digest"`
}

const maxDraftText = 1 << 20

func (c *Client) CreateDraft(ctx context.Context, directory string, target PreviewTarget) (DraftWorkspace, error) {
	var bundle *packageformat.Bundle
	var err error
	if directory != "" && target.Source != "" {
		return DraftWorkspace{}, errors.New("choose one draft source")
	}
	if directory != "" {
		_, bundle, err = packageformat.Pack(directory)
	} else if target.Source != "" {
		release, archive, downloadErr := c.downloadPreview(ctx, target)
		if downloadErr != nil {
			return DraftWorkspace{}, downloadErr
		}
		if target.Digest != "" && release.GetDigest() != target.Digest {
			return DraftWorkspace{}, errors.New("draft source changed")
		}
		bundle, err = inspectPreviewBundle(release, archive)
	} else {
		document, templateErr := packageformat.Template("my-skill", "Describe when to use this skill.")
		if templateErr != nil {
			return DraftWorkspace{}, templateErr
		}
		bundle = &packageformat.Bundle{Files: []packageformat.File{{Path: "SKILL.md", Data: document}}}
	}
	if err != nil {
		return DraftWorkspace{}, err
	}
	if err = ctx.Err(); err != nil {
		return DraftWorkspace{}, err
	}
	id := uuid.NewString()
	parent := filepath.Join(c.store.root, "drafts")
	confined, err := os.OpenRoot(c.store.root)
	if err != nil {
		return DraftWorkspace{}, err
	}
	defer confined.Close()
	if err = confined.MkdirAll("drafts", 0700); err != nil {
		return DraftWorkspace{}, err
	}
	if err = confined.Mkdir(filepath.Join("drafts", id), 0700); err != nil {
		return DraftWorkspace{}, err
	}
	work, err := confined.OpenRoot(filepath.Join("drafts", id))
	if err != nil {
		return DraftWorkspace{}, err
	}
	defer work.Close()
	complete := false
	defer func() {
		if !complete {
			_ = confined.RemoveAll(filepath.Join("drafts", id))
		}
	}()
	for _, file := range bundle.Files {
		if !validDraftPath(file.Path) {
			return DraftWorkspace{}, errors.New("invalid draft file path")
		}
		if err = work.MkdirAll(path.Dir(file.Path), 0700); err != nil {
			return DraftWorkspace{}, err
		}
		mode := os.FileMode(0600)
		if file.Executable {
			mode = 0700
		}
		f, e := work.OpenFile(file.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if e != nil {
			return DraftWorkspace{}, e
		}
		_, writeErr := f.Write(file.Data)
		if e = errors.Join(writeErr, f.Close()); e != nil {
			return DraftWorkspace{}, e
		}
	}
	complete = true
	root := filepath.Join(parent, id)
	result := DraftWorkspace{ID: id, Directory: root, Files: []string{}}
	for _, file := range bundle.Files {
		result.Files = append(result.Files, file.Path)
	}
	return result, nil
}
func (s *Store) openDraft(id string) (*os.Root, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, errors.New("invalid draft")
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.OpenRoot(filepath.Join("drafts", id))
}
func validDraftPath(name string) bool {
	return name != "" && name != "." && path.Clean(name) == name && !strings.HasPrefix(name, "/") && !strings.Contains(name, "\\") && !strings.Contains(name, ":") && name != ".." && !strings.HasPrefix(name, "../")
}
func readDraftFile(root *os.Root, name string) (DraftFile, error) {
	if !validDraftPath(name) {
		return DraftFile{}, errors.New("invalid draft file path")
	}
	info, err := root.Lstat(name)
	if err != nil {
		return DraftFile{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxDraftText {
		return DraftFile{}, errors.New("only regular UTF-8 text files up to 1 MiB can be edited")
	}
	f, err := root.Open(name)
	if err != nil {
		return DraftFile{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxDraftText+1))
	if err != nil {
		return DraftFile{}, err
	}
	if len(b) > maxDraftText || !utf8.Valid(b) || strings.ContainsRune(string(b), 0) {
		return DraftFile{}, errors.New("only UTF-8 text files up to 1 MiB can be edited")
	}
	return DraftFile{Path: name, Text: string(b), Digest: packageformat.Digest(b)}, nil
}
func (s *Store) ReadDraftFile(id, name string) (DraftFile, error) {
	root, err := s.openDraft(id)
	if err != nil {
		return DraftFile{}, err
	}
	defer root.Close()
	return readDraftFile(root, name)
}

// SaveDraftFile 使用乐观摘要检查和跨进程锁；Root 将符号链接与相对路径限制在草稿内。
func (s *Store) SaveDraftFile(ctx context.Context, id, name, expected, text string) (DraftFile, error) {
	if len(text) > maxDraftText || !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return DraftFile{}, errors.New("only UTF-8 text files up to 1 MiB can be edited")
	}
	root, err := s.openDraft(id)
	if err != nil {
		return DraftFile{}, err
	}
	defer root.Close()
	lock := flock.New(filepath.Join(s.root, "drafts", "."+id+".lock"))
	if err = lock.Lock(); err != nil {
		return DraftFile{}, err
	}
	defer lock.Unlock()
	if err = ctx.Err(); err != nil {
		return DraftFile{}, err
	}
	current, err := readDraftFile(root, name)
	if err != nil {
		return DraftFile{}, err
	}
	if expected == "" || current.Digest != expected {
		return DraftFile{}, errors.New("file changed since it was opened; reload before saving")
	}
	info, err := root.Stat(name)
	if err != nil {
		return DraftFile{}, err
	}
	temp := path.Join(path.Dir(name), ".draft-"+uuid.NewString())
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return DraftFile{}, err
	}
	defer root.Remove(temp)
	_, writeErr := f.WriteString(text)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return DraftFile{}, err
	}
	if err = root.Rename(temp, name); err != nil {
		return DraftFile{}, err
	}
	return DraftFile{Path: name, Text: text, Digest: packageformat.Digest([]byte(text))}, nil
}

// 草稿目录是持久资产；枚举不要求 frontmatter 有效，允许重启后继续修复未完成内容。
type DraftSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	UpdatedAt string `json:"updated_at"`
}

func (s *Store) ListDrafts() ([]DraftSummary, error) {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	directory, err := root.OpenRoot("drafts")
	if errors.Is(err, os.ErrNotExist) {
		return []DraftSummary{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries, err := fs.ReadDir(directory.FS(), ".")
	if err != nil {
		return nil, err
	}
	result := []DraftSummary{}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if _, err := uuid.Parse(entry.Name()); err != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		item := DraftSummary{ID: entry.Name(), UpdatedAt: info.ModTime().UTC().Format(time.RFC3339Nano)}
		work, err := directory.OpenRoot(entry.Name())
		if err != nil {
			return nil, err
		}
		file, readErr := readDraftFile(work, "SKILL.md")
		work.Close()
		if readErr == nil {
			lines := strings.Split(strings.ReplaceAll(file.Text, "\r\n", "\n"), "\n")
			if len(lines) > 1 && lines[0] == "---" {
				for i := 1; i < len(lines); i++ {
					if lines[i] == "---" {
						var meta struct {
							Name string `yaml:"name"`
						}
						if yaml.Unmarshal([]byte(strings.Join(lines[1:i], "\n")), &meta) == nil && packageformat.ValidName(meta.Name) {
							item.Name = meta.Name
						}
						break
					}
				}
			}
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		left, _ := time.Parse(time.RFC3339Nano, result[i].UpdatedAt)
		right, _ := time.Parse(time.RFC3339Nano, result[j].UpdatedAt)
		if left.Equal(right) {
			return result[i].ID < result[j].ID
		}
		return left.After(right)
	})
	return result, nil
}

func (s *Store) OpenDraft(id string) (DraftWorkspace, error) {
	root, err := s.openDraft(id)
	if err != nil {
		return DraftWorkspace{}, err
	}
	defer root.Close()
	work := DraftWorkspace{ID: id, Directory: filepath.Join(s.root, "drafts", id), Files: []string{}}
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == "." {
			return nil
		}
		if !validDraftPath(name) || entry.Type()&os.ModeSymlink != 0 {
			return errors.New("draft contains an unsafe path")
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || len(work.Files) >= packageformat.MaxFiles {
			return errors.New("draft contains unsupported files")
		}
		work.Files = append(work.Files, name)
		return nil
	})
	return work, err
}
