
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
  - payload: `{time: <seconds since epoch>,  url: <webhook url>, signature: <signature of "time-url">}`
  - Description: Registers a new webhook for the mobile app.

- **LNURL Pay Info Endpoint:**
  - Endpoint: `lnurlp/{pubkey}/`
  - Method: GET
  - Description: Handles LNURL pay requests, forwarding them to the corresponding mobile app webhook.

- **LNURL Pay Invoice Endpoint:**
  - Endpoint: `lnurlpay/{pubkey}/invoice?amount=<amount>`
  - Method: GET
  - Description: Handles LNURL pay invoice requests, forwarding them to the corresponding mobile app webhook.

- **Webhook Callback Endpoint:**
  - Endpoint: `/response/{responseID}`
  - Method: POST
  - Description: Handles webhook callback responses from the node.
