ALTER TABLE public.nwc_webhooks ADD COLUMN last_used_at timestamp without time zone;
UPDATE public.nwc_webhooks SET last_used_at = updated_at;
ALTER TABLE public.nwc_webhooks DROP COLUMN updated_at;
