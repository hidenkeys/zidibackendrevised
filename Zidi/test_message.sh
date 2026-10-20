#!/bin/bash

# Test script to send a WhatsApp message
# Usage: ./test_message.sh <phone_number> <message>

PHONE=${1:-"2348123456789"}
MESSAGE=${2:-"Hello from Zidi WhatsApp Bot! 🤖"}

echo "📱 Testing WhatsApp message sending..."
echo "Phone: $PHONE"
echo "Message: $MESSAGE"
echo ""

curl -X POST http://localhost:8080/test-send \
  -H "Content-Type: application/json" \
  -d "{
    \"phone\": \"$PHONE\",
    \"message\": \"$MESSAGE\"
  }" | jq .

echo ""
echo "✅ Test completed!"