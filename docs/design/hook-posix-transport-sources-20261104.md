# Issue 89 POSIX value transport: bounded source check

Date: 2026-11-04

## Finding

The bounded source check found a reusable POSIX shell quotation primitive, not a complete JSON-to-assignment implementation. `al.essio.dev/pkg/shellescape` exposes `Quote(s string) string`; its source says the result is safe as one token in a shell command line and implements that with single-quoted text plus quote escaping. A temporary probe composed that returned token after a fixed `NAME=` prefix and passed the assignment through `/bin/sh` command substitution and `eval`. Seven non-NUL values round-tripped byte-for-byte, including apostrophes, embedded and trailing newlines, carriage return, Unicode, shell metacharacters, and empty string.

This supports only the quoted value token in a fixed-name POSIX assignment stream. It does not source JSON decoding, native editor field mappings, fail-soft branches, key schema or ordering, nullable invocation rules, `eval` policy, or deduplication. Those remain separate provenance questions.

## Donors checked

### Reusable candidate: alessio/shellescape

- Immutable source: [`shellescape.go` L30-L42](https://github.com/alessio/shellescape/blob/693bd3506cecc3d8e643dc686dcf00adb56b4a76/shellescape.go#L30-L42), commit `693bd3506cecc3d8e643dc686dcf00adb56b4a76`.
- License: [LICENSE](https://github.com/alessio/shellescape/blob/693bd3506cecc3d8e643dc686dcf00adb56b4a76/LICENSE), MIT.
- API contract: `Quote` returns one shell token; empty input becomes `''`. Values containing characters outside its unquoted allowlist are wrapped in single quotes, and embedded apostrophes are represented by closing and reopening quotes around an escaped quote.
- Compatibility evidence: the pinned module declares Go 1.18 in [`go.mod` L1-L5](https://github.com/alessio/shellescape/blob/693bd3506cecc3d8e643dc686dcf00adb56b4a76/go.mod#L1-L5). The quoted implementation imports only Go standard-library packages. This is a pure-Go source primitive; the repository module declares an additional Go module requirement, so dependency-graph impact should be checked before adoption.
- Limitation: `Quote` does not reject NUL. It returns a string containing the NUL byte. POSIX shell variables and process argument strings cannot carry NUL, so this primitive cannot transport a JSON-decoded value containing NUL losslessly. The probe's attempt to pass such a payload as `/bin/sh` argv failed with `fork/exec /bin/sh: invalid argument`.

### Rejected for multiline values: mvdan/sh

- Immutable source: [`syntax/quote.go` L29-L42, L71-L78](https://github.com/mvdan/sh/blob/5943e49d40d22467e60056a0f39ba58fb8fc767a/syntax/quote.go#L29-L42), commit `5943e49d40d22467e60056a0f39ba58fb8fc767a`.
- License: [LICENSE](https://github.com/mvdan/sh/blob/5943e49d40d22467e60056a0f39ba58fb8fc767a/LICENSE), BSD 3-Clause.
- API contract: `syntax.Quote(s, lang)` quotes one word and returns `QuoteError` for unsupported values. The POSIX variant rejects non-printable runes, including newline; the pinned test explicitly expects a newline error in [`syntax/quote_test.go` L31-L32](https://github.com/mvdan/sh/blob/5943e49d40d22467e60056a0f39ba58fb8fc767a/syntax/quote_test.go#L31-L32). It therefore does not fit arbitrary multiline values.
- Compatibility limit: this pinned module declares Go 1.26.0 in [`go.mod` L1-L3](https://github.com/mvdan/sh/blob/5943e49d40d22467e60056a0f39ba58fb8fc767a/go.mod#L1-L3), above RawClaw's Go 1.24 minimum.

### Also viable for one assignment value: kballard/go-shellquote

- Immutable source: [`quote.go` L9-L20, L23-L27, L29-L101](https://github.com/kballard/go-shellquote/blob/95032a82bc518f77982ea72343cc1ade730072f0/quote.go#L9-L20), commit `95032a82bc518f77982ea72343cc1ade730072f0`.
- License: [LICENSE](https://github.com/kballard/go-shellquote/blob/95032a82bc518f77982ea72343cc1ade730072f0/LICENSE), MIT.
- API contract: `Join(args...)` quotes a list into a command line whose words split back into the original arguments. The pinned implementation handles empty strings and literal newlines. Although its API is list/command-line composition, a one-value use is valid after a fixed `NAME=` prefix: a probe of `NAME=` plus `Join(value)` round-tripped seven non-NUL values through `/bin/sh`, including embedded and trailing newlines, CR, Unicode, apostrophes, empty input, and shell metacharacters. This is a tested alternate for the narrow assignment-value seam.
- Limitation: the implementation does not reject NUL, so it has the same shell transport boundary for NUL values.

## Probe result

Using the pinned `alessio/shellescape` source bytes, the Go probe emitted `NAME=` plus `Quote(value)` and a line feed, then evaluated the payload through `/bin/sh` command substitution. The probe compared the resulting shell variable to the original bytes.

```text
apostrophe: err=<nil> equal=true got=697427732066696e65 want=697427732066696e65 emitted="X='it'\"'\"'s fine'\n"
embedded-newline: err=<nil> equal=true got=610a62 want=610a62 emitted="X='a\nb'\n"
trailing-newline: err=<nil> equal=true got=7461696c0a want=7461696c0a emitted="X='tail\n'\n"
carriage-return: err=<nil> equal=true got=610d62 want=610d62 emitted="X='a\rb'\n"
unicode: err=<nil> equal=true got=736e6f776d616e20e29883202f20e4bda0e5a5bd want=736e6f776d616e20e29883202f20e4bda0e5a5bd emitted="X='snowman ☃ / 你好'\n"
shell-metacharacters: err=<nil> equal=true got=2428746f75636820697373756538392d6d61726b6572293b20606964603b202a203f205b785d20232026207c203b want=2428746f75636820697373756538392d6d61726b6572293b20606964603b202a203f205b785d20232026207c203b emitted="X='$(touch issue89-marker); `id`; * ? [x] # & | ;'\n"
empty: err=<nil> equal=true got= want= emitted="X=''\n"
nul: err=fork/exec /bin/sh: invalid argument equal=false got= want=610062 emitted="X='a\x00b'\n"
```

The metacharacter case did not create its `touch` marker. The embedded and trailing newline cases both preserved the newline byte; command substitution removed only the transport line ending after the closing quote.

## Evidence boundary

This is a bounded comparison of the three named repositories, not a claim that no other source exists. A quotation primitive does not establish RawClaw's native JSON field mapping, source-specific branches, fail-soft behavior, output keys or order, nullable invocation policy, or deduplication. It also does not establish that concatenating the assignment stream and evaluating it is an authorized protocol design.
