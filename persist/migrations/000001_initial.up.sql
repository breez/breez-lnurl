CREATE TABLE public.lnurl_webhooks (
  id bigserial primary key,
	pubkey bytea NOT NULL,  
	url varchar NOT NULL,
	created_at bigint NOT NULL,
	refreshed_at bigint NOT NULL
);

CREATE INDEX lnurl_webhooks_pubkey_idx ON public.lnurl_webhooks (pubkey);
CREATE UNIQUE INDEX lnurl_webhooks_pubkey_url ON public.lnurl_webhooks (pubkey, url);