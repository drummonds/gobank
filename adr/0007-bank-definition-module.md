# ADR-0007: The bank's definition is a module of its own

Status: Proposed
Date: 2026-10-10
Stage: ADR-0002 stage 8 (roadmap 1.8), stories 1.8.2, 1.8.4 and 1.8.5

## Context

gobank is becoming a general banking solution: a ledger, a general
ledger, customers, payments, users, a BFF and the product contract of
ADR-0006. What makes the running demo *this* bank is scattered through
it: the catalogue is a list in `bank/products`, the blurb for each
product beside it, the name "Model Bank" in the demo, the base-rate
series in the simulation. A second bank, a white label on the same
capabilities, would be a fork.

## Decision

One Go module per bank entity holds everything that makes the general
solution this bank, as code. gobank holds the capabilities; a running
bank is gobank opened with one definition; another bank is another
definition module over the same capabilities.

| In the definition | In the capabilities (gobank) |
|---|---|
| The bank's name | the ledger, the GL, customers, payments, users, the BFF and apps |
| The product versions it carries at one time, in catalogue order, each with the bank's blurb and whether it is withdrawn from sale: the catalogue and its migration data, release by release | the product contract (gobank-products), the runner, the adoption record |
| Its policy: the savings and lending base rates derived from the market rate, their spreads and lags (ADR-0006, story 1.8.4) | the parameters component that stores settings and resolves them |
| Its own products, once a product is the bank's rather than the library's (story 1.8.5) | products a bank shares with others |
| Customisation that overrules defaults as it arises: data lifetime, security settings | the defaults |

The shape today is `bank/definition.Bank` (name, product listings) and
the package `modelbank` in gobank that implements it for the Model
Bank; `bank.Open` takes the definition and refuses to open without one.
The package becomes the module `modelbank`, in its own repository, in
story 1.8.4 when the policy joins it: a module with one product list and
no policy is not yet worth a repository.

What the definition is not: configuration read at run time. It is
compiled in, versioned with the bank, reviewed as code, and a change to
it is a release. A product version coming off sale is a line in the
definition; its adoption and withdrawal days are recorded by the bank
when it first runs with the change.

## Consequences

Easier: a white label is a module; a product change is a diff in one
place; the capabilities are tested against more than one definition
(stage 9's scenario tests build a definition each).

Harder: two repositories to release for a change that touches both, in
order (gobank first, the definition pinning it); the definition's module
version becomes part of what a deployment names.

Given up: a bank that can be reconfigured without a release.
