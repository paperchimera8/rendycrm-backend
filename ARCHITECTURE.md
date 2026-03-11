# Architecture

## Layering

The application uses four layers for business-critical flows:

1. `internal/transport`
   - HTTP, webhooks, Telegram update parsing, response mapping.
   - Must not mutate business state directly.
2. `internal/usecase`
   - Business scenarios and orchestration.
   - Owns access checks, audit intents, and cross-module workflows.
3. `internal/repository`
   - SQL and persistence only.
   - Must not know about HTTP handlers, Telegram callback semantics, or UI text.
4. `internal/domain`
   - Actor types, invariants, status concepts, and shared business primitives.

`internal/app` is the compatibility shell during migration. New business logic should move into `internal/usecase`, while legacy handlers in `internal/app` should shrink to transport adapters. SQL-backed storage is being moved toward `internal/repository`.

## Critical Flows

The following scenarios must go through a single use case path:

- `ReceiveInboundMessage`
- `ReceiveInboundMessageForWorkspace`
- `ReplyToDialog`
- `ReopenDialog`
- `TakeDialogByHuman`
- `ReturnDialogToAuto`
- `CloseDialog`
- `CreateBooking`
- `ConfirmBooking`
- `CancelBooking`
- `CompleteBooking`
- `RescheduleBooking`
- `UpdateCustomerProfile`
- `UpdateReviewStatus`
- `UpdateMasterProfile`
- `UpdateChannelSettings`
- `UpdateBotConfig`
- `LinkOperatorTelegram`
- `CreateOperatorLinkCode`
- `UnlinkOperatorTelegram`
- `StartBotSession`
- `ClearBotSession`
- `StoreClientBotRoute`
- `ClearClientBotRoute`
- `ToggleAutoReply`

Transport handlers may read data for rendering, but they must call these use cases for mutations.

## Access Control

Access checks live in `internal/usecase/policy.go`.

- Transport authenticates the actor.
- Use case validates workspace access and actor capability.
- Repository methods stay tenant-scoped by `workspace_id`.

## Integrations and Outbound

- External APIs go through adapters such as `internal/telegram`.
- Outbound delivery uses the outbox queue and worker path, not direct handler sends.
- Retries, timeout handling, and payload mapping belong to adapters and outbound dispatchers, not repositories.

## Migration Rule

The repository currently contains legacy orchestration shims. During migration:

1. add/extend a use case,
2. switch transport to the use case,
3. keep repository as storage-only adapter,
4. delete duplicated orchestration paths.

Do not add a second implementation of an existing business scenario.

## AI Guardrails

- Follow the existing project structure.
- Do not introduce parallel layers or duplicate business logic.
- Reuse existing use cases and adapters.
- If a flow already exists, extend it instead of creating a new service path.
