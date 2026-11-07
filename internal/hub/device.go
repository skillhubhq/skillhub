package hub

import (
	"context"
	"database/sql"
	"errors"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"time"
)

type DeviceAuthorization struct {
	DeviceCode string
	UserCode   string
	ExpiresAt  time.Time
}

func (s *Store) StartDevice(ctx context.Context) (DeviceAuthorization, error) {
	value := DeviceAuthorization{DeviceCode: randomSecret(), UserCode: strings.ToUpper(secretHash(randomSecret())[:8]), ExpiresAt: s.now().Add(10 * time.Minute)}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DeviceAuthorization{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM devices WHERE expires_at<=?", s.now().Unix()); err != nil {
		return DeviceAuthorization{}, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM devices").Scan(&count); err != nil {
		return DeviceAuthorization{}, err
	}
	if count >= 10000 {
		return DeviceAuthorization{}, ErrRateLimited
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO devices(device_hash,user_code,expires_at) VALUES(?,?,?)", secretHash(value.DeviceCode), value.UserCode, value.ExpiresAt.Unix())
	if err != nil {
		return DeviceAuthorization{}, err
	}
	return value, tx.Commit()
}

func (s *Store) ApproveDevice(ctx context.Context, code, username, password string) error {
	if len(code) != 8 {
		return ErrInvalid
	}
	user, err := s.verifyPassword(ctx, username, password)
	if err != nil {
		return err
	}
	return s.approveDeviceIdentity(ctx, code, user, func(tx *hubdb.Tx) error {
		var hash []byte
		if err := tx.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id=?", user.ID).Scan(&hash); err != nil {
			return err
		}
		if bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil {
			return ErrUnauthenticated
		}
		return nil
	})
}

// ApproveDeviceSession 使用已认证的浏览器会话，仍需用户提交设备码确认。
func (s *Store) ApproveDeviceSession(ctx context.Context, code, token string) error {
	user, err := s.Authenticate(ctx, token)
	if err != nil {
		return err
	}
	return s.approveDeviceIdentity(ctx, code, user, func(tx *hubdb.Tx) error {
		var valid bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM sessions WHERE token_hash=? AND user_id=? AND expires_at>?)", secretHash(token), user.ID, s.now().Unix()).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return ErrUnauthenticated
		}
		return nil
	})
}

func (s *Store) approveDeviceIdentity(ctx context.Context, code string, user Identity, verify func(*hubdb.Tx) error) error {
	if len(code) != 8 {
		return ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE devices SET user_id=? WHERE user_code=? AND user_id IS NULL AND expires_at>? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled=0)", user.ID, strings.ToUpper(code), s.now().Unix(), user.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	if err = verify(tx); err != nil {
		return err
	}
	if err = audit(ctx, tx, user.ID, "device.approve", user.ID, s.now()); err != nil {
		return err
	}
	return tx.Commit()
}

// CompleteDevice 返回 nil 表示仍待确认；批准只能领取一次，短代码不能换取令牌。
func (s *Store) CompleteDevice(ctx context.Context, secret string) (*Session, error) {
	if len(secret) != 43 {
		return nil, ErrUnauthenticated
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// 领取与轮询时间在同一写事务中串行，跨进程也不能重复发放。
	if _, err = tx.ExecContext(ctx, "DELETE FROM devices WHERE expires_at<=?", s.now().Unix()); err != nil {
		return nil, err
	}
	var userID sql.NullString
	var lastPoll int64
	err = tx.QueryRowContext(ctx, "SELECT user_id,last_poll FROM devices WHERE device_hash=? AND expires_at>?", secretHash(secret), s.now().Unix()).Scan(&userID, &lastPoll)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	if s.now().Unix()-lastPoll < 2 {
		return nil, ErrRateLimited
	}
	if !userID.Valid {
		if _, err = tx.ExecContext(ctx, "UPDATE devices SET last_poll=? WHERE device_hash=?", s.now().Unix(), secretHash(secret)); err != nil {
			return nil, err
		}
		return nil, tx.Commit()
	}
	var user Identity
	err = tx.QueryRowContext(ctx, "SELECT id,username,administrator FROM users WHERE id=? AND disabled=0", userID.String).Scan(&user.ID, &user.Username, &user.Administrator)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	value, err := s.newSession(ctx, tx, user)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM devices WHERE device_hash=?", secretHash(secret)); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &value, nil
}
