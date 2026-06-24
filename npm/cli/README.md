# @foir/demesne-cli

The Demesne CLI, distributed as a prebuilt binary for the JavaScript toolchain. Compile one
`.demesne` spec into a Postgres RLS enforcement floor and a typed, equivalence-checked app surface.

```sh
npx @foir/demesne-cli emit app.demesne all
# or install it
npm i -D @foir/demesne-cli
```

This package is a thin launcher: on install, npm pulls the one
`@foir/demesne-cli-<platform>` optional dependency that matches your OS and CPU, and the `demesne`
bin execs that binary. No Go toolchain required.

Prefer Go? `go install github.com/eidestudio/demesne/cmd/demesne@latest`.

See the [project README](https://github.com/eidestudio/demesne#readme) for the spec grammar and
the full command reference (`demesne help`). Licensed under Apache-2.0.
