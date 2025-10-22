
# Webhook implementation for Breez SDK

## Overview
This server application allows mobile apps that use the Breez SDK to register webhooks and expose different endpoints, such as LNURL-pay or NWC, for that app. It also acts as a bridge between the mobile app and payer.

## How it works?

### LNURL
- **lnurlpay Registration**: A mobile app registers a webhook to be reached by the server. The webhook is registered under a specific node pubkey. The server stores the pubkey and the webhook details in a database.
- **LNURL Pay Endpoint**: The server exposes an LNURL pay endpoint for the mobile app (lnurlp/pubkey).
- **Bridge**: When a payer starts the lnurl pay flow, the server receives the request and forwards it to the mobile app's webhook, providing a callback. The mobile app processes the request and responds via the callback. The server matches the response to the request and returns it to the payer.

### NWC
- **Nostr event Subscription**: A mobile app registers a webhook to be reached by the server. The webhook is registered under a specific the wallet's Nostr pubkey. The server stores the pubkey and the webhook details in a database.
- **Offline notifications**: The server listens to events related to that Nostr pubkey, and forwards them to the mobile app's webhook. The mobile app hten wakes up and processes the NWC request.

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

### Database setup and migration
1. Create a database user for your application.
2. For the initial setup and each time you pull this repo, check the `persist/migrations` directory for any additional migrations.
3. In sequence, run each of the SQL statements in the *.up.sql files in your prefered SQL query tool.

### Configuration
There are two optional environment variables that can be set:
- **SERVER_EXTERNAL_URL**: The url this server can be reached from the outside world.
- **SERVER_INTERNAL_URL**: The internal url the server listens to.
- **DATABASE_URL**: The database url.
For DNS management of BIP353 records
- **NAME_SERVER**: The name server to connect to.
- **DNS_PROTOCOL**: The DNS protocol to use (one of "tcp", "tcp-tls" or "udp". Default "udp").
- **TSIG_KEY**: The TSIG key used to authenticate updates.
- **TSIG_SECRET**: The TSIG secret used to authenticate updates.

### Running the Server
Execute the command below to start the server:
```
go run .
```

## API Endpoints

### BOLT12 Offer

- **Register BOLT12 Offer:**
  - Endpoint: `/bolt12offer/{pubkey}`
  - Method: POST
  - Params:
    - `pubkey` used to sign the request signature
  - Payload (JSON): 
    - `time` in seconds since epoch
    - `username` for the BIP353 address
    - `offer` for the username's BIP353 record
    - `signature` of "<time>-<username>-<offer>"
  - Description: Registers a new BOLT12 Offer.

- **Unregister BOLT12 Offer:**
  - Endpoint: `/bolt12offer/{pubkey}`
  - Method: DELETE
  - Params:
    - `pubkey` used to sign the request signature
  - Payload (JSON): 
    - `time` in seconds since epoch
    - `offer` for the pubkey's BIP353 record
    - `signature` of "<time>-<offer>"
  - Description: Unregisters a BOLT12 Offer.

- **Recover Registered Lightning Address:**
  - Endpoint: `/bolt12offer/{pubkey}/recover`
  - Method: POST
  - Params:
    - `pubkey` used to sign the request signature
  - Payload (JSON): 
    - `time` in seconds since epoch
    - `offer` for the pubkey's BIP353 record
    - `signature` of "<time>-<offer>"
  - Description: Recovers the lightning address registered.

### BOLT12 Offer and LNURL-Pay

- **Register LNURL Webhook:**
  - Endpoint: `/lnurlpay/{pubkey}`
  - Method: POST
  - Params:
    - `pubkey` used to sign the request signature
  - Payload (JSON): 
    - `time` in seconds since epoch
    - `webhook_url` to receive requests to
    - `username` for the lightning and BIP353 addresses (optional)
    - `offer` for the username's BIP353 record (optional)
    - `signature` of "<time>-<webhook_url>" or "<time>-<webhook_url>-<username>" or "<time>-<webhook_url>-<username>-<offer>"
  - Description: Registers a new webhook for the mobile app.

- **Unregister LNURL Webhook:**
  - Endpoint: `/lnurlpay/{pubkey}`
  - Method: DELETE
  - Params:
    - `pubkey` used to sign the request signature
  - Payload (JSON): 
    - `time` in seconds since epoch
    - `webhook_url` to receive requests to
    - `signature` of "<time>-<webhook_url>"
  - Description: Unregisters a webhook from the LNURL service.

- **Recover Registered LNURL and Lightning Address:**
  - Endpoint: `/lnurlpay/{pubkey}/recover`
  - Method: POST
  - Params:
    - `pubkey` used to sign the request signature
  - Payload (JSON): 
    - `time` in seconds since epoch
    - `webhook_url` to receive requests to
    - `signature` of "<time>-<webhook_url>"
  - Description: Recovers the LNURL and lightning address registered.

- **LNURL Pay Info Endpoint:**
  - Endpoint: `lnurlp/{identifier}`
  - Method: GET
  - Params:
    - `identifier` represents the pubkey or username registered
  - Description: Handles LNURL pay requests, forwarding them to the corresponding mobile app webhook.

- **LNURL Pay Invoice Endpoint:**
  - Endpoint: `lnurlpay/{identifier}/invoice?amount=<amount>&comment=<comment>`
  - Method: GET
  - Params: 
    - `identifier`: represents the pubkey or username registered
    - `amount`: invoice amount in millisatoshi
    - `comment`: pay request comment (optional)
  - Description: Handles LNURL pay invoice requests, forwarding them to the corresponding mobile app webhook.

- **Webhook Callback Endpoint:**
  - Endpoint: `/response/{responseID}`
  - Method: POST
  - Description: Handles webhook callback responses from the node.

### Nostr Wallet Connect

- **Register NWC Webhook:**
  - Endpoint: `/nwc/{pubkey}`
  - Method: POST
  - Params:
    - `pubkey` used to sign the request signature
  - Payload (JSON):
    - `webhookUrl` to receive requests to
    - `appPubkey` for the app's pubkey
    - `relays` array of relay URLs
    - `signature` of "<webhookUrl>-<appPubkey>-<relays>"
  - Description: Registers a new webhook for Nostr Wallet Connect events.

- **Unregister NWC Webhook:**
  - Endpoint: `/nwc/{pubkey}`
  - Method: DELETE
  - Params:
    - `pubkey` used to sign the request signature
  - Payload (JSON):
    - `time` in seconds since epoch
    - `appPubkey` for the app's pubkey
    - `signature` of "<time>-<appPubkey>"
  - Description: Unregisters a webhook from the NWC service.
