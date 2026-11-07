package packageformat

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Template 使用同一元数据校验规则生成纯文本，不嵌入本地身份或路径。
func Template(name, description string) ([]byte, error) {
	header, err := yaml.Marshal(Metadata{Name: name, Description: description})
	if err != nil {
		return nil, err
	}
	document := []byte("---\n" + string(header) + "---\n# " + strings.ReplaceAll(name, "-", " ") + "\n\n## When to use\n\nDescribe the task and the inputs needed.\n\n## Instructions\n\n1. Confirm the available inputs and constraints.\n2. Follow the steps for this task.\n3. Check the result against the expected output.\n\n## Expected output\n\nDescribe the format and acceptance criteria.\n\n## Boundaries\n\nUse only authorized inputs. Ask before external writes or destructive actions.\n")
	if _, err = parseMetadata(document); err != nil {
		return nil, err
	}
	return document, nil
}

// InitDirectory 只创建不存在的目录，拒绝覆盖已有目录、文件或符号链接。
func InitDirectory(directory, name, description string) (string, error) {
	document, err := Template(name, description)
	if err != nil {
		return "", err
	}
	directory, err = filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	if err = os.Mkdir(directory, 0700); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		_ = os.Remove(directory)
		return "", err
	}
	defer root.Close()
	file, err := root.OpenFile("SKILL.md", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		_ = os.Remove(directory)
		return "", err
	}
	_, writeErr := file.Write(document)
	err = errors.Join(writeErr, file.Close())
	if err != nil {
		_ = root.Remove("SKILL.md")
		_ = os.Remove(directory)
		return "", err
	}
	return directory, nil
}
