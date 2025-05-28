# Files for testing

Files with "known content" for testing migrations.


## rootchain_v0.db

Version 0 of the `rootchain.db` from live environment (21.05.2025).

Contains:
- 67 blocks (bug, old blocks are not properly cleaned out), root block (block with non nil CommitQC) for round 3513790;
- high QC round: 3513791
- high voted round: 3513792
- vote message (`VoteMsg`) for round 3513792;
- timeout certificate (`TimeoutCert`) for round 3354117;