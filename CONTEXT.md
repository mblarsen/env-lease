# env-lease Context

`env-lease` manages temporary local-development access to secrets by granting them into developer destinations and revoking them after a lease duration.

## Language

**Secret**:
A sensitive value fetched from an external provider for temporary local use.
_Avoid_: credential, token, value

**Lease**:
A time-limited grant of a Secret into a developer destination.
_Avoid_: entry, item, config block

**Grant**:
The act of fetching a Secret, materializing it at its destination, and registering its revocation schedule.
_Avoid_: inject, write, enable

**Revoke**:
The act of removing or clearing a granted Secret from its destination.
_Avoid_: delete, cleanup, expire

**Destination**:
The local place where a granted Secret is materialized, such as an environment file, whole file, or shell session.
_Avoid_: target, output, sink

**Destination Mutation**:
The reversible act of applying or removing a Secret at a Destination during Grant and Revoke.
_Avoid_: file write, shell output, destination handling

**Provider**:
The external secret store or CLI that resolves a Secret source into Secret material.
_Avoid_: backend, vault, service

**Secret Lookup**:
The act of resolving an approved Lease's Secret source through a Provider before Grant materializes it.
_Avoid_: provider fetch, backend lookup, source read

**Secret Transformation**:
The act of converting fetched Secret material into the final Secret or set of Secrets that Grant can materialize.
_Avoid_: post-processing, parsing, pipeline output

**Grant Workflow**:
The ordered decisions and steps that turn a desired Lease set into a Daemon registration request.
_Avoid_: grant command logic, grant orchestration, CLI flow

**Daemon**:
The background process that owns active Lease state and performs scheduled revocation.
_Avoid_: worker, server, scheduler

**Lease Lifecycle**:
The state transitions that register, expire, retry, reconcile, and revoke active Leases inside the Daemon.
_Avoid_: state handling, timer logic, daemon cleanup

**Config**:
The TOML declaration that describes desired Leases for a project.
_Avoid_: manifest, spec, settings

**Presentation**:
The command-line output adapter that renders already-derived facts as prompts, status tables, hints, and user-facing messages.
_Avoid_: command printing, formatting logic, UI glue

## Relationships

- A **Config** declares zero or more **Leases**.
- A **Lease** identifies exactly one **Secret** source and one **Destination**.
- **Destination Mutation** applies or removes a **Secret** at a **Destination**.
- A **Provider** performs **Secret Lookup** for an approved **Lease**.
- A **Grant** runs a **Grant Workflow**.
- The **Grant Workflow** uses **Secret Lookup** before **Secret Transformation**.
- **Secret Transformation** produces one **Secret** or an exploded set of Secrets for the **Grant Workflow** to materialize at their **Destination**.
- The **Grant Workflow** produces the request that registers **Leases** with the **Daemon**.
- The **Daemon** owns the **Lease Lifecycle** for active **Leases**.
- The **Lease Lifecycle** performs **Revoke** when a **Lease** expires or is removed from **Config**.
- **Presentation** consumes facts from commands, the **Grant Workflow**, and the **Daemon** without owning **Grant**, **Revoke**, or **Lease Lifecycle** business rules.

## Example dialogue

> **Dev:** "When I run **Grant**, does every **Secret** go through **Secret Lookup** immediately?"
> **Domain expert:** "The **Grant Workflow** decides which **Leases** are approved first. Only approved **Leases** perform **Secret Lookup**; then **Secret Transformation** shapes the fetched material, **Destination Mutation** applies it, and the **Daemon** owns the **Lease Lifecycle** that decides when each **Lease** should **Revoke** its **Destination**."

## Flagged ambiguities

- "destination" and "target" were both used for where a **Secret** is written — resolved: use **Destination**.
- "config lease" and "runtime lease" both describe a **Lease** at different stages — resolved: **Config** declares raw Lease intent; the runtime Lease is the normalized form used by Grant and the Daemon.
- Command files mixed user-facing formatting with command orchestration — resolved: use **Presentation** for output labels, styling, routing conventions, and common messages.
