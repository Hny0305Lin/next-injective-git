# Archived repositories

This document records owner-requested repository retirement decisions whose
SHA-256 digest is anchored in the on-chain moderation transaction.

## `inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/igit-dev`

- Decision date: 2026-08-05
- Status: frozen
- Reason: this was an early import created with an unintended repository name.
- Replacement: `inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5/Huawei-IAM-Java`
- Effect: normal repository listings hide it and the contract rejects future
  ref writes. Historical metadata, transactions, and direct audit queries are
  intentionally retained because the registry is on-chain.
