package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/skillhubhq/skillhub/internal/packageformat"
)

func TestPackageCacheKeyBindsHubAndCoordinateWithoutCredentialOrAlias(t *testing.T) {
	source := Source{Alias: "company", RegistryID: "registry-one", URL: "https://one.example"}
	coordinate := Coordinate{Source: "company", Namespace: "team", Name: "check", Version: "1.0.0"}
	key := packageCacheKey(source, coordinate)
	alias := source
	alias.Alias = "another"
	alias.CredentialRef = "another-login"
	if packageCacheKey(alias, coordinate) != key {
		t.Fatal("same Hub identity/coordinate did not share cached bytes")
	}
	other := source
	other.RegistryID = "registry-two"
	if packageCacheKey(other, coordinate) == key {
		t.Fatal("cache shared between registry identities")
	}
	other = source
	other.URL = "https://two.example"
	if packageCacheKey(other, coordinate) == key {
		t.Fatal("cache shared between origins")
	}
	for _, changed := range []Coordinate{{Namespace: "other", Name: "check", Version: "1.0.0"}, {Namespace: "team", Name: "other", Version: "1.0.0"}, {Namespace: "team", Name: "check", Version: "2.0.0"}} {
		if packageCacheKey(source, changed) == key {
			t.Fatal("cache shared between coordinates")
		}
	}
}

func TestPackageCacheEvictionRecoveryAndCleanupPreserveInstallations(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	archive, digest := testArchive(t, "cached instructions")
	keys := []string{fmt.Sprintf("%064x", 1), fmt.Sprintf("%064x", 2), fmt.Sprintf("%064x", 3)}
	limit := int64(len(archive) * 2)
	for _, key := range keys[:2] {
		if err := s.cachePackageBounded(ctx, key, archive, limit, 2); err != nil {
			t.Fatal(err)
		}
	}
	info, err := s.CacheInfo(ctx)
	if err != nil || info.Entries != 2 || info.Bytes != limit {
		t.Fatalf("cache stats: %+v %v", info, err)
	}
	old := time.Unix(100, 0)
	if err = os.Chtimes(filepath.Join(info.Directory, keys[1]+".zip"), old, old); err != nil {
		t.Fatal(err)
	}
	if err = s.cachePackageBounded(ctx, keys[0], archive, limit, 2); err != nil {
		t.Fatal(err)
	}
	if err = s.cachePackageBounded(ctx, keys[2], archive, limit, 2); err != nil {
		t.Fatal(err)
	}
	if _, err = s.cachedPackage(ctx, keys[1]); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("did not evict least recently used package")
	}
	if data, err := s.cachedPackage(ctx, keys[0]); err != nil || !bytes.Equal(data, archive) {
		t.Fatal("evicted recently used package")
	}
	if err = s.cachePackageBounded(ctx, keys[0], archive, int64(len(archive)), 10); err != nil {
		t.Fatal(err)
	}
	if info, err = s.CacheInfo(ctx); err != nil || info.Entries != 1 || info.Bytes != int64(len(archive)) {
		t.Fatal("cache hit did not enforce byte bound")
	}
	if err = os.WriteFile(filepath.Join(info.Directory, ".download-interrupted"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CacheInfo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(info.Directory, ".download-interrupted")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("interrupted cache write survived recovery")
	}
	plan, err := s.PlanInstall(ctx, record(filepath.Join(t.TempDir(), "check"), digest), archive)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(ctx, *plan, archive); err != nil {
		t.Fatal(err)
	}
	if info, err = s.ClearCache(ctx); err != nil || info.Entries != 0 || info.Bytes != 0 {
		t.Fatalf("cache clear: %+v %v", info, err)
	}
	installed, err := s.ListInstallations(ctx)
	if err != nil || len(installed) != 1 {
		t.Fatal("cache cleanup changed installed inventory")
	}
	if _, err = s.PlanRemove(ctx, installed[0].Target); err != nil {
		t.Fatal("cache cleanup changed installed files")
	}
}

func TestTwoClientStoresCoordinateCacheWritesAndClear(t *testing.T) {
	s := testStore(t)
	second, err := OpenStore(s.root)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	archive, _ := testArchive(t, "shared")
	ctx := context.Background()
	var wg sync.WaitGroup
	failures := make(chan error, 12)
	for i := range 12 {
		wg.Go(func() {
			store := s
			if i%2 != 0 {
				store = second
			}
			if i%5 == 0 {
				_, err := store.ClearCache(ctx)
				failures <- err
				return
			}
			failures <- store.cachePackageBounded(ctx, fmt.Sprintf("%064x", i), archive, int64(4*len(archive)), 4)
		})
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	info, err := s.CacheInfo(ctx)
	if err != nil || info.Entries > 4 || info.Bytes > int64(4*len(archive)) {
		t.Fatalf("concurrent cache exceeded bounds: %+v %v", info, err)
	}
}

func TestPackageCacheRejectsOversizedAndUnsafeEntries(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	info, err := s.CacheInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("%064x", 1)
	file, err := os.Create(filepath.Join(info.Directory, key+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(packageformat.MaxArchiveBytes + 1)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	if _, err = s.cachedPackage(ctx, key); err == nil {
		t.Fatal("read oversized cached file")
	}
	if _, err = s.cachedPackage(ctx, "../../outside"); err == nil {
		t.Fatal("accepted cache path injection")
	}
	if err = s.cachePackage(ctx, "../../outside", []byte("bad")); err == nil {
		t.Fatal("accepted cache write path injection")
	}
}

func TestPackageCacheDoesNotFollowDirectoryOrFileSymlinks(t *testing.T) {
	s := testStore(t)
	outside := t.TempDir()
	cache := filepath.Join(s.root, "package-cache")
	if err := os.Symlink(outside, cache); err != nil {
		t.Skip(err)
	}
	ctx := context.Background()
	if _, err := s.ClearCache(ctx); err == nil {
		t.Fatal("followed cache directory symlink")
	}
	if err := os.Remove(cache); err != nil {
		t.Fatal(err)
	}
	info, err := s.CacheInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("%064x", 1)
	outsideFile := filepath.Join(outside, "keep")
	if err = os.WriteFile(outsideFile, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outsideFile, filepath.Join(info.Directory, key+".zip")); err != nil {
		t.Fatal(err)
	}
	if _, err = s.cachedPackage(ctx, key); err == nil {
		t.Fatal("followed cache file symlink")
	}
	if _, err = s.ClearCache(ctx); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(outsideFile); err != nil || string(data) != "keep" {
		t.Fatal("cache cleanup changed symlink destination")
	}
}
