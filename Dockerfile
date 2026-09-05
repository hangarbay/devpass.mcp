# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY main.go .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.serverVersionOverride=${VERSION}" \
    -o /out/devpass-usage .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/devpass-usage /devpass-usage
ENTRYPOINT ["/devpass-usage", "serve"]
