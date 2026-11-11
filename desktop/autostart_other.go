//go:build !darwin && !windows

package main

import "errors"

func setStartAtLogin(enabled bool) error {
	if enabled {
		return errors.New("start at login is unavailable on this platform")
	}
	return nil
}
