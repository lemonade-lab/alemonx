//go:build !alxdev

package main

import (
	"context"
	"embed"
)

// Release builds require the complete archive; only development can load it later.
//
//go:embed all:resources/templates all:resources/nvm all:resources/packages/yarn all:resources/packages/pm2 resources/dsh/runtime.zip
var resourceFiles embed.FS

func prepareDevelopmentDSH(context.Context) {}
