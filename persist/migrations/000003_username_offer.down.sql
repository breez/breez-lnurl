ALTER TABLE public.lnurl_pubkey_usernames DROP COLUMN offer;
ALTER TABLE public.pubkey_details RENAME TO lnurl_pubkey_usernames;