# Separate refreshable outputs from user-owned Seeds

Status: Accepted
Date: 2026-09-03

Generated infrastructure needs deterministic refresh, while application code
must retain user intent. Devctl may replace complete Managed Outputs but creates
Scaffold Seeds only when absent, so ownership determines what refresh can
change. Publication is atomic at its file or tree boundary, and a later failure
preserves completed work instead of attempting workflow-wide rollback.

## Consequences

Users keep handwritten code outside managed trees and inspect partial results
before retrying. Seed creation accepts the documented external-writer race;
it is not an atomic cross-process create-if-absent guarantee.

The rationale is recorded in
[ADR 0003](../../adr/0003-managed-artifacts-and-publication.md). Current
behavior is described in [Scaffold and ownership](../scaffold-and-ownership.md).
