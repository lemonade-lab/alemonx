#!/bin/bash
set -e

find . -name '.DS_Store' -type f -delete
go test ./internal/...
go build
git diff --exit-code
