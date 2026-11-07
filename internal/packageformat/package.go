// Package packageformat 定义 Hub 与客户端共同执行的纯数据包规则。
package packageformat

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

const (
	MaxArchiveBytes  = 20 << 20
	MaxExpandedBytes = 80 << 20
	MaxFileBytes     = 8 << 20
	MaxFiles         = 2048
)

var skillName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type File struct {
	Path       string
	Data       []byte
	Executable bool
}

type Metadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	License     string `yaml:"license"`
}

type Bundle struct {
	Metadata    Metadata
	Files       []File
	Digest      string
	Fingerprint string
	Markdown    string
}

func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ValidName(name string) bool {
	return len(name) > 0 && len(name) <= 64 && skillName.MatchString(name)
}

// Inspect 只读取有界包内容，不执行脚本，也不写入目标目录。
func Inspect(data []byte) (*Bundle, error) {
	if len(data) == 0 || len(data) > MaxArchiveBytes {
		return nil, errors.New("archive size exceeds limits")
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("invalid ZIP archive")
	}
	if len(zr.File) > MaxFiles {
		return nil, errors.New("too many archive entries")
	}
	files := make([]File, 0, len(zr.File))
	seen := map[string]bool{}
	total := int64(0)
	for _, item := range zr.File {
		name := strings.TrimSuffix(item.Name, "/")
		if err := validatePath(name); err != nil {
			return nil, err
		}
		key := strings.ToLower(name)
		if seen[key] {
			return nil, errors.New("duplicate or case-colliding archive path")
		}
		seen[key] = true
		if item.Mode().IsDir() {
			continue
		}
		if !item.Mode().IsRegular() {
			return nil, errors.New("only regular files are supported")
		}
		if item.UncompressedSize64 > MaxFileBytes {
			return nil, errors.New("file size exceeds limits")
		}
		r, err := item.Open()
		if err != nil {
			return nil, errors.New("cannot open archive entry")
		}
		content, readErr := io.ReadAll(io.LimitReader(r, MaxFileBytes+1))
		closeErr := r.Close()
		if readErr != nil || closeErr != nil {
			return nil, errors.New("invalid archive entry content")
		}
		if len(content) > MaxFileBytes {
			return nil, errors.New("file size exceeds limits")
		}
		total += int64(len(content))
		if total > MaxExpandedBytes {
			return nil, errors.New("expanded archive exceeds limits")
		}
		files = append(files, File{Path: name, Data: content, Executable: item.Mode()&0111 != 0})
	}
	if err := validateTree(files); err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	var markdown []byte
	for _, file := range files {
		if file.Path == "SKILL.md" {
			markdown = file.Data
		}
	}
	meta, err := parseMetadata(markdown)
	if err != nil {
		return nil, err
	}
	return &Bundle{Metadata: meta, Files: files, Digest: Digest(data), Fingerprint: Fingerprint(files), Markdown: string(markdown)}, nil
}

func validatePath(name string) error {
	if name == "" || len(name) > 512 || !utf8.ValidString(name) || strings.ContainsAny(name, "\\:\x00\r\n") || path.IsAbs(name) || path.Clean(name) != name {
		return errors.New("unsafe package path")
	}
	for _, part := range strings.Split(name, "/") {
		if part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.ContainsAny(part, `<>"|?*`) {
			return errors.New("unsafe package path")
		}
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || (len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9') {
			return errors.New("reserved package path")
		}
	}
	return nil
}

func validateTree(files []File) error {
	names := map[string]bool{}
	spellings := map[string]string{}
	for _, file := range files {
		names[strings.ToLower(file.Path)] = true
		for node := file.Path; node != "."; node = path.Dir(node) {
			key := strings.ToLower(node)
			if previous, ok := spellings[key]; ok && previous != node {
				return errors.New("case-colliding package tree")
			}
			spellings[key] = node
		}
	}
	for _, file := range files {
		for parent := path.Dir(strings.ToLower(file.Path)); parent != "."; parent = path.Dir(parent) {
			if names[parent] {
				return errors.New("file conflicts with parent directory")
			}
		}
	}
	return nil
}

func parseMetadata(markdown []byte) (Metadata, error) {
	var meta Metadata
	if len(markdown) == 0 || !utf8.Valid(markdown) {
		return meta, errors.New("SKILL.md must contain UTF-8 text")
	}
	text := strings.ReplaceAll(string(markdown), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if lines[0] != "---" {
		return meta, errors.New("SKILL.md must start with YAML frontmatter")
	}
	end := 0
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end == 0 {
		return meta, errors.New("SKILL.md frontmatter is not terminated")
	}
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &meta); err != nil {
		return meta, errors.New("invalid SKILL.md frontmatter")
	}
	if !ValidName(meta.Name) {
		return meta, errors.New("invalid skill name")
	}
	if strings.TrimSpace(meta.Description) == "" || utf8.RuneCountInString(meta.Description) > 1024 {
		return meta, errors.New("description must contain 1 to 1024 characters")
	}
	return meta, nil
}

// Fingerprint 包括路径、执行位和文件内容，不依赖 ZIP 时间戳。
func Fingerprint(files []File) string {
	ordered := append([]File(nil), files...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	h := sha256.New()
	for _, file := range ordered {
		_ = binary.Write(h, binary.BigEndian, uint64(len(file.Path)))
		_, _ = io.WriteString(h, file.Path)
		flag := byte(0)
		if file.Executable {
			flag = 1
		}
		_, _ = h.Write([]byte{flag})
		_ = binary.Write(h, binary.BigEndian, uint64(len(file.Data)))
		_, _ = h.Write(file.Data)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// ReadDirectory 拒绝任何符号链接，避免把包目录之外的内容打包上传。
func ReadDirectory(root string) ([]File, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("package root must be a directory")
	}
	var files []File
	total := int64(0)
	err = filepath.WalkDir(root, func(full string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if full == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("package cannot contain symlinks")
		}
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if err := validatePath(rel); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > MaxFileBytes {
			return errors.New("unsupported package file")
		}
		r, err := os.Open(full)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(r, MaxFileBytes+1))
		closeErr := r.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if len(data) > MaxFileBytes {
			return errors.New("file size exceeds limits")
		}
		total += int64(len(data))
		if total > MaxExpandedBytes || len(files) >= MaxFiles {
			return errors.New("package exceeds limits")
		}
		files = append(files, File{Path: rel, Data: data, Executable: info.Mode()&0111 != 0})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// Pack 生成可复现 ZIP；调用 Inspect 使本地打包和远端校验遵守同一规则。
func Pack(root string) ([]byte, *Bundle, error) {
	files, err := ReadDirectory(root)
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, file := range files {
		header := &zip.FileHeader{Name: file.Path, Method: zip.Deflate}
		header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		mode := fs.FileMode(0644)
		if file.Executable {
			mode = 0755
		}
		header.SetMode(mode)
		w, err := zw.CreateHeader(header)
		if err != nil {
			return nil, nil, err
		}
		if _, err = w.Write(file.Data); err != nil {
			return nil, nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, nil, err
	}
	data := out.Bytes()
	bundle, err := Inspect(data)
	return data, bundle, err
}

// Extract 只写入新建的空暂存目录，调用者在校验后负责目录切换。
func (b *Bundle) Extract(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("extraction root must be a directory")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("extraction root must be empty")
	}
	for _, file := range b.Files {
		if err := validatePath(file.Path); err != nil {
			return err
		}
		full := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			return err
		}
		mode := fs.FileMode(0644)
		if file.Executable {
			mode = 0755
		}
		f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return err
		}
		_, writeErr := f.Write(file.Data)
		syncErr := f.Sync()
		closeErr := f.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			return fmt.Errorf("extract package file: %w", err)
		}
	}
	return nil
}
