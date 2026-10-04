# gh-argus

A terminal UI for watching GitHub Actions across many repositories at once.

## Install

```sh
gh extension install EvilNick2/gh-argus
```

Then run `gh argus`.

## Development

```sh
go build
gh extension install .
gh argus
```

`gh extension install .` links the local checkout, so each `go build` is
picked up immediately. Remove it with `gh extension remove argus`.

Releases are built by `cli/gh-extension-precompile` when a `v*` tag is pushed.
Tags containing a hyphen, such as `v0.1.0-rc.1`, publish as prereleases.

## Licence

Apache-2.0, see [LICENSE](LICENSE).
