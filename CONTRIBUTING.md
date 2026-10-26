# Contributing

Issues and pull requests are welcome. Bug reports should include the Go version, deployment method, sanitized configuration, sample upstream response shape (never credentials), and expected versus actual behavior.

1. Fork the repository and create a focused branch.
2. Install Go 1.23 or newer, then run `go test -race ./...`, `go vet ./...`, and `gofmt -w main.go main_test.go`.
3. Use Conventional Commits (`feat:`, `fix:`, `perf:`, `test:`, `docs:`, `chore:`), with small atomic changes.
4. Open a PR describing how you verified the behavior, especially any authentication/cache changes.

## Performance changes

Performance contributions should include reproducible before/after numbers and keep all authorization, stale-catalog, and error-handling tests passing. Use `go test -bench . -benchmem` to inspect conversion allocations. The cache has a 30-second default TTL; never share a cached catalog across caller-provided credentials.

Please be respectful and constructive. See [SECURITY.md](SECURITY.md) for vulnerability reports.
