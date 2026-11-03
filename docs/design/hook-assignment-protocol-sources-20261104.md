# Issue 89 assignment protocol: bounded upstream source review

Date: 2026-11-04

## Finding

Within the three requested repositories, Infisical is the closest match: its Go exporter accepts typed `Key`/`Value` records and formats them as POSIX `export` statements with quoted values. Its formatter is a source for generic typed-record-to-export serialization, but it is only a partial match for issue 89's hook transport. The inspected path obtains Infisical secret records, then formats the record list; it does not decode hook JSON from stdin into RawClaw's fixed editor fields or choose RawClaw's fixed output names.

A temporary probe using the pinned Infisical model and formatter source confirmed that its emitted statements round-trip seven non-NUL fixture values through `/bin/sh`, including quotes, embedded and trailing newlines, carriage return, Unicode, shell substitutions and metacharacters, and empty input. The NUL fixture did not round-trip: the shell value was `ab` for input bytes `a\x00b`.

This report establishes no donor for the complete JSON-input-to-fixed-hook-assignment protocol within this bounded corpus. It does not cover editor-native field mappings, fail-soft behavior, nullable invocation rules, or deduplication.

## Closest partial match: Infisical CLI

Pinned commit: `49634ac7f94c9ea02eb7d571fc64577cefe199e5`.

- The typed record has string fields `Key` and `Value` tagged `json:"key"` and `json:"value"` in [`packages/models/cli.go` L105-L117](https://github.com/Infisical/cli/blob/49634ac7f94c9ea02eb7d571fc64577cefe199e5/packages/models/cli.go#L105-L117).
- The export flow gets `[]models.SingleEnvironmentVariable` from [`packages/util/secrets.go` L302-L340](https://github.com/Infisical/cli/blob/49634ac7f94c9ea02eb7d571fc64577cefe199e5/packages/util/secrets.go#L302-L340), sorts those records, dispatches by format, and writes the resulting formatter string to stdout or an output file in [`packages/cmd/export.go` L145-L175](https://github.com/Infisical/cli/blob/49634ac7f94c9ea02eb7d571fc64577cefe199e5/packages/cmd/export.go#L145-L175) and [`L274-L291`](https://github.com/Infisical/cli/blob/49634ac7f94c9ea02eb7d571fc64577cefe199e5/packages/cmd/export.go#L274-L291).
- `formatAsDotEnvEval` loops over the typed records and emits `export <Key>=<quoted Value>` followed by a newline. `posixShellQuote` single-quotes the value and replaces embedded single quotes with the POSIX close/escape/reopen sequence in [`packages/cmd/export.go` L323-L341](https://github.com/Infisical/cli/blob/49634ac7f94c9ea02eb7d571fc64577cefe199e5/packages/cmd/export.go#L323-L341).
- The upstream unit fixtures cover empty input, a simple value, apostrophes, multiline values, and shell metacharacters in [`packages/cmd/export_test.go` L81-L153](https://github.com/Infisical/cli/blob/49634ac7f94c9ea02eb7d571fc64577cefe199e5/packages/cmd/export_test.go#L81-L153). Those tests check emitted strings; the actual `/bin/sh` round-trip below is a separate probe.
- The formatter interpolates `env.Key` directly in its output. The cited formatter does not validate or quote that key. The upstream key-input contract was not established here, so this observation is not a claim that Infisical accepts unsafe keys. It does mean the formatter alone does not establish arbitrary dynamic-key handling for a different caller.
- Exact role boundary: the source has generic secret-record keys, sorted record ordering, and `export` output. It does not show a JSON hook payload decoder, source-specific field extraction, or the fixed RawClaw assignment schema. Its value handling is a close source for the serialization stage, not the whole RawClaw protocol.
- Compatibility: the pinned root `go.mod` declares Go `1.26.10` at [`go.mod` L1-L5](https://github.com/Infisical/cli/blob/49634ac7f94c9ea02eb7d571fc64577cefe199e5/go.mod#L1-L5), above RawClaw's Go 1.24 minimum. The extracted formatter itself imports only `fmt` and `strings`; the full module has a much larger dependency graph. The repository root license says content outside `ee/` is MIT Expat; the cited `packages/cmd` path is outside `ee/` in [LICENSE](https://github.com/Infisical/cli/blob/49634ac7f94c9ea02eb7d571fc64577cefe199e5/LICENSE).
- NUL boundary: the formatter does not reject NUL. The actual shell probe below shows it cannot preserve that byte in a shell variable; the output becomes `ab`.

## Other bounded repositories

### 99designs/aws-vault

Pinned commit: `74e2f7ac256f4da1efbc8a48a4c0c364e454acd4`.

`ExportCommandInput` and format dispatch select an AWS credential export mode in [`cli/export.go` L19-L33](https://github.com/99designs/aws-vault/blob/74e2f7ac256f4da1efbc8a48a4c0c364e454acd4/cli/export.go#L19-L33) and [`L88-L112`](https://github.com/99designs/aws-vault/blob/74e2f7ac256f4da1efbc8a48a4c0c364e454acd4/cli/export.go#L88-L112). `printEnv` emits a fixed set of AWS credential and region names using `fmt.Printf` with raw `%s` formatting in [`cli/export.go` L189-L209](https://github.com/99designs/aws-vault/blob/74e2f7ac256f4da1efbc8a48a4c0c364e454acd4/cli/export.go#L189-L209).

This is partial fixed-field serialization. Its function consumes typed AWS credentials and a region, not arbitrary JSON hook strings. The observed formatting does not itself source quoting for arbitrary strings; this review makes no claim about whether the AWS SDK credential contract restricts those values enough for its own use. The pinned module declares Go 1.20 in [`go.mod` L1-L5](https://github.com/99designs/aws-vault/blob/74e2f7ac256f4da1efbc8a48a4c0c364e454acd4/go.mod#L1-L5); the repository license is [MIT](https://github.com/99designs/aws-vault/blob/74e2f7ac256f4da1efbc8a48a4c0c364e454acd4/LICENSE). It does not provide a JSON-input-to-arbitrary-value transport implementation.

### Doppler CLI

Pinned commit: `236908d5f8d27da251479bfad05e6bb243339208`.

The inspected `doppler run` path parses JSON secret bytes into a `map[string]string` in [`pkg/controllers/secrets.go` L590-L595](https://github.com/DopplerHQ/cli/blob/236908d5f8d27da251479bfad05e6bb243339208/pkg/controllers/secrets.go#L590-L595). `PrepareSecrets` merges those map entries and converts them to an environment slice; the caller passes that slice into the command runner in [`pkg/controllers/secrets.go` L340-L409](https://github.com/DopplerHQ/cli/blob/236908d5f8d27da251479bfad05e6bb243339208/pkg/controllers/secrets.go#L340-L409) and [`pkg/cmd/run.go` L351-L362](https://github.com/DopplerHQ/cli/blob/236908d5f8d27da251479bfad05e6bb243339208/pkg/cmd/run.go#L351-L362). The `ParseEnvStrings` helper handles existing `key=value` environment entries in [`pkg/utils/env.go` L20-L31](https://github.com/DopplerHQ/cli/blob/236908d5f8d27da251479bfad05e6bb243339208/pkg/utils/env.go#L20-L31).

This is an adjacent JSON-to-child-environment design. It injects the values through the process environment instead of emitting a POSIX assignment script, so it does not source issue 89's shell transport serialization. The inspected `doppler run` route declares JSON as its input format for environment injection at [`pkg/cmd/run.go` L182-L190](https://github.com/DopplerHQ/cli/blob/236908d5f8d27da251479bfad05e6bb243339208/pkg/cmd/run.go#L182-L190). The pinned module declares Go 1.26.10 in [`go.mod` L1-L5](https://github.com/DopplerHQ/cli/blob/236908d5f8d27da251479bfad05e6bb243339208/go.mod#L1-L5), above RawClaw's Go 1.24 minimum; the repository license is [Apache-2.0](https://github.com/DopplerHQ/cli/blob/236908d5f8d27da251479bfad05e6bb243339208/LICENSE).

## Actual `/bin/sh` probe

The temporary Go probe used the pinned Infisical model and the exact `formatAsDotEnvEval` and `posixShellQuote` source function bodies, with a fixed known-safe key `RC_TEST`. It wrote the produced bytes to a temporary file, then evaluated the file contents through `/bin/sh` command substitution. It captured the shell variable bytes and compared them with each original fixture. Shell substitution fixtures only call `printf`; they do not access services or use live credentials.

```text
apostrophe: err=<nil> stderr="" equal=true got=697427732066696e65 want=697427732066696e65 emitted="export RC_TEST='it'\\''s fine'\n"
embedded-newline: err=<nil> stderr="" equal=true got=6c696e65310a6c696e6532 want=6c696e65310a6c696e6532 emitted="export RC_TEST='line1\nline2'\n"
trailing-newline: err=<nil> stderr="" equal=true got=7461696c0a want=7461696c0a emitted="export RC_TEST='tail\n'\n"
carriage-return: err=<nil> stderr="" equal=true got=610d62 want=610d62 emitted="export RC_TEST='a\rb'\n"
unicode: err=<nil> stderr="" equal=true got=736e6f776d616e20e29883202f20e4bda0e5a5bd want=736e6f776d616e20e29883202f20e4bda0e5a5bd emitted="export RC_TEST='snowman ☃ / 你好'\n"
shell-metacharacters: err=<nil> stderr="" equal=true got=24287072696e746620696e6a65637465642920607072696e746620696e6a656374656460203b202a203f205b785d20232026207c want=24287072696e746620696e6a65637465642920607072696e746620696e6a656374656460203b202a203f205b785d20232026207c emitted="export RC_TEST='$(printf injected) `printf injected` ; * ? [x] # & |'\n"
empty: err=<nil> stderr="" equal=true got= want= emitted="export RC_TEST=''\n"
nul: err=<nil> stderr="" equal=false got=6162 want=610062 emitted="export RC_TEST='a\x00b'\n"
```

`NUL` is unsupported for shell-variable transport: `posixShellQuote` emits it literally, and the shell's command-substitution path drops it. The probe demonstrates this boundary; it does not establish handling for arbitrary output keys.

## Result boundary

Among the three named repositories, the inspected Infisical formatter is the closest partial source for typed value serialization into shell export statements. Its format-and-quote composition does not provide the JSON hook boundary or RawClaw's fixed field mapping. AWS Vault's credential-specific output and Doppler's direct child-environment injection are adjacent mechanisms, not complete matches. This bounded result does not imply that no implementation exists outside these three repositories.
