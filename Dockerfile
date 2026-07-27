# Multi-stage build: a static, CGO-free binary in the builder, copied into a
# minimal non-root runtime. The binary embeds its migrations (go:embed), so the
# runtime image carries only the binary — nothing else to ship.

# --- builder ---
FROM golang:1.26.2 AS build
WORKDIR /src

# Cache module downloads separately from the source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO off → a fully static binary (modernc sqlite and pgx are pure Go), so it runs
# on the distroless static base. -s -w strips debug info to keep it small.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/yaadegar ./cmd/yaadegar

# --- runtime ---
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/yaadegar /usr/local/bin/yaadegar
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/yaadegar"]
CMD ["serve"]
