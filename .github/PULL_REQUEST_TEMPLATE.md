<!-- Keep the title imperative and scoped: "engine: read FD limits from /proc/<pid>/limits". -->

## What changes

## Why

## How to verify it

```sh
make lint test race build
```

- [ ] `go test ./...` passes locally, with `-race` on the packages you touched.
- [ ] I ran the TUI (`make install && vibe-check`) and looked at what I changed.
- [ ] Every number I wrote in this PR or in the docs came from a real run, not a guess.
- [ ] README / CHANGELOG updated if behaviour or the platform matrix changed.
- [ ] New collectors have a fallback for the platforms that do not expose them.

## Platform impact

| OS | Tested | Notes |
|---|---|---|
| Linux | | |
| macOS | | |
| Windows | | |

## Risk

What could break, and how it gets reverted.
