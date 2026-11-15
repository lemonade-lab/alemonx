//go:build !darwin || ios

package main

import "github.com/wailsapp/wails/v3/pkg/application"

func prepareTrayMaterial(_ *application.WebviewWindow) bool { return false }
