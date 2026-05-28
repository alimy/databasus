#!/usr/bin/env bash
set -euo pipefail

sudo chown -R vscode:vscode \
  /workspaces/postgresus \
  /home/vscode/go \
  /home/vscode/.cache \
  /home/vscode/.local/share/pnpm

cd /workspaces/postgresus

cd backend
go mod download
cd ..

cd frontend
pnpm install --frozen-lockfile
cd ..

pre-commit install --install-hooks
