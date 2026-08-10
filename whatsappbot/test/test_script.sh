#!/bin/bash

echo "🧪 Testing WhatsApp Bot Implementation"
echo "======================================"

# Start the test server in background
echo "Starting test server..."
go run test_server.go &
SERVER_PID=$!

# Wait for server to start
sleep 3

echo ""
echo "1. Testing webhook verification..."
curl -s "http://localhost:3000/whatsapp/webhook?hub.mode=subscribe&hub.verify_token=test_verify_token&hub.challenge=test_challenge"
echo ""

echo ""
echo "2. Testing webhook message handling..."
curl -s -X POST http://localhost:3000/whatsapp/webhook \
  -H "Content-Type: application/json" \
  -d @test_webhook.json
echo ""

echo ""
echo "3. Testing message sending..."
curl -s -X POST http://localhost:3000/whatsapp/test \
  -H "Content-Type: application/json" \
  -d '{"to":"2348123456789","message":"Hello from WhatsApp Bot Test!"}'
echo ""

echo ""
echo "4. Testing server endpoints..."
curl -s http://localhost:3000/
echo ""

# Clean up
echo ""
echo "Stopping test server..."
kill $SERVER_PID

echo "✅ Test completed!"