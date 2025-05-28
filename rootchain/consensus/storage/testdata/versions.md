
## Version 1

Buckets
- `blocks`: keys are round numbers (uint64, big endian) and value is CBOR of the `ExecutedBlock`;
- `certificates`: key `tc` value is CBOR encoded `TimeoutCert` struct;
- `votes`: key `vote` CBOR encoded struct which contains vote type tag (timeout or consensus) and vote struct data;
- `safety`: keys `votedRound` and `qcRound` values are big endian uint64 respectively "highest voted round" and "highest QC round";
- `metadata`: Contains key `version` which stores current database version as 64 bit uint (big-endian);


## Version 0

Bucket `default` contains keys:
- `block_N`: where `N` is round number (64bit uint bytes in big-endian). Data is CBOR of the `ExecutedBlock`;
- `tc`: latest timeout certificate (CBOR encoded `TimeoutCert` struct);
- `vote`: latest timeout vote or consensus vote, value is CBOR which has a tag for vote type and raw CBOR of the vote struct;
- `votedRound` - CBOR encoded int of highest voted round;
- `qcRound` - CBOR encoded int of highest QC round;
