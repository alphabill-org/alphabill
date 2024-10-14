# Custom Alphabill Predicate

This directory contains precompiled WASM modules for the
["Transferable Conference Tickets"](https://guardtime.atlassian.net/wiki/spaces/AB/pages/3538419747/Example+Use-Case+Transferable+Conference+Tickets)
example use-case, implemented in 
[alphabill-experiments repository](https://gitdc.ee.guardtime.com/alphabill/alphabill-experiments/-/tree/master/rust-sdk/predicates/conference-tickets).

These are meant for testing, for both unit-testing backward compatibility and
for testers (ie don't have to install Rust in order to compile the predicates
from source).

Different versions are organized into subdirectories, each containing particular
version of the predicates. Keep the changelog in this document!

Filenames use pattern `<unit>-<predicate>.wasm` so
- `token-bearer.wasm` implements bearer predicate to be used with token

while

- `type-bearer.wasm` implements bearer predicate to be used with token type.

## V2: 07.08.2024
- host API `tx_signed_by_pkh` returns status code instead of boolean allowing to
  optimize `token_bearer` and `token_update_data` predicates;

## V2: 21.06.2024
- switching to "Tickets created by the conference organizer" version;

## V1: 15.05.2024

Initial release:
- implementing "baseline version";
- all data structures in SDK request version `1` from host;
- round number is used as "current time";
- tx payload type is string (plan is to switch to int?);
- reference number of the mint predicate doesn't include typeID;

Files in directory:
- `bearer.wasm` exports only `bearer_invariant`;
- `mint.wasm` exports only `mint_token`;
- `update.wasm` exports only `update_data`;
- `conf_tickets.wasm` exports all three entrypoints;
