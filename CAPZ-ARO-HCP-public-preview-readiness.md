# CAPZ Readiness for ARO-HCP Public Preview

## Overview

This document defines the work required on the CAPZ (Cluster API Provider Azure) and ASO (Azure Service Operator) side to support the ARO-HCP (Azure Red Hat OpenShift - Hosted Control Plane) public preview.

**Current state:**
- ARO-HCP support is implemented in **downstream forks** (stolostron/cluster-api-provider-azure, stolostron/azure-service-operator)
- Latest supported API version: `2025-12-23-preview` (`v1api20251223preview`)
- CAPZ ARO feature gate: **alpha** (disabled by default, introduced in v1.19)
- Three CAPZ CRDs: `AROCluster`, `AROControlPlane`, `AROMachinePool` (v1beta2)
- Three ASO resource types: `HcpOpenShiftCluster`, `HcpOpenShiftClustersNodePool`, `HcpOpenShiftClustersExternalAuth`
- Downstream release branches for MCE: `backplane-2.11`, `backplane-2.17` (based on release-1.22 and used for Adobe now), `main` (based on release-1.23 and FF-synced into `backplane-5.0`, `backplane-5.1`)

**Target state:**
- ARO-HCP support contributed **upstream** to Azure/azure-service-operator and kubernetes-sigs/cluster-api-provider-azure
- **Same ASO with the same ARO-HCP API specification** used for both upstream and the ARO-HCP public preview downstream — no divergence between upstream and downstream API types
- Downstream forks synced from upstream, eliminating the need for maintaining parallel implementations

**API version note:** The `2025-12-23-preview` API version (`v1api20251223preview`) is the only candidate for the public preview. No new API version (e.g., `v1api2026XXXX`) will be created. Some changes to the `2025-12-23-preview` spec are expected before it becomes the official public preview API.

**Key blocker:** [ARO-22049](https://redhat.atlassian.net/browse/ARO-22049) - The updated `2025-12-23-preview` API spec is not yet pushed to azure-rest-api-specs. Expected end of July 2026.

---

## 1. Pre-requisites (What We Need from Other Teams)

### From ARO HCP Team

| # | Dependency | Description | Jira | Expected |
|---|-----------|-------------|------|----------|
| 1 | **Updated `2025-12-23-preview` API spec in azure-rest-api-specs** | The updated ARO-HCP `2025-12-23-preview` API specification must be pushed to the [Azure/azure-rest-api-specs](https://github.com/Azure/azure-rest-api-specs) repository. The spec source is currently maintained at [Azure/ARO-HCP](https://github.com/Azure/ARO-HCP/tree/main/api/redhatopenshift/resource-manager/Microsoft.RedHatOpenShift/hcpclusters/preview/2025-12-23-preview). This is the **single critical dependency** that gates all upstream work for both ASO and CAPZ. No new API version will be created — `2025-12-23-preview` is the public preview API version. | [ARO-22049](https://redhat.atlassian.net/browse/ARO-22049) | End of July 2026 |
| 2 | **Changelog of changes within `2025-12-23-preview`** | Documentation of what changed in the `2025-12-23-preview` spec compared to the version we currently use. We need to assess the impact on our existing generated types, controllers, CRDs, and custom extensions. | - | With API spec |
| 3 | **ARO-HCP dev environment access** | Access to the ARO-HCP development environment for end-to-end testing of CAPZ-provisioned clusters against the new API. | [ARO-25085](https://redhat.atlassian.net/browse/ARO-25085) | Now (ongoing) |
| 4 | **Public preview deployment region(s)** | Confirmation of Azure regions where the public preview API will be available for testing and validation. | - | Before Phase 1 |

### From ASO Upstream Maintainers (Azure/azure-service-operator)

| # | Dependency | Description | Expected |
|---|-----------|-------------|----------|
| 1 | **Review and merge updated `v1api20251223preview` PR** | Once we regenerate types from the updated `2025-12-23-preview` spec and submit a PR, ASO maintainers need to review and merge it. | Phase 2 |
| 2 | **Code generation pipeline compatibility** | Confirm ASO's code generation pipeline handles the updated spec without modifications. | Phase 2 |

### From CAPZ Upstream Maintainers (kubernetes-sigs/cluster-api-provider-azure)

| # | Dependency | Description | Jira | Expected |
|---|-----------|-------------|------|----------|
| 1 | **Proposal acceptance** | The upstream CAPZ proposal for ARO-HCP support must be accepted by the community. | [ARO-24155](https://redhat.atlassian.net/browse/ARO-24155), [ARO-23324](https://redhat.atlassian.net/browse/ARO-23324) | Phase 0-3 |
| 2 | **Review and merge ARO-HCP PR** | Once ASO upstream has the new API types, CAPZ maintainers need to review and merge the ARO-HCP implementation. | - | Phase 3 |

---

## 2. What Will Be Done on ASO + CAPZ for Public Preview

### 2.1 ASO (Azure Service Operator)

**Repository:** stolostron/azure-service-operator (upstream: Azure/azure-service-operator)

| # | Task | Description | Jira |
|---|------|-------------|------|
| 1 | **Regenerate `v1api20251223preview` types** | Run ASO code generation pipeline against the updated `2025-12-23-preview` spec from azure-rest-api-specs. This updates the existing Go types for `HcpOpenShiftCluster`, `HcpOpenShiftClustersNodePool`, and `HcpOpenShiftClustersExternalAuth` in the existing `v1api20251223preview` package. No new package is created. | - |
| 2 | **Update custom extensions** | Update `hcp_open_shift_cluster_extension.go` for any pre-reconciliation checks or admin credential secret export changes required by the spec changes. | - |
| 3 | **Update API version conversion** | Update `ConvertFrom`/`ConvertTo` logic in `v1api20251223preview` if the spec changes affect fields involved in conversion with `v1api20240610preview`. | - |
| 4 | **Update unit and integration tests** | Update generated tests and add new test cases for any changed or new fields in the updated spec. | - |
| 5 | **Submit upstream PR** | Submit a PR to Azure/azure-service-operator with the regenerated types and updated custom extensions. | - |

### 2.2 CAPZ (Cluster API Provider Azure)

**Repository:** stolostron/cluster-api-provider-azure (upstream: kubernetes-sigs/cluster-api-provider-azure)

| # | Task | Description | Jira |
|---|------|-------------|------|
| 1 | **Update CRD API references** | Update `AROControlPlane` and `AROMachinePool` if the updated `v1api20251223preview` types introduce changed or new fields in the resource schema. The API version package remains the same. | - |
| 2 | **Update controllers and reconcilers** | Modify `AROControlPlane`, `AROCluster`, and `AROMachinePool` reconcilers for any changed fields or behavior in the updated `2025-12-23-preview` spec. | - |
| 3 | **Update mutators** | Update resource mutators that inject defaults and dynamic values before ASO resource creation if the spec changes affect mutated fields. | - |
| 4 | **Update ResourceReconciler** | If the spec changes affect resource status reporting or conditions, update the generic `ResourceReconciler` accordingly. | - |
| 5 | **Update tests** | Update unit tests and controller tests for any changed fields or behavior. | - |
| 6 | **Submit upstream PR** | Submit a PR to kubernetes-sigs/cluster-api-provider-azure with the ARO-HCP implementation using the updated `v1api20251223preview` types. Requires the upstream proposal to be accepted and ASO upstream to have the updated types merged. | [ARO-24155](https://redhat.atlassian.net/browse/ARO-24155) |

### 2.3 cluster-api-installer

**Repository:** stolostron/cluster-api-installer

| # | Task | Description |
|---|------|-------------|
| 1 | **Update Helm charts** | Update Helm chart versions for ASO and CAPZ to reference the new upstream releases containing public preview support. |
| 2 | **Update Kustomize configs** | Update Kustomize transformations if CRD or webhook changes require it. |
| 3 | **Update example manifests** | Update `aro-hcp.yaml` and deployment scripts to reflect any changed fields in the updated `2025-12-23-preview` spec. |

### 2.4 capi-tests

**Repository:** stolostron/capi-tests

| # | Task | Description | Jira |
|---|------|-------------|------|
| 1 | **Update e2e tests for spec changes** | Update the 8-phase test suite to validate ARO-HCP provisioning against the updated `2025-12-23-preview` public preview API. | [ARO-25085](https://redhat.atlassian.net/browse/ARO-25085) |
| 2 | **PROW CI integration** | Complete the reusable PROW step using kind/k3s as management cluster for automated e2e testing. | [ARO-25085](https://redhat.atlassian.net/browse/ARO-25085) |

---

## 3. Upstream / Downstream Correspondence

### Repository Mapping

| Component | Downstream (stolostron) | Upstream | Relationship |
|-----------|------------------------|----------|-------------|
| ASO | [stolostron/azure-service-operator](https://github.com/stolostron/azure-service-operator) | [Azure/azure-service-operator](https://github.com/Azure/azure-service-operator) | Fork with ARO-HCP CRD types added via code generation. Once upstream accepts the public preview API types, the fork syncs from upstream and downstream-only patches are minimized. |
| CAPZ | [stolostron/cluster-api-provider-azure](https://github.com/stolostron/cluster-api-provider-azure) | [kubernetes-sigs/cluster-api-provider-azure](https://github.com/kubernetes-sigs/cluster-api-provider-azure) | Fork with 3 ARO CRDs, controllers, and reconcilers behind the `ARO` feature gate (alpha). Once upstream accepts the proposal and PR, the fork syncs from upstream. |
| cluster-api-installer | [stolostron/cluster-api-installer](https://github.com/stolostron/cluster-api-installer) | N/A | No upstream counterpart. Helm charts and Kustomize configs for MCE deployment. |
| capi-tests | [stolostron/capi-tests](https://github.com/stolostron/capi-tests) | N/A | No upstream counterpart. E2E test suite. |

### Key Principle: Same ASO, Same API Specification

The ARO-HCP public preview API types in ASO must be **identical** between upstream and downstream. There is no separate "downstream API version" — the public preview API spec from `Azure/azure-rest-api-specs` is used to generate types that go directly into upstream `Azure/azure-service-operator`, and downstream syncs from upstream. This ensures:
- No API divergence between what customers use via upstream ASO and what runs in MCE
- No dual maintenance of generated types
- Adobe and other customers can eventually consume upstream ASO directly

**Implication for the timeline:** The downstream public preview (Phase 1) uses the same regenerated `v1api20251223preview` code that is later submitted upstream (Phase 2). The ASO upstream PR is the **source of truth** long-term — after upstream merges (Phase 4), downstream syncs from it, eliminating the parallel implementation.

### What Goes Upstream vs. Stays Downstream

| Upstream (identical in both) | Downstream only |
|------------------------------|----------------|
| ASO: Public preview API types (HcpOpenShiftCluster, NodePool, ExternalAuth) — **same code in upstream and downstream** | ASO: Stolostron Dockerfile (UBI9-based image for MCE) |
| ASO: Custom extensions for ARO-HCP resources | ASO: MCE-specific release branches (backplane-2.17, 5.0, 5.1) |
| CAPZ: AROCluster, AROControlPlane, AROMachinePool CRDs and controllers | CAPZ: MCE-specific release branches |
| CAPZ: ARO feature gate, ResourceReconciler, mutators | CAPZ: MCE-specific Konflux build configuration |
| CAPZ: ARO-HCP proposal and design documentation | cluster-api-installer: Helm charts, Kustomize configs |
| | capi-tests: E2E test suite |

### Downstream Release Branches

| Branch | MCE Version | Notes |
|--------|------------|-------|
| `backplane-2.11` | MCE 2.11 / ACM 2.16 | Supported release with 2024-06-10-preview only |
| `backplane-2.17` | MCE 2.17 / ACM 2.17 | Current development |
| `backplane-5.0` | MCE 5.0 | New branch ([ARO-26556](https://redhat.atlassian.net/browse/ARO-26556)) |
| `backplane-5.1` | MCE 5.1 | New branch ([ARO-26556](https://redhat.atlassian.net/browse/ARO-26556)) |

---

## 4. Dependencies for Creating Upstream PRs

### ASO Upstream PR — Dependency Chain

```
Azure/azure-rest-api-specs          (1) Updated 2025-12-23-preview spec pushed
        |
        v
ASO code generation pipeline        (2) Regenerate v1api20251223preview types from updated spec
        |
        v
Azure/azure-service-operator PR     (3) Submit PR with regenerated types + updated custom extensions
        |
        v
ASO upstream release                (4) New ASO release with updated v1api20251223preview types
        |                                (this is the SINGLE SOURCE OF TRUTH)
        v
stolostron/azure-service-operator   (5) Downstream syncs from upstream — no parallel implementation
```

**Specific requirements:**
- The updated `2025-12-23-preview` spec must be in the `Azure/azure-rest-api-specs` repository in the standard ARM format
- ASO's code generation must be able to process the updated spec (verify compatibility with `azure-arm.yaml` configuration)
- Custom extensions (`hcp_open_shift_cluster_extension.go`) must be adapted for any spec changes
- **The same regenerated `v1api20251223preview` types are used upstream and downstream** — no separate downstream API version

### CAPZ Upstream PR — Dependency Chain

```
CAPZ upstream proposal accepted     (1) ARO-24155 / ARO-23324 — community agrees on design
        |
        v
ASO upstream release with           (2) CAPZ depends on updated v1api20251223preview ASO types
updated v1api20251223preview
        |
        v
kubernetes-sigs/cluster-api-        (3) Submit PR with ARO CRDs, controllers, reconcilers,
provider-azure PR                       feature gate, tests
        |
        v
CAPZ upstream release               (4) New CAPZ release with ARO-HCP support
```

**Specific requirements:**
- The upstream proposal ([ARO-24155](https://redhat.atlassian.net/browse/ARO-24155)) must be accepted before submitting the implementation PR
- The ASO upstream release must include the updated `v1api20251223preview` types (CAPZ imports ASO types)
- The `ARO` feature gate starts as alpha (disabled by default), with a graduation plan to beta/GA
- The existing downstream proposal document (`docs/proposals/20250425-aro-hcp.md`) serves as the basis for the upstream proposal
- Tests must pass against real Azure infrastructure with the public preview API

---

## 5. Timeline

### Phase 0: Preparation (Now - July 2026)

Work that can be done **before** the API spec is available:

| Task | Description | Jira | Status |
|------|-------------|------|--------|
| PROW CI pipeline | Establish reusable PROW step with kind/k3s for CAPZ e2e tests | [ARO-25085](https://redhat.atlassian.net/browse/ARO-25085) | In progress |
| Dedicated quay.io org | Set up dedicated CAPZ quay.io organization for image publishing | [ARO-26301](https://redhat.atlassian.net/browse/ARO-26301) | In progress |
| Retry backoff | Implement useAgent header and retry backoff to prevent subscription quota exhaustion | [ARO-24154](https://redhat.atlassian.net/browse/ARO-24154), [ARO-26531](https://redhat.atlassian.net/browse/ARO-26531) | In progress |
| Upstream proposal draft | Prepare the CAPZ upstream proposal for ARO-HCP based on the existing downstream proposal | [ARO-24155](https://redhat.atlassian.net/browse/ARO-24155), [ARO-23324](https://redhat.atlassian.net/browse/ARO-23324) | To do |
| Security hardening | Complete security fixes for CAPZ and ASO | [ARO-25822](https://redhat.atlassian.net/browse/ARO-25822) | Done |
| Adobe support | Continue addressing Adobe production issues | - | Ongoing |

### Phase 1: Downstream ARO-HCP Public Preview (API spec available, ~end of July 2026)

**Trigger:** Updated `2025-12-23-preview` API spec pushed to Azure/azure-rest-api-specs

This phase delivers the ARO-HCP public preview in the downstream stolostron forks. The **same `v1api20251223preview` types** regenerated here will later be submitted upstream — no divergence.

| Step | Task | Duration estimate |
|------|------|-------------------|
| 1.1 | Analyze changes in the updated `2025-12-23-preview` spec vs current version | 1-2 days |
| 1.2 | Regenerate `v1api20251223preview` types via ASO code generation pipeline | 1-2 days |
| 1.3 | Update custom extensions and conversion logic for changed fields | 2-3 days |
| 1.4 | Update stolostron/azure-service-operator with the regenerated `v1api20251223preview` types | 1-2 days |
| 1.5 | Update stolostron/cluster-api-provider-azure controllers, mutators, and reconcilers for changed fields | 3-5 days |
| 1.6 | Update MCE release branches (backplane-2.17, 5.0, 5.1) | 2-3 days |
| 1.7 | Update cluster-api-installer Helm charts and example manifests | 1-2 days |
| 1.8 | Trigger Konflux builds and verify | 1-2 days |
| 1.9 | Run e2e tests on ARO-HCP DEV environment | 2-3 days |
| 1.10 | Run e2e tests on ARO-HCP STAGE environment | 2-3 days |
| 1.11 | Run e2e tests on ARO-HCP PROD environment | 2-3 days |
| 1.12 | Adobe customer validation on downstream public preview | 1-2 weeks |

**Key principle:** The downstream release uses the exact same `v1api20251223preview` code that will be submitted upstream in Phase 2. When upstream eventually merges (Phase 4), the downstream sync is a no-op for API types — only upstream-side changes from review feedback need to be picked up.

### Phase 2: ASO Upstream (after downstream public preview delivered)

**Trigger:** Downstream public preview validated (Phase 1 complete)

| Step | Task | Duration estimate |
|------|------|-------------------|
| 2.1 | Submit PR to Azure/azure-service-operator with the regenerated `v1api20251223preview` types and custom extensions (same code as downstream) | 1 day |
| 2.2 | Address review feedback | 1-2 weeks |

### Phase 3: CAPZ Upstream (ASO upstream PR merged)

**Trigger:** ASO upstream release with updated `v1api20251223preview` types

| Step | Task | Duration estimate |
|------|------|-------------------|
| 3.1 | Update CAPZ to use the updated `v1api20251223preview` ASO types from the upstream release | 2-3 days |
| 3.2 | Update controllers, mutators, and reconcilers for changed fields (largely done in Phase 1, adapt for upstream context) | 3-5 days |
| 3.3 | Update and run tests | 3-5 days |
| 3.4 | Submit upstream proposal (if not already accepted) | 1 day |
| 3.5 | Submit PR to kubernetes-sigs/cluster-api-provider-azure with ARO-HCP support using `v1api20251223preview` | 1 day |
| 3.6 | Address review feedback | 2-4 weeks |

### Phase 4: Downstream Sync from Upstream (Both upstream PRs merged)

**Trigger:** Both ASO and CAPZ upstream releases available

Since Phase 1 already delivered the downstream public preview using the same code, this phase only picks up any changes that resulted from the upstream review process. If no review-driven changes were made, this is effectively a no-op.

| Step | Task | Duration estimate |
|------|------|-------------------|
| 4.1 | Sync stolostron/azure-service-operator from upstream (pick up any review-driven changes) | 1 day |
| 4.2 | Sync stolostron/cluster-api-provider-azure from upstream (pick up any review-driven changes) | 1 day |
| 4.3 | Update MCE release branches if needed | 1-2 days |
| 4.4 | Trigger Konflux builds and verify | 1 day |

### Phase 5: Final Validation

| Step | Task | Duration estimate |
|------|------|-------------------|
| 5.1 | Run e2e tests on ARO-HCP PROD environment | 2-3 days |
| 5.2 | Adobe customer validation | 1-2 weeks |
| 5.3 | Fix issues discovered during validation | Ongoing |

### Phase Dependency Diagram

```
Phase 0 (Now)          Phase 1                  Phase 2            Phase 3              Phase 4           Phase 5
Preparation      -->   Downstream          -->  ASO Upstream  -->  CAPZ Upstream   -->  Upstream     -->  Final
                       ARO-HCP Public            PR submitted       PR submitted         Sync              Validation
                       Preview                   |                  |                    (review deltas)
                       |                         |                  |
                  [API spec in              [downstream         [ASO upstream
                   azure-rest-               validated]           release
                   api-specs]                                     available]
```

---

## Appendix: Related Jira Issues

| Jira | Title | Phase |
|------|-------|-------|
| [ARO-22049](https://redhat.atlassian.net/browse/ARO-22049) | ARO HCP public preview | Blocker |
| [ARO-25799](https://redhat.atlassian.net/browse/ARO-25799) | CAPZ Readiness for ARO HCP Public Preview | All |
| [ARO-24155](https://redhat.atlassian.net/browse/ARO-24155) | Upstream CAPZ proposal for ARO-HCP | Phase 0, 3 |
| [ARO-23324](https://redhat.atlassian.net/browse/ARO-23324) | Upstream CAPZ proposal for ARO-HCP | Phase 0, 3 |
| [ARO-24928](https://redhat.atlassian.net/browse/ARO-24928) | CAPZ Downstream for ACM 2.17 | Phase 1 |
| [ARO-25085](https://redhat.atlassian.net/browse/ARO-25085) | E2E testing with ARO-HCP devel environment | Phase 0, 1, 5 |
| [ARO-26301](https://redhat.atlassian.net/browse/ARO-26301) | Dedicated CAPZ quay.io organization | Phase 0 |
| [ARO-24154](https://redhat.atlassian.net/browse/ARO-24154) | CAPZ useAgent header | Phase 0 |
| [ARO-26531](https://redhat.atlassian.net/browse/ARO-26531) | Retry backoff for subscription quota | Phase 0 |
| [ARO-25822](https://redhat.atlassian.net/browse/ARO-25822) | Security fixes for CAPZ and ASO | Phase 0 |
| [ARO-26556](https://redhat.atlassian.net/browse/ARO-26556) | MCE-5.0/5.1 branches | Phase 0, 1 |
