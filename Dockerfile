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
# VERSION stamps the version at link time. The publish workflow passes it as a
# build-arg on a tagged release (a pure semver); a non-empty arg is used verbatim, so
# the release path keeps emitting exactly that. Left EMPTY for a source build (e.g. a
# `docker compose up --build` deployment), where it is derived below from the `.git`
# copied into THIS builder stage. The runtime stage copies only the binary, so `.git`
# never reaches the final image (#225).
ARG VERSION=
# go build and `git describe` read from the .git in the build context; mark it a safe
# directory so git does not refuse it as dubious-ownership under the build user.
RUN git config --global --add safe.directory /src
# CGO off → a fully static binary (modernc sqlite and pgx are pure Go), so it runs
# on the distroless static base. -s -w strips debug info to keep it small.
# On a source build (VERSION empty), derive the nearest release tag so the instance
# reports a legible version — 0.12.0 at a tag, 0.12.0-3-gabc1234 ahead of one, with a
# -dirty suffix when the built tree was modified — instead of a bare commit (#259).
# --match keeps it to release tags (yaadegar-v*) and the prefix is stripped to a pure
# semver; --always still yields the bare commit when no tag is reachable (a shallow
# `--depth 1` clone has none), which reads as distinct-from-a-version rather than
# misleading. A non-empty VERSION (the publish path) wins untouched.
RUN VERSION="${VERSION:-$(git describe --tags --always --dirty --match 'yaadegar-v*' 2>/dev/null)}"; \
    VERSION="${VERSION#yaadegar-v}"; \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/yaadegar ./cmd/yaadegar

# --- runtime ---
# gcr.io/distroless/static-debian12:nonroot
FROM gcr.io/distroless/static-debian12@sha256:f5b485ea962d9bd1186b2f6b3a061191539b905b82ec395de78cbfae51f20e35
COPY --from=build /out/yaadegar /usr/local/bin/yaadegar
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/yaadegar"]
CMD ["serve"]
