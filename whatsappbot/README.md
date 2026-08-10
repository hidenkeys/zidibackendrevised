# WhatsApp Bot Setup

## Environment Variables Required

Add these to your `.env` file:

```env
WHATSAPP_ACCESS_TOKEN=your_access_token_here
WHATSAPP_PHONE_NUMBER_ID=your_phone_number_id_here
WHATSAPP_WEBHOOK_VERIFY_TOKEN=your_verify_token_here
```

## Setup Steps

### 1. Meta Developer Setup

1. Go to [developers.facebook.com](https://developers.facebook.com)
2. Create a new app or use existing one
3. Add WhatsApp product to your app
4. Get your access token and phone number ID

### 2. Webhook Configuration

1. Set webhook URL: `https://yourdomain.com/whatsapp/webhook`
2. Set verify token (same as `WHATSAPP_WEBHOOK_VERIFY_TOKEN`)
3. Subscribe to `messages` field

### 3. Integration with Your App

Add these routes to your main application:

```go
// In your main.go or routes file
import "github.com/hidenkeys/zidibackend/whatsappbot"

// Webhook verification (GET)
app.Get("/whatsapp/webhook", whatsappbot.WebhookVerification)

// Webhook handler (POST)
app.Post("/whatsapp/webhook", whatsappbot.WebhookHandler(db))

// Test endpoint (optional)
app.Post("/whatsapp/test", whatsappbot.TestMessage)
```

## How Users Interact

Users start by sending a natural message with the campaign name:

Example: `Hi I am here for the Joy campaign`

The bot extracts the campaign name from the message and finds the matching campaign using fuzzy matching.

## Sharing with Consumers

### Option 1: WhatsApp Link (Recommended)

```
https://wa.me/YOUR_BUSINESS_NUMBER?text=Hi%20I%20am%20here%20for%20the%20CAMPAIGN_NAME%20campaign
```

Example: `https://wa.me/2348085105382?text=Hi%20I%20am%20here%20for%20the%20Joy%20campaign`

### Option 2: QR Code

Generate QR codes that contain the WhatsApp link above

### Option 3: Direct Instructions

Tell users to:

1. Click the WhatsApp link or scan QR code
2. Send the pre-filled message

### Legacy Support

The bot still supports the legacy `start <campaign_id>` format for backwards compatibility.

## Testing

1. Use the test endpoint:

```bash
curl -X POST http://localhost:8080/whatsapp/test \
  -H "Content-Type: application/json" \
  -d '{"to":"234XXXXXXXXXX","message":"Hello from WhatsApp Bot!"}'
```

2. Send a message to your WhatsApp Business number like: `Hi I am here for the Joy campaign`

## Notes

- Phone numbers should be in international format without + (e.g., 234XXXXXXXXXX)
- WhatsApp buttons are limited to 3 options max
- Template messages may be required for certain use cases
- Business verification may be needed for production use
