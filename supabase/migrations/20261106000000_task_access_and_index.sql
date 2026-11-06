-- Complete task access and indexing after the table/RLS migration.
create index if not exists tasks_user_created_idx on public.tasks (user_id, created_at);
grant select, insert, update, delete on public.tasks to authenticated;
notify pgrst, 'reload schema';
