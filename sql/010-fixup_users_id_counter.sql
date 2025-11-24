SELECT setval(
    'public.users_id_seq', 
    COALESCE((SELECT MAX(id) FROM public.users), 0)
);
