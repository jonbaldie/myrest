# syntax=docker/dockerfile:1

FROM golang:1.26-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/myrest ./cmd/myrest

FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata tini

RUN addgroup -S -g 10001 myrest && \
    adduser -S -u 10001 -G myrest -h /home/myrest myrest

COPY --from=builder /bin/myrest /usr/local/bin/myrest

USER myrest
WORKDIR /home/myrest

ENV MYREST_LISTEN=0.0.0.0:3000

EXPOSE 3000

ENTRYPOINT ["/sbin/tini", "--", "myrest"]
