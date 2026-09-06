# Devctl ubiquitous language

Devctl is a non-interactive control plane for defining, validating, and
materializing reproducible Go projects. These are the canonical terms used by
manifests, commands, diagnostics, and architecture documents.

## Project model

**Project**:
A repository rooted at a selected Manifest and managed through Devctl
workflows.

**Manifest**:
The canonical desired-state document that names a Project and declares its
Components, Resources, paths, and generator policy.
_Avoid_: configuration file, when referring to the complete Project model

**Component**:
An application-facing area such as HTTP, gRPC, Kafka, logging, or telemetry.
A Component can contain Capabilities and related Resources.

**Capability**:
Enableable runtime behavior such as an HTTP server, health endpoint, or
telemetry. A Capability may have a Runtime Start Policy.

**Resource**:
A named infrastructure dependency declared by a Project, such as a database,
Redis connection, S3 bucket, or client.

**Connection**:
A named access configuration for an external system. A Connection can expose
one or more Variants.

**Variant**:
One concrete backend form of a logical Resource, such as SQLite, PostgreSQL,
or ClickHouse for a database connection.

## Effective project model

**Target**:
A stable, addressable effective unit on which `sync`, `lint`, or `gen` can
operate, such as `http-client:payments`. A Target ID is its CLI address.

**Target Catalog**:
The immutable, deterministically ordered projection of one Manifest into
effective Targets. It is the canonical source of Target identities, inputs,
outputs, references, and operation capabilities.

**Logical Input**:
The Manifest-derived Contract selection recorded by a Target before external
state is materialized.

**Resolved Input**:
The concrete Contract entrypoint or module root selected from valid committed
Snapshot Metadata.

**Readiness**:
The Project state required to execute its declared toolchain, including local
files, native generator configs, module tools, and task declarations.

## Contracts

**Contract**:
A stable description of an API or event surface: OpenAPI, Proto, JSON Schema,
or a raw Kafka message convention.

**Source**:
A named origin and containment root for Contracts. A Source is local,
URL-based, Git-based, or another Devctl Project.

**Contract Reference**:
A Component's selection of a Contract from a Source, either by a relative
Entrypoint or by a named Export.

**Export**:
A named Contract surface published by one Devctl Project for consumption by
another.

**Contract Snapshot**:
An exact, self-contained Contract closure selected from a Source, including
its Entrypoint, Module Root, files, and Snapshot Metadata when applicable.

**Entrypoint**:
The contained file through which a file-based Contract is interpreted.
_Avoid_: first file, main file

**Module Root**:
The contained directory that defines a multi-file Contract module, especially
a Proto module.
_Avoid_: entrypoint, when the Contract is selected as a module

**Snapshot Metadata**:
Committed, machine-readable facts required to interpret a Managed External
Contract without rediscovering its structure or contacting its supplier.

**Managed External Contract**:
A Contract Snapshot published by `sync` into a Project-owned namespace and
committed for review. Offline workflows consume this committed state.

## Ownership and runtime

**Managed Output**:
A file or complete tree whose contents belong entirely to Devctl and may be
atomically replaced or pruned by its owning workflow.

**Scaffold Seed**:
A file created once by scaffold and owned by the user afterwards, such as an
application entrypoint, provider binding, or handler.
_Avoid_: create-only artifact, in user-facing language

**Provider Binding**:
User-owned composition code that selects concrete application behavior or
adapts generated infrastructure to application types.

**Canonical DI Key**:
A stable, namespaced identifier for one named runtime dependency or runner.
_Avoid_: ad hoc string key

**Scenario**:
An executable application mode that owns a composed dependency graph, its
selected runtime work, and orderly shutdown.

**Runtime Config**:
The immutable, typed catalog of effective environment-backed settings derived
from the Manifest. Migration-only environment is not application Runtime
Config.

**Runtime Start Policy**:
An optional environment-backed gate for a Capability. It distinguishes
construction of a runtime dependency from starting its background work.

**Migration Target**:
The migration path and migration-only environment belonging to one SQLite,
PostgreSQL, or ClickHouse database Variant.
