package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/gofrs/flock"
)

const (
	cacheMaxBytes   int64 = 256 << 20
	cacheMaxEntries       = 128
)

type CacheInfo struct {
	Entries      int    `json:"entries"`
	Bytes        int64  `json:"bytes"`
	LimitBytes   int64  `json:"limit_bytes"`
	LimitEntries int    `json:"limit_entries"`
	Directory    string `json:"directory"`
}

// 键包含服务身份、规范地址与精确坐标；不把某个源的包摘要发给另一来源。
func packageCacheKey(source Source, coordinate Coordinate) string {
	raw, _ := json.Marshal([]string{source.RegistryID, source.URL, coordinate.Namespace, coordinate.Name, coordinate.Version})
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func (s *Store) withPackageCache(ctx context.Context, use func(string) error) error {
	lock := flock.New(filepath.Join(s.root, ".package-cache.lock"))
	defer lock.Close()
	locked, err := lock.TryLockContext(ctx, 25*time.Millisecond)
	if err != nil {
		return err
	}
	if !locked {
		return errors.New("package cache is busy")
	}
	dir := filepath.Join(s.root, "package-cache")
	if err = os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("package cache must be a local directory")
	}
	return use(dir)
}

func validCacheKey(key string) bool {
	if len(key) != 64 || strings.ToLower(key) != key {
		return false
	}
	_, err := hex.DecodeString(key)
	return err == nil
}

type cachedPackage struct {
	name string
	size int64
	used time.Time
}

func scanPackageCache(dir string) ([]cachedPackage, int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0, err
	}
	items := []cachedPackage{}
	var size int64
	for _, entry := range entries {
		// 写操作都持同一个进程间锁，此处临时文件必然来自已中断的写入。
		if strings.HasPrefix(entry.Name(), ".download-") {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return nil, 0, err
			}
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".zip") || !validCacheKey(strings.TrimSuffix(entry.Name(), ".zip")) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, 0, err
		}
		if !info.Mode().IsRegular() {
			return nil, 0, errors.New("package cache entry must be a regular file")
		}
		items = append(items, cachedPackage{entry.Name(), info.Size(), info.ModTime()})
		size += info.Size()
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].used.Equal(items[j].used) {
			return items[i].name < items[j].name
		}
		return items[i].used.Before(items[j].used)
	})
	return items, size, nil
}

func (s *Store) CacheInfo(ctx context.Context) (CacheInfo, error) {
	result := CacheInfo{LimitBytes: cacheMaxBytes, LimitEntries: cacheMaxEntries}
	err := s.withPackageCache(ctx, func(dir string) error {
		items, size, err := scanPackageCache(dir)
		result.Entries, result.Bytes, result.Directory = len(items), size, dir
		return err
	})
	return result, err
}

func (s *Store) ClearCache(ctx context.Context) (CacheInfo, error) {
	result := CacheInfo{LimitBytes: cacheMaxBytes, LimitEntries: cacheMaxEntries}
	err := s.withPackageCache(ctx, func(dir string) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if (strings.HasSuffix(entry.Name(), ".zip") && validCacheKey(strings.TrimSuffix(entry.Name(), ".zip"))) || strings.HasPrefix(entry.Name(), ".download-") {
				if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
					return err
				}
			}
		}
		result.Directory = dir
		return syncDirectory(dir)
	})
	return result, err
}

func (s *Store) cachedPackage(ctx context.Context, key string) ([]byte, error) {
	if !validCacheKey(key) {
		return nil, errors.New("invalid package cache key")
	}
	var data []byte
	err := s.withPackageCache(ctx, func(dir string) error {
		var err error
		data, err = readCachedPackage(filepath.Join(dir, key+".zip"))
		return err
	})
	return data, err
}

func readCachedPackage(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > packageformat.MaxArchiveBytes {
		return nil, errors.New("invalid package cache entry")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, statErr := file.Stat()
	if statErr != nil || !os.SameFile(info, opened) {
		_ = file.Close()
		return nil, errors.New("package cache changed while opening")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, packageformat.MaxArchiveBytes+1))
	if err = errors.Join(readErr, file.Close()); err != nil {
		return nil, err
	}
	if len(data) > packageformat.MaxArchiveBytes {
		return nil, errors.New("package cache exceeds archive limit")
	}
	return data, nil
}

func (s *Store) cachePackage(ctx context.Context, key string, data []byte) error {
	return s.cachePackageBounded(ctx, key, data, cacheMaxBytes, cacheMaxEntries)
}

func (s *Store) cachePackageBounded(ctx context.Context, key string, data []byte, maxBytes int64, maxEntries int) error {
	if !validCacheKey(key) || len(data) == 0 || len(data) > packageformat.MaxArchiveBytes || int64(len(data)) > maxBytes || maxEntries < 1 {
		return errors.New("invalid cache write")
	}
	return s.withPackageCache(ctx, func(dir string) error {
		items, size, err := scanPackageCache(dir)
		if err != nil {
			return err
		}
		path := filepath.Join(dir, key+".zip")
		// 缓存已命中且内容一致时只更新时间，减少大包重复写入。
		unchanged := false
		for _, item := range items {
			if item.name != key+".zip" {
				continue
			}
			previous, readErr := readCachedPackage(path)
			if readErr == nil && len(previous) == len(data) && packageformat.Digest(previous) == packageformat.Digest(data) {
				unchanged = true
			}
			if !unchanged {
				if err := os.Remove(path); err != nil {
					return err
				}
			}
			size -= item.size
			break
		}
		remaining := make([]cachedPackage, 0, len(items))
		for _, item := range items {
			if item.name != key+".zip" {
				remaining = append(remaining, item)
			}
		}
		for len(remaining) >= maxEntries || size+int64(len(data)) > maxBytes {
			if len(remaining) == 0 {
				return errors.New("cache capacity exceeded")
			}
			item := remaining[0]
			if err := os.Remove(filepath.Join(dir, item.name)); err != nil {
				return err
			}
			size -= item.size
			remaining = remaining[1:]
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if unchanged {
			now := time.Now()
			return os.Chtimes(path, now, now)
		}
		file, err := os.CreateTemp(dir, ".download-")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())
		_, writeErr := file.Write(data)
		if err = errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
			return err
		}
		if err = os.Rename(file.Name(), path); err != nil {
			return err
		}
		return syncDirectory(dir)
	})
}
