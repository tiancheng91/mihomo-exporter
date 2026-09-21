# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/mihomo-exporter ./cmd/mihomo-exporter

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/mihomo-exporter /mihomo-exporter
EXPOSE 9091
USER nonroot:nonroot
ENTRYPOINT ["/mihomo-exporter"]

