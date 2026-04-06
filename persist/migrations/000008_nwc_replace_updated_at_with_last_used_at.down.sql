ALTER TABLE public.nwc_webhooks ADD COLUMN updated_at timestamp without time zone NOT NULL DEFAULT NOW();
ALTER TABLE public.nwc_webhooks DROP COLUMN last_used_at;
