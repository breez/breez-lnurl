CREATE TABLE public.lnurl_webhooks (
  id bigserial primary key,
	pubkey bytea NOT NULL,
  hook_key_hash bytea NOT NULL,
	url varchar NOT NULL,
	created_at bigint NOT NULL,
	refreshed_at bigint NOT NULL
);

CREATE INDEX lnurl_webhooks_pubkey_idx ON public.lnurl_webhooks (pubkey);
CREATE UNIQUE INDEX lnurl_webhooks_pubkey_url_key ON public.lnurl_webhooks (pubkey, url);
CREATE UNIQUE INDEX lnurl_webhooks_pubkey_hook_key_hash_key ON public.lnurl_webhooks (pubkey, hook_key_hash);