# Bing Chun Nigeria Commerce Onboarding

Phase 10 keeps Bing Chun data in `config/merchants/bing-chun-nigeria.json` and applies it through the generic `cmd/onboard-commerce` command. Nothing in the commerce services branches on the Bing Chun merchant.

## Prerequisites

The Zidi organization must already exist. Use that organization's UUID for `BING_CHUN_ORGANIZATION_ID`.

Set these variables in the Railway backend service:

```text
BING_CHUN_ORGANIZATION_ID=<existing-zidi-organization-uuid>
BING_CHUN_WHATSAPP_PROVIDER_ACCOUNT_ID=<meta-phone-number-id>
BING_CHUN_WHATSAPP_DISPLAY_PHONE_NUMBER=+234...
```

The display phone number must be the WhatsApp Business number customers should open from the Bing Chun website, in E.164 format such as `+2348012345678` with no spaces. The provider account ID is Meta's phone number ID received in webhook metadata; it is not the display phone number.

## Validate

```bash
go run ./cmd/onboard-commerce -config config/merchants/bing-chun-nigeria.json -dry-run
```

The expected summary is 7 stores, 6 categories, 28 products, 28 variants, and 196 store catalogue rows.

## Apply

Run this once from a Railway shell or as a one-off command in the backend service:

```bash
go run ./cmd/onboard-commerce -config config/merchants/bing-chun-nigeria.json
```

The command is transactional and safe to re-run. It synchronizes merchant metadata, stores, known opening hours, fulfilment modes, categories, products, prices, availability, reorder thresholds, and WhatsApp configuration. Existing inventory quantities are preserved on subsequent runs.

Four items marked sold out on the published Nigerian menu are onboarded as disabled with zero opening stock. Only Jara Mall and Olive Mall are activated initially because complete opening and closing times were verified for those branches. Confirm the remaining hours in Commerce Admin before activating those stores for automated recommendations.

Pickup and customer-arranged rider fulfilment are enabled. Merchant-arranged delivery remains disabled until a live delivery provider is configured.

## Website integration

After onboarding, the public endpoint below resolves the active channel for this merchant without exposing provider or organization identifiers:

```text
GET /api/v1/commerce/public/merchants/bing-chun-nigeria/whatsapp-link
```

The standalone Bing Chun site uses `VITE_ZIDI_COMMERCE_API_BASE_URL` and `VITE_ZIDI_COMMERCE_MERCHANT_SLUG` to call this endpoint. It never stores the WhatsApp number in frontend source. Deploy the backend, run the onboarding command, verify this endpoint returns a `wa.me` URL, and then deploy the frontend.
