FROM golang:1.26-bookworm

ARG NODE_MAJOR=22
ARG PNPM_VERSION=9.15.9
ARG GOLANGCI_LINT_VERSION=v1.62.2
ARG USERNAME=vscode
ARG USER_UID=1000
ARG USER_GID=1000

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates curl gnupg sudo git \
      postgresql-client \
      zstd \
      python3 python3-pip pre-commit \
      libncurses5 libncurses6 libtinfo5 libtinfo6 \
      libmariadb3 libgnutls30 \
 && mkdir -p /etc/apt/keyrings \
 && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
      | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
 && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_${NODE_MAJOR}.x nodistro main" \
      > /etc/apt/sources.list.d/nodesource.list \
 && apt-get update \
 && apt-get install -y --no-install-recommends nodejs \
 && rm -rf /var/lib/apt/lists/*

RUN corepack enable

RUN groupadd --gid ${USER_GID} ${USERNAME} \
 && useradd --uid ${USER_UID} --gid ${USER_GID} --create-home --shell /bin/bash ${USERNAME} \
 && echo "${USERNAME} ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/${USERNAME} \
 && chmod 0440 /etc/sudoers.d/${USERNAME} \
 && mkdir -p /home/${USERNAME}/go /home/${USERNAME}/.cache /home/${USERNAME}/.local/share/pnpm \
 && chown -R ${USERNAME}:${USERNAME} /home/${USERNAME}

ENV GOPATH=/home/${USERNAME}/go
ENV PATH=${GOPATH}/bin:${PATH}

USER ${USERNAME}

RUN corepack prepare pnpm@${PNPM_VERSION} --activate

RUN go install github.com/pressly/goose/v3/cmd/goose@latest \
 && go install github.com/swaggo/swag/cmd/swag@latest

RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
    | sh -s -- -b $(go env GOPATH)/bin ${GOLANGCI_LINT_VERSION}

USER root
