
# LNURL implementation for Breez SDK

## Overview
This server application allows mobile apps that use the Breez SDK to register webhooks and exposes an LNURL pay endpoint for that app. It also acts as a bridge between the mobile app and payer.

## How it works?
- **Webhook Registration**: A mobile app registers a webhook to be reached by the server. The webhook is registered under a specific key and node pubkey. The server stores the key hash and the webhook details in a database.
- **LNURL Pay Endpoint**: The server exposes an LNURL pay endpoint for the mobile app (lnurlpay/pubkey/key_hash).
- **Bridge**: When a payer starts the lnurl pay flow, the server receives the request and forwards it to the mobile app's webhook, providing a callback. The mobile app processes the request and responds via the callback. The server matches the response to the request and returns it to the payer.

## Getting Started

### Installation
1. **Clone the repository:**
   ```
   git clone https://github.com/breez/breez-lnurl.git
   ```
2. **Navigate to the project directory:**
   ```
   cd breez-lnurl
   ```

3. **Install dependencies:**
   ```
   go mod tidy
   ```

### Configuration
There are two optional environment variables that can be set:
- **SERVER_EXTERNAL_URL**: The url this server can be reached from the outside world.
- **SERVER_INTERNAL_URL**: The internal url the server listens to.

### Running the Server
Execute the command below to start the server:
```
go run .
```

## API Endpoints
- **Register LNURL Webhook:**
  - Endpoint: `/webhooks/{pubkey}`
  - Method: POST
  - payload: `{time: <seconds since epoch>, hook_key: "<hook key>", url: <webhook url>, signature: <signature of "time-hook_key-url">}`
  - Description: Registers a new webhook for the mobile app.

- **LNURL Pay Endpoint:**
  - Endpoint: `lnurlpay/{pubkey}/{hook_key_hash}`
  - Method: GET
  - Description: Handles LNURL pay requests, forwarding them to the corresponding mobile app webhook.

- **LNURL Pay Endpoint:**
  - Endpoint: `lnurlpay/{pubkey}/{hook_key_hash}/invoice?amount=<amount>`
  - Method: GET
  - Description: Handles LNURL pay invoice requests, forwarding them to the corresponding mobile app webhook.