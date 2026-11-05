# Numeric Reference Contract Regression

This note records the historical contract protected by the accompanying test and its source.

## Contract covered

The test exercises `Read` with all-digit UUID prefixes, a prefix beginning with a digit, and the `ref=` form printed to callers. It checks that each resolves to the expected session and one anchor with the expected text. It also checks that the ordinal-looking reference `12345678:2` remains a `*ErrMsgNotFound` for UUID prefix `2`, even though the fixture contains a second message.

This is a regression guard for a historical numeric-prefix rejection. It does not establish a present runtime defect, broader numeric-reference behavior, or issue closure.

## Source and base

The complete 59-line test is adopted from `MoonCaves/rawclaw` at immutable commit `767be51bcca2bdbccf2f63c5e95d3edcff147032`, file `internal/agentproto/reference_resolution_contract_test.go` lines 1–59, under the MIT License. The fetched source SHA256 is `b9df5c73f230baa6750e0097b8e447636a65b54d4cfb2b8babb9fd5e46afada0`. The pinned source repository LICENSE SHA256 is `dadad3ec35186c90dad8eb3ea41dd323f71ff26bfd5b94096ed6dab5778f316f`.

The adoption base is `09526006b2b93788f105be492238c27705753acd`. This note records the historical guard and source provenance; it does not report test execution or validation status.
