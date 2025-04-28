ALTER TABLE public.lnurl_pubkey_usernames ADD COLUMN offer varchar;
ALTER TABLE public.lnurl_pubkey_usernames RENAME TO pubkey_details;
