create table if not exists public.tasks (
    id text primary key,
    title text not null,
    status text not null default 'pending',
    user_id text not null,
    created_at timestamptz not null default now(),
    description text,
    priority text,
    due_date timestamptz,
    assignee_id text
);

alter table public.tasks enable row level security;

create policy "Users can view their own tasks"
on public.tasks for select
to authenticated
using ((select auth.uid()::text) = user_id);

create policy "Users can create their own tasks"
on public.tasks for insert
to authenticated
with check ((select auth.uid()::text) = user_id);

create policy "Users can update their own tasks"
on public.tasks for update
to authenticated
using ((select auth.uid()::text) = user_id)
with check ((select auth.uid()::text) = user_id);

create policy "Users can delete their own tasks"
on public.tasks for delete
to authenticated
using ((select auth.uid()::text) = user_id);
