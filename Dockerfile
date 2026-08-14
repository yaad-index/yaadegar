# Multi-stage build: a static, CGO-free binary in the builder, copied into a
# minimal non-root runtime. The binary embeds its migrations (go:embed), so the
# runtime image carries only the binary — nothing else to ship.

# --- builder ---
# Base images are pinned by digest (reproducible, tamper-evident); the readable tag
# sits on the comment line above each digest. Both digests are multi-arch
# manifest-list digests, so the arm64 + amd64 publish build still resolves each
# platform.
# golang:1.26.2
FROM golang@sha256:b54cbf583d390341599d7bcbc062425c081105cc5ef6d170ced98ef9d047c716 AS build
WORKDIR /src

# Cache module downloads separately from the source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# VERSION is stamped into the binary at link time so a running image reports its
# build instead of "dev" (#190). The publish workflow passes the release version
# as a build-arg; a plain `docker build` with no build-arg keeps the "dev" default.
ARG VERSION=dev
# CGO off → a fully static binary (modernc sqlite and pgx are pure Go), so it runs
# on the distroless static base. -s -w strips debug info to keep it small;
# -X main.version stamps VERSION (main.version is the var in ./cmd/yaadegar).
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/yaadegar ./cmd/yaadegar

# --- runtime ---
# gcr.io/distroless/static-debian12:nonroot
FROM gcr.io/distroless/static-debian12@sha256:f5b485ea962d9bd1186b2f6b3a061191539b905b82ec395de78cbfae51f20e35
COPY --from=build /out/yaadegar /usr/local/bin/yaadegar
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/yaadegar"]
CMD ["serve"]
