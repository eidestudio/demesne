/**
 * Hand-written projection literals mirroring the specs the Go unit tests parse, so the
 * pure-compute ports can be exercised without the emitter (which arrives in a later
 * increment). Each corresponds to a spec in the Go test suite — names noted inline.
 */

import type { Vocabulary, HoldsResolver, Pdp, Claims } from "../src/index.js";

/** The `roles` vocabulary from holds_test.go's `holdsSpec` (nested preset + star + rank). */
export const rolesVocab: Vocabulary = {
  name: "roles",
  permissions: ["docs:read", "docs:write", "docs:publish", "admin:read", "admin:write"],
  presets: [
    { name: "viewer", star: false, set: ["docs:read", "admin:read"] },
    { name: "editor", star: false, set: ["viewer", "docs:write", "docs:publish"] },
    { name: "owner", star: true, set: [] },
  ],
  rank: ["owner", "editor", "viewer"],
};

/** The materialized-permissions resolver from holds_test.go's `holdsSpec`. */
export const rolesResolver: HoldsResolver = {
  assignments: "role_assignments",
  kindCol: "principal_kind",
  kindVal: "member",
  subjectCol: "principal_id",
  scopeCols: ["tenant_id", "team_id"],
  revokedCol: "revoked_at",
  roleCol: "role_id",
  rolesTable: "roles_tbl",
  rolesId: "id",
  keyCol: "key",
  permsCol: "perms",
  vocab: rolesVocab,
};

/** The same resolver with no materialized column — role keys expand through the vocabulary. */
export const rolesResolverNoPerms: HoldsResolver = { ...rolesResolver, permsCol: "" };

/** The `admin` cap vocabulary from delegation_test.go's `capVocabSpec`. */
export const capVocab: Vocabulary = {
  name: "admin",
  permissions: ["a:read", "a:write", "b:read", "b:write"],
  presets: [
    { name: "viewer", star: false, set: ["a:read", "b:read"] },
    { name: "editor", star: false, set: ["viewer", "a:write"] },
    { name: "owner", star: true, set: [] },
  ],
  rank: ["owner", "editor", "viewer"],
};

/** The `admin` PDP from runtime_test.go's `runtimeSpec`. */
export const adminPdp: Pdp = {
  emitSite: "admin",
  policy: { "records.v1.RecordsService/UpdateRecord": "content:write" },
  ungoverned: { "records.v1.RecordsService/GetRecord": "read path" },
};

/** The claims projection for runtime_test.go / session_test.go's `runtimeSpec` (no claims block → defaults). */
export const runtimeClaims: Claims = {
  setting: "request.jwt.claims",
  cast: "json",
  role: "authenticated",
  contract: ["customer_id", "project_id", "sub", "tenant_id"],
  entries: [
    { key: "customer_id", level: "", subjects: ["customer"] },
    { key: "project_id", level: "project", subjects: [] },
    { key: "sub", level: "", subjects: ["admin"] },
    { key: "tenant_id", level: "tenant", subjects: [] },
  ],
  subjects: [
    { name: "admin", identifies: "sub" },
    { name: "customer", identifies: "customer_id" },
  ],
  levels: [
    { name: "tenant", claimKey: "tenant_id", virtual: false },
    { name: "project", claimKey: "project_id", virtual: false },
  ],
};

/** session_test.go's `virtualRootSpec` — a spec-declared GUC/role + a VIRTUAL root level. */
export const virtualRootClaims: Claims = {
  setting: "app.ctx",
  cast: "jsonb",
  role: "app_user",
  contract: ["sub", "tenant_id"],
  entries: [
    { key: "sub", level: "", subjects: ["admin"] },
    { key: "tenant_id", level: "tenant", subjects: [] },
  ],
  subjects: [{ name: "admin", identifies: "sub" }],
  levels: [
    { name: "platform", claimKey: "platform_id", virtual: true },
    { name: "tenant", claimKey: "tenant_id", virtual: false },
  ],
};

/** session_test.go's claim-key override spec (`claim org_ref` / `claim team_ref`, `identifies who`). */
export const overrideClaims: Claims = {
  setting: "request.jwt.claims",
  cast: "json",
  role: "authenticated",
  contract: ["org_ref", "team_ref", "who"],
  entries: [
    { key: "org_ref", level: "org", subjects: [] },
    { key: "team_ref", level: "team", subjects: [] },
    { key: "who", level: "", subjects: ["admin"] },
  ],
  subjects: [{ name: "admin", identifies: "who" }],
  levels: [
    { name: "org", claimKey: "org_ref", virtual: false },
    { name: "team", claimKey: "team_ref", virtual: false },
  ],
};

/** A subject with NO identity key (session_test.go's TestSession_BuildClaims_NoIdentityKey). */
export const noIdentityClaims: Claims = {
  setting: "request.jwt.claims",
  cast: "json",
  role: "authenticated",
  contract: [],
  entries: [],
  subjects: [{ name: "svc", identifies: "" }],
  levels: [{ name: "tenant", claimKey: "tenant_id", virtual: false }],
};
