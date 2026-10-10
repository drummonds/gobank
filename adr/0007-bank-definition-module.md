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

### Release or setting

Not everything about the bank is a release. The parameter tree
(ADR-0006) is run-time configuration: a rate, a spread, a lag, an
allowance, a term is changed by a setting, decided for a day, with no
release. Rules are code, and a change to rules is a tested release: a
new rules version in gobank-products or in the bank module, and the
definition carrying it. Between the two sits the product itself: a
product is an instance of a tested rules version with its own name and
its own parameter values. A new fixed-rate saver with a different term
is the same rules with another term, and that is configuration, not a
release: the bank creates it, names it, sets its parameters and later
withdraws it through the parameters it already has.

| Change | How | Who |
|---|---|---|
| A rate, a spread, a lag, an allowance, a term | a setting, effective from a day | the bank's staff, through the parameters |
| A new product on existing rules: another term, another rate, another name | a product instance created in the catalogue, its parameters set | the bank's staff |
| A product withdrawn from sale | the instance withdrawn | the bank's staff |
| New or changed rules: a new event answer, a new derivation, a new parameter | a rules version, tested, released; the definition lists it | engineers, a release |
| The definition's own contents: name, the rules it carries, the policy, the overrides | the module, released | engineers, a release |

The definition therefore carries the bank's *initial* catalogue and the
rules versions it is allowed to instantiate; what the bank has done with
them since is in the database, as every other setting is.

## Consequences

Easier: a white label is a module; a product change is a diff in one
place; the capabilities are tested against more than one definition
(stage 9's scenario tests build a definition each).

Harder: two repositories to release for a change that touches both, in
order (gobank first, the definition pinning it); the definition's module
version becomes part of what a deployment names; ADR-0006's product
identity moves out of the rules package into the catalogue (its
amendment below).

Given up: a product list that can be read off the definition alone.
The definition says what the bank started with and what it may build
products from; the catalogue in the database says what it sells today.
