## Summary

- 

## Validation

- [ ] `gofmt` reports no changes
- [ ] `go test -race ./...`
- [ ] `go vet ./...`
- [ ] `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...`
- [ ] Relevant installer, distribution, npm, and/or target-build checks

## Integration checklist

- [ ] ani-cli remains the owner of lookup and playback logic
- [ ] Raw ani-cli menu values are preserved when sent back upstream
- [ ] New interactions stay inside the persistent workspace
- [ ] Documentation and regression tests are updated
- [ ] No credentials, private media URLs, generated binaries, or release artifacts are included
