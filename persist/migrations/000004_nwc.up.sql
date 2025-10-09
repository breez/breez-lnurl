CREATE TABLE public.nwc_webhooks (
  id bigserial primary key,
  url varchar NOT NULL,
  user_pubkey bytea NOT NULL,
  app_pubkey bytea NOT NULL,
	updated_at bigint NOT NULL
);
CREATE INDEX nwc_webhooks_pubkey_idx ON public.nwc_webhooks (user_pubkey);
CREATE UNIQUE INDEX nwc_webhooks_pubkey_pair ON public.nwc_webhooks (user_pubkey, app_pubkey);

CREATE TABLE public.nwc_relays (
  id serial primary key,
  url varchar UNIQUE NOT NULL
);

CREATE TABLE public.nwc_webhooks_relays (
  webhook_id bigint references public.nwc_webhooks(id) ON DELETE CASCADE,
  relay_id bigint references public.nwc_relays(id),
  PRIMARY KEY (webhook_id, relay_id)
);
