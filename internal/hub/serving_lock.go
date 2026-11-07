package hub

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// 服务与离线账号恢复共用互斥锁，避免运维恢复与活动实例并行。
func AcquireServingLock(root string) (func(), error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	lock := flock.New(filepath.Join(root, ".server.lock"))
	ok, err := lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("Hub is serving or maintenance is active; stop it before continuing")
	}
	return func() { _ = lock.Unlock() }, nil
}
