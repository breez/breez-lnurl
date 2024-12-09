CREATE TABLE public.lnurl_pubkey_usernames (
	pubkey bytea PRIMARY KEY,  
	username varchar NOT NULL
);

CREATE UNIQUE INDEX lnurl_pubkey_usernames_username_uk ON public.lnurl_pubkey_usernames (username);
