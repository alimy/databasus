FROM mcr.microsoft.com/devcontainers/go:1-1.26-bookworm

ARG NODE_MAJOR=22
ARG PNPM_VERSION=9.15.9
ARG GOLANGCI_LINT_VERSION=v1.62.2

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates curl gnupg \
      postgresql-client \
      zstd \
      python3 python3-pip pre-commit \
 && mkdir -p /etc/apt/keyrings \
 && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
      | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
 && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_${NODE_MAJOR}.x nodistro main" \
      > /etc/apt/sources.list.d/nodesource.list \
 && apt-get update \
 && apt-get install -y --no-install-recommends nodejs \
 && rm -rf /var/lib/apt/lists/*

USER vscode

RUN corepack enable \
 && corepack prepare pnpm@${PNPM_VERSION} --activate

RUN go install github.com/pressly/goose/v3/cmd/goose@latest \
 && go install github.com/swaggo/swag/cmd/swag@latest

RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
    | sh -s -- -b $(go env GOPATH)/bin ${GOLANGCI_LINT_VERSION}
