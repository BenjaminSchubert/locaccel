FROM --platform=$BUILDPLATFORM docker.io/golang:1.25 AS build

WORKDIR /src

COPY . .

ARG TARGETOS TARGETARCH
RUN \
    --mount=type=cache,target=/go \
    --mount=type=cache,target=/root/.cache/go-build \
    go generate ./... && \
	GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0 \
        go build -ldflags '-s -w' -trimpath -o /build/locaccel ./cmd/locaccel

FROM scratch

COPY --from=build /build/locaccel /locaccel
COPY LICENSE /LICENSE
ENTRYPOINT [ "/locaccel" ]
