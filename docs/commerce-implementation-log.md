# ZidiCommerce Implementation Log

This log records the completed ZidiCommerce implementation phases and their verification evidence.

| Phase | Changes made | Primary modules | Database and APIs | Verification |
| --- | --- | --- | --- | --- |
| 1 - Commerce foundation | Added tenant-scoped merchant, store, opening-hours, fulfilment-mode, and staff-assignment foundations with role-aware access. | `models/commerce_foundation.go`, `repository/commerce_foundation_repo.go`, `services/commerce_foundation_service.go`, commerce foundation handlers | Additive commerce foundation migrations and merchant/store/staff APIs | Focused service tests and repository-wide checks |
| 2 - Catalogue and inventory | Added categories, products, variants, images, store availability, price overrides, stock levels, and stock adjustments. | Commerce catalogue models, repository, service, and handlers | Additive catalogue/inventory migrations and catalogue/store-stock APIs | Catalogue service tests and repository-wide checks |
| 3 - Customers and carts | Added channel identities, tenant customers, active single-store carts, item mutation, and authoritative cart totals. | Commerce customer/cart models, repository, service, and handlers | Additive customer/cart migrations and customer/cart APIs | Customer/cart service tests and repository-wide checks |
| 4 - Orders and checkout | Added idempotent checkout, inventory reservations, immutable order lines, totals, order events, and status rules. | Commerce order models, repository, service, and handlers | Additive order/reservation migrations and checkout/order APIs | Order repository/service tests and repository-wide checks |
| 5 - Payments and invoices | Added authoritative invoices, provider-neutral payment boundaries, Paystack integration, idempotent webhook processing, and expiry handling. | `payments/`, commerce payment repository/service/handlers | Additive finance migrations and payment/webhook APIs | Payment provider/service tests and repository-wide checks |
| 6 - Fulfilment | Added pickup, customer-rider, and merchant-rider state, delivery quote abstraction, verification codes, and completion rules. | `fulfilment/`, commerce fulfilment repository/service/handlers | Additive fulfilment migrations and fulfilment APIs | Fulfilment unit/service tests and repository-wide checks |
| 7 - WhatsApp commerce | Added tenant channel configuration, conversation state, inbound idempotency, ordering/tracking/support flows, and outbound delivery. | Commerce channel models, repository/services/handlers, `messaging/`, `whatsappbot/` | Additive channel/conversation migrations and WhatsApp channel APIs | Channel, delivery, messaging, and webhook tests |
| 8 - Store order console | Added store-scoped order queues and prepare/ready/handover workflows using the order and fulfilment services. | Commerce store-order service and handlers | Store order console APIs over existing commerce tables | Store-order service tests and repository-wide checks |
| 9 - Merchant Commerce Admin | Added merchant-facing commerce management screens and API integration for catalogue, stores, inventory, orders, fulfilment, channel configuration, and complaints. | `zidi-client` Commerce Admin modules | Uses the authenticated commerce APIs from phases 1-8 | Frontend type checks/build and responsive browser review |
| 10 - Bing Chun onboarding | Added generic JSON-driven transactional onboarding, Bing Chun's seven-store and 28-product dataset, WhatsApp configuration, public merchant link resolution, and standalone website integration. | `commerceonboarding/`, `cmd/onboard-commerce/`, `config/merchants/`, channel repository/service/handler, standalone `bingchunnigeria` site | No new migration; adds public merchant WhatsApp resolver API | Full Go tests, vet, build, onboarding dry-run, and standalone frontend build |
| 11 - End-to-end testing | Added a disposable PostgreSQL harness that applies the real migrations and Bing Chun seed, then exercises customer, payment, store, fulfilment, notification, tracking, support, isolation, concurrency, and recovery paths. The run also exposed and fixed the UUID type contract on `users.organization_id`, including an existing-schema conversion. | `e2e/commerce_e2e_test.go`, `scripts/run-commerce-e2e.sh`, `migrations/legacy.go`, `models/user.go`, `Makefile` | No external call; tests use a fresh PostgreSQL container plus deterministic payment and WhatsApp test doubles | `make commerce-e2e`, full Go tests, vet, build, legacy schema conversion, and migration rerun validation |

## Phase 11 evidence

The end-to-end suite covers:

- Bing Chun onboarding, active catalogue, inventory, nearest-open-store selection, and closed-store fallback.
- Persisted WhatsApp ordering from location through cart, checkout, invoice, and payment-link creation.
- Conversation recovery after recreating the channel service during checkout.
- Webhook-only payment success, duplicate delivery idempotency, invoice settlement, inventory commitment, and customer/store outbox events.
- Store assignment isolation, merchant isolation, preparation, ready notifications, pickup arrival, wrong-code rejection, secure handover, and completed-order immutability.
- Customer-rider and merchant-rider lifecycles, including manual quote acceptance, assignment, arrival, handover, delivery, and completion.
- Real tracking status, invalid order IDs, and complaint creation over the persisted WhatsApp conversation.
- Product removal after cart creation, authoritative price changes, duplicate checkout, expired payment release, and concurrent checkout for the final inventory unit.

Payment gateway verification and WhatsApp delivery use in-process test doubles. This is intentional: Phase 11 does not call live provider accounts or production data. Provider sandbox smoke tests remain deployment checks rather than repository tests.
