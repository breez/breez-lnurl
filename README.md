
# LNURL implementation for Breez SDK

## Overview
This server application allows mobile apps that use the Breez SDK to register webhooks and exposes an LNURL pay endpoint for that app. It also acts as a bridge between the mobile app and payer.

## How it works?
- **lnurlpay Registration**: A mobile app registers a webhook to be reached by the server. The webhook is registered under a specific node pubkey. The server stores the pubkey and the webhook details in a database.
- **LNURL Pay Endpoint**: The server exposes an LNURL pay endpoint for the mobile app (lnurlp/pubkey).
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
- **DATABASE_URL**: The database url.

### Running the Server
Execute the command below to start the server:
```
go run .
```

## API Endpoints
- **Register LNURL Webhook:**
  - Endpoint: `/lnurlpay/{pubkey}`
  - Method: POST
  - Params:
    - `pubkey` used to sign the request signature
  - Payload: `{time: <seconds since epoch>, webhook_url: <webhook url>, username: <optional username for lightning address>, signature: <signature of "time-webhook_url" or "time-webhook_url-username">}`
  - Description: Registers a new webhook for the mobile app.

- **Unregister LNURL Webhook:**
  - Endpoint: `/lnurlpay/{pubkey}`
  - Method: DELETE
  - Params:
    - `pubkey` used to sign the request signature
  - Payload: `{time: <seconds since epoch>, webhook_url: <webhook url>, signature: <signature of "time-webhook_url">}`
  - Description: Unregisters a webhook from the LNURL service.

- **Recover Registered LNURL and Lightning Address:**
  - Endpoint: `/lnurlpay/{pubkey}/recover`
  - Method: POST
  - Params:
    - `pubkey` used to sign the request signature
  - Payload: `{time: <seconds since epoch>, webhook_url: <webhook url>, signature: <signature of "time-webhook_url">}`
  - Description: Recovers the LNURL and lightning address registered.

- **LNURL Pay Info Endpoint:**
  - Endpoint: `lnurlp/{identifier}`
  - Method: GET
  - Params:
    - `identifier` represents the pubkey or username registered
  - Description: Handles LNURL pay requests, forwarding them to the corresponding mobile app webhook.

- **LNURL Pay Invoice Endpoint:**
  - Endpoint: `lnurlpay/{identifier}/invoice?amount=<amount>`
  - Method: GET
  - Params: 
    - `identifier`: represents the pubkey or username registered
    - `amount`: invoice amount in millisatoshi
  - Description: Handles LNURL pay invoice requests, forwarding them to the corresponding mobile app webhook.

- **Webhook Callback Endpoint:**
  - Endpoint: `/response/{responseID}`
  - Method: POST
  - Description: Handles webhook callback responses from the node.
