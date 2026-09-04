# syntax=docker/dockerfile:1

FROM golang:1.26-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    curl \
    docker.io \
    git \
    make \
    && rm -rf /var/lib/apt/lists/*

RUN go install github.com/quality-gates/messgo/cmd/messgo@latest

# mutago fails to build on linux/arm64 due to syscall.Dup2 in an indirect dependency.
# Install mutago on supported architectures (such as amd64).
RUN if [ "$(dpkg --print-architecture)" = "amd64" ]; then \
        go install github.com/quality-gates/mutago/v2/cmd/mutago@latest; \
    fi

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .

CMD ["bash"]
