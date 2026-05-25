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

**Provider**:
The external secret store or CLI that resolves a Secret source into Secret material.
_Avoid_: backend, vault, service

**Secret Lookup**:
The act of resolving an approved Lease's Secret source through a Provider before Grant materializes it.
_Avoid_: provider fetch, backend lookup, source read

**Daemon**:
The background process that owns active Lease state and performs scheduled revocation.
_Avoid_: worker, server, scheduler

**Config**:
The TOML declaration that describes desired Leases for a project.
_Avoid_: manifest, spec, settings

## Relationships

- A **Config** declares zero or more **Leases**.
- A **Lease** identifies exactly one **Secret** source and one **Destination**.
- A **Provider** performs **Secret Lookup** for an approved **Lease**.
- A **Grant** uses **Secret Lookup** before materializing a **Secret** at its **Destination**.
- A **Grant** registers a **Lease** with the **Daemon**.
- The **Daemon** performs **Revoke** when a **Lease** expires or is removed from **Config**.

## Example dialogue

> **Dev:** "When I run **Grant**, does every **Secret** go through **Secret Lookup** immediately?"
> **Domain expert:** "Only approved **Leases** should perform **Secret Lookup**, then the **Daemon** tracks when each **Lease** should **Revoke** its **Destination**."

## Flagged ambiguities

- "destination" and "target" were both used for where a **Secret** is written — resolved: use **Destination**.
- "config lease" and "runtime lease" both describe a **Lease** at different stages — resolved: **Config** declares raw Lease intent; the runtime Lease is the normalized form used by Grant and the Daemon.
