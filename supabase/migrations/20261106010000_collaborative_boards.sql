-- Boards are the sharing boundary. All writes below use the caller's auth.uid().
begin;
create table public.boards (
 id uuid primary key default gen_random_uuid(),
 owner_id uuid not null unique references auth.users(id) on delete cascade,
 name text not null default 'Personal board', revision bigint not null default 0,
 created_at timestamptz not null default now()
);
create table public.board_members (
 id uuid primary key default gen_random_uuid(),
 board_id uuid not null references public.boards(id) on delete cascade,
 user_id uuid references auth.users(id) on delete set null,
 name text not null check (length(trim(name)) between 1 and 80),
 color text not null default '#71717a' check (color ~ '^#[0-9a-fA-F]{6}$'),
 created_at timestamptz not null default now(), unique(board_id,user_id)
);
create table public.board_invitations (
 id uuid primary key default gen_random_uuid(),
 board_id uuid not null references public.boards(id) on delete cascade,
 member_id uuid not null references public.board_members(id) on delete cascade,
 token_hash text not null unique, expires_at timestamptz not null default now()+interval '7 days',
 accepted_by uuid references auth.users(id), created_at timestamptz not null default now()
);
alter table public.tasks add column board_id uuid references public.boards(id) on delete cascade;
alter table public.tasks add column assignee_ids uuid[] not null default '{}';
-- Preserve existing guest tasks when moving from a personal board to shared boards.
insert into public.boards(owner_id) select distinct user_id::uuid from public.tasks on conflict do nothing;
update public.tasks t set board_id=b.id from public.boards b where t.user_id=b.owner_id::text;
update public.tasks set due_date=null where due_date < '1900-01-01';
create index tasks_board_idx on public.tasks(board_id,created_at);
create index members_user_idx on public.board_members(user_id,board_id);

create function public.can_access_board(p_board uuid) returns boolean
language sql stable security definer set search_path=public as $$
 select exists(select 1 from boards where id=p_board and owner_id=auth.uid())
 or exists(select 1 from board_members where board_id=p_board and user_id=auth.uid())
$$;
revoke all on function public.can_access_board(uuid) from public;
grant execute on function public.can_access_board(uuid) to authenticated;
alter table public.boards enable row level security;
alter table public.board_members enable row level security;
alter table public.board_invitations enable row level security;
create policy board_read on public.boards for select to authenticated using (can_access_board(id));
create policy members_read on public.board_members for select to authenticated using (can_access_board(board_id));
-- Invitations have no direct client access: only the controlled RPCs can handle them.
grant select on public.boards,public.board_members to authenticated;
revoke all on public.board_invitations from anon,authenticated;
drop policy "Users can view their own tasks" on public.tasks;
drop policy "Users can create their own tasks" on public.tasks;
drop policy "Users can update their own tasks" on public.tasks;
drop policy "Users can delete their own tasks" on public.tasks;
create policy task_read on public.tasks for select to authenticated using
 (can_access_board(board_id) or (board_id is null and user_id=auth.uid()::text));
create policy task_insert on public.tasks for insert to authenticated with check
 (user_id=auth.uid()::text and (can_access_board(board_id) or board_id is null));
create policy task_update on public.tasks for update to authenticated using
 (can_access_board(board_id) or (board_id is null and user_id=auth.uid()::text)) with check
 (can_access_board(board_id) or (board_id is null and user_id=auth.uid()::text));
create policy task_delete on public.tasks for delete to authenticated using
 (can_access_board(board_id) or (board_id is null and user_id=auth.uid()::text));

-- An existing task cannot be moved to a different security boundary by a client.
create function public.validate_task_board() returns trigger language plpgsql set search_path=public as $$
begin
 if TG_OP='UPDATE' and (new.user_id<>old.user_id or (old.board_id is not null and new.board_id is distinct from old.board_id)) then
  raise exception 'Task ownership cannot be changed' using errcode='42501';
 end if;
 if exists(select 1 from unnest(new.assignee_ids) a where not exists(select 1 from board_members m where m.id=a and m.board_id=new.board_id)) then
  raise exception 'Assignees must belong to this board' using errcode='22023';
 end if;
 return new;
end $$;
create trigger validate_task_board before insert or update on public.tasks for each row execute function public.validate_task_board();

-- The durable revision repairs missed SSE events and works across Gin instances.
create function public.bump_board_revision() returns trigger language plpgsql security definer set search_path=public as $$
begin
 if TG_OP='DELETE' then update boards set revision=revision+1 where id=old.board_id; return old;
 else update boards set revision=revision+1 where id=new.board_id; return new; end if;
end $$;
create trigger tasks_revision after insert or update or delete on public.tasks for each row execute function public.bump_board_revision();
create trigger members_revision after insert or update or delete on public.board_members for each row execute function public.bump_board_revision();

create function public.board_revision(p_board uuid) returns bigint language plpgsql security definer set search_path=public as $$
begin
 if not can_access_board(p_board) then raise exception 'Board access denied' using errcode='42501'; end if;
 return (select revision from boards where id=p_board);
end $$;

create function public.workspace_snapshot(p_board uuid default null) returns jsonb language plpgsql security definer set search_path=public as $$
declare v_board uuid; v_user uuid:=auth.uid();
begin
 if v_user is null then raise exception 'Session required' using errcode='42501'; end if;
 insert into boards(owner_id) values(v_user) on conflict(owner_id) do nothing;
 select id into v_board from boards where owner_id=v_user;
 insert into board_members(board_id,user_id,name) values(v_board,v_user,'Board owner') on conflict(board_id,user_id) do nothing;
 if p_board is not null then v_board:=p_board; end if;
 if not can_access_board(v_board) then raise exception 'Board access denied' using errcode='42501'; end if;
 return jsonb_build_object(
  'user_id',v_user,'board',(select to_jsonb(b) from boards b where id=v_board),
  'boards',(select coalesce(jsonb_agg(to_jsonb(b) order by b.created_at),'[]') from boards b where can_access_board(b.id)),
  'members',(select coalesce(jsonb_agg(to_jsonb(m) order by m.created_at),'[]') from board_members m where board_id=v_board),
  'tasks',(select coalesce(jsonb_agg(to_jsonb(t) order by t.created_at,t.id),'[]') from tasks t where board_id=v_board)
 );
end $$;

-- Link invitations are single-use, expire in seven days, and store only a hash.
create function public.invite_board_member(p_board uuid,p_name text,p_color text default '#71717a',p_member uuid default null) returns jsonb
language plpgsql security definer set search_path=public,extensions as $$
declare v_member board_members; v_token text:=encode(gen_random_bytes(24),'hex');
begin
 if not exists(select 1 from boards where id=p_board and owner_id=auth.uid()) then raise exception 'Only the board owner can invite members' using errcode='42501'; end if;
 if p_member is null then
  if (select count(*) from board_members where board_id=p_board)>=20 then raise exception 'A board supports up to 20 members' using errcode='22023'; end if;
  insert into board_members(board_id,name,color) values(p_board,trim(p_name),p_color) returning * into v_member;
 else select * into v_member from board_members where id=p_member and board_id=p_board and user_id is null for update;
  if not found then raise exception 'Member is already connected or does not exist' using errcode='22023'; end if;
 end if;
 update board_invitations set expires_at=now() where member_id=v_member.id and accepted_by is null;
 insert into board_invitations(board_id,member_id,token_hash) values(p_board,v_member.id,encode(digest(v_token,'sha256'),'hex'));
 return jsonb_build_object('member',to_jsonb(v_member),'token',v_token);
end $$;
create function public.accept_board_invitation(p_token text) returns uuid language plpgsql security definer set search_path=public,extensions as $$
declare v_invite board_invitations; v_user uuid:=auth.uid();
begin
 if v_user is null then raise exception 'Session required' using errcode='42501'; end if;
 select * into v_invite from board_invitations where token_hash=encode(digest(p_token,'sha256'),'hex') for update;
 if not found or v_invite.expires_at<=now() or (v_invite.accepted_by is not null and v_invite.accepted_by<>v_user) then raise exception 'Invitation expired or already used' using errcode='22023'; end if;
 if can_access_board(v_invite.board_id) then return v_invite.board_id; end if;
 update board_members set user_id=v_user where id=v_invite.member_id and user_id is null;
 if not found then raise exception 'Invitation already used' using errcode='22023'; end if;
 update board_invitations set accepted_by=v_user where id=v_invite.id;
 return v_invite.board_id;
end $$;

-- Row locking makes each drop atomic. Later committed writes win; no cursor data is stored.
create function public.save_board_task(p_board uuid,p_task text,p_changes jsonb) returns jsonb
language plpgsql security definer set search_path=public as $$
declare v_task tasks; v_key text;
begin
 if not can_access_board(p_board) then raise exception 'Board access denied' using errcode='42501'; end if;
 for v_key in select jsonb_object_keys(p_changes) loop
  if v_key not in ('title','description','status','priority','assignee_ids','due_date') then raise exception 'Unknown task field: %',v_key using errcode='22023'; end if;
 end loop;
 if p_task is null then
  v_task.id:=gen_random_uuid()::text; v_task.user_id:=auth.uid()::text; v_task.board_id:=p_board;
  v_task.created_at:=now(); v_task.status:='pending'; v_task.priority:='medium'; v_task.assignee_ids:='{}';
 else select * into v_task from tasks where id=p_task and board_id=p_board for update;
  if not found then raise exception 'Task not found' using errcode='P0002'; end if;
 end if;
 v_task:=jsonb_populate_record(v_task,p_changes);
 if v_task.title is null or length(trim(v_task.title)) not between 1 and 160 then raise exception 'Title must be 1-160 characters' using errcode='22023'; end if;
 if length(coalesce(v_task.description,''))>4000 then raise exception 'Description is too long' using errcode='22023'; end if;
 if v_task.status not in ('pending','in_progress','review','completed') or v_task.status is null then raise exception 'Invalid status' using errcode='22023'; end if;
 if v_task.priority not in ('low','medium','high') or v_task.priority is null then raise exception 'Invalid priority' using errcode='22023'; end if;
 if v_task.assignee_ids is null then raise exception 'Assignees must be an array' using errcode='22023'; end if;
 if p_task is null then insert into tasks select v_task.* returning * into v_task;
 else update tasks set title=trim(v_task.title),description=v_task.description,status=v_task.status,priority=v_task.priority,assignee_ids=v_task.assignee_ids,due_date=v_task.due_date where id=p_task returning * into v_task;
 end if;
 return to_jsonb(v_task);
end $$;

revoke all on function public.workspace_snapshot(uuid),public.board_revision(uuid),public.invite_board_member(uuid,text,text,uuid),public.accept_board_invitation(text),public.save_board_task(uuid,text,jsonb) from public;
grant execute on function public.workspace_snapshot(uuid),public.board_revision(uuid),public.invite_board_member(uuid,text,text,uuid),public.accept_board_invitation(text),public.save_board_task(uuid,text,jsonb) to authenticated;
notify pgrst,'reload schema';
commit;
