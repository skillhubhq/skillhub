//go:build !native_smoke

package main

import "github.com/wailsapp/wails/v3/pkg/application"
import "github.com/skillhubhq/skillhub/internal/client"

func nativeSmokeServices() ([]application.Service, error) { return nil, nil }

func nativeClientOptions() client.ClientOptions { return client.ClientOptions{} }
