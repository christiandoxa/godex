# CLI error parity oracle

`clierrorparity` runs a credential-free matrix of valid and invalid command
forms against the exact Prodex 0.437.7 binary and a clean Godex build. It
requires the pinned Prodex source commit and binary digest, and it checks the
candidate's embedded clean Git revision before executing any probe.

The matrix covers the profile, session, runtime information, status, and
top-level `current` delivery paths. Invalid forms must fail with a diagnostic
and the canonical parser exit status; the valid empty-profile read must
succeed. The report includes a mutation negative control that proves an
accepted invalid `current` argument would fail the comparison.

This is a narrow CLI argument contract check. It does not establish parity for
interactive TUI rendering, authentication, live quota, provider transports,
or cross-platform process supervision.
