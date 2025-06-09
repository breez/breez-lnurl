CREATE TABLE public.cached_responses (
    id bigserial primary key,
	url varchar NOT NULL,
	body bytea NOT NULL,  
	expires_at bigint NOT NULL
);

CREATE UNIQUE INDEX cached_responses_url_uk ON public.cached_responses (url);

