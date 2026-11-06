begin;
create table public.board_labels (
 id uuid primary key default gen_random_uuid(), board_id uuid not null references public.boards(id) on delete cascade,
 name text not null check(length(trim(name)) between 1 and 32),
 color text not null default '#71717a' check(color ~ '^#[0-9a-fA-F]{6}$'), created_at timestamptz not null default now(),
 unique(board_id,name)
);
alter table public.board_labels enable row level security;
create policy labels_read on public.board_labels for select to authenticated using(can_access_board(board_id));
grant select on public.board_labels to authenticated;
alter table public.tasks add column label_ids uuid[] not null default '{}';
create trigger labels_revision after insert or update or delete on public.board_labels for each row execute function public.bump_board_revision();

create function public.create_board_label(p_board uuid,p_name text,p_color text default '#71717a') returns jsonb language plpgsql security definer set search_path=public as $$
declare v_label board_labels;
begin
 if not can_access_board(p_board) then raise exception 'Board access denied' using errcode='42501'; end if;
 if (select count(*) from board_labels where board_id=p_board)>=50 then raise exception 'A board supports up to 50 labels' using errcode='22023'; end if;
 insert into board_labels(board_id,name,color) values(p_board,trim(p_name),p_color) returning * into v_label;
 return to_jsonb(v_label);
end $$;
create function public.validate_task_labels() returns trigger language plpgsql set search_path=public as $$
begin
 new.label_ids:=coalesce(new.label_ids,'{}');
 if exists(select 1 from unnest(new.label_ids) a where not exists(select 1 from board_labels l where l.id=a and l.board_id=new.board_id)) then raise exception 'Labels must belong to this board' using errcode='22023'; end if;
 return new;
end $$;
create trigger validate_task_labels before insert or update on public.tasks for each row execute function public.validate_task_labels();

create or replace function public.save_board_task(p_board uuid,p_task text,p_changes jsonb) returns jsonb
language plpgsql security definer set search_path=public as $$
declare v_task tasks; v_key text;
begin
 if not can_access_board(p_board) then raise exception 'Board access denied' using errcode='42501'; end if;
 for v_key in select jsonb_object_keys(p_changes) loop
  if v_key not in ('title','description','status','priority','assignee_ids','due_date','label_ids') then raise exception 'Unknown task field: %',v_key using errcode='22023'; end if;
 end loop;
 if p_task is null then
  v_task.id:=gen_random_uuid()::text; v_task.user_id:=auth.uid()::text; v_task.board_id:=p_board;
  v_task.created_at:=now(); v_task.status:='pending'; v_task.priority:='medium'; v_task.assignee_ids:='{}'; v_task.label_ids:='{}';
 else select * into v_task from tasks where id=p_task and board_id=p_board for update;
  if not found then raise exception 'Task not found' using errcode='P0002'; end if;
 end if;
 v_task:=jsonb_populate_record(v_task,p_changes);
 if v_task.title is null or length(trim(v_task.title)) not between 1 and 160 then raise exception 'Title must be 1-160 characters' using errcode='22023'; end if;
 if length(coalesce(v_task.description,''))>4000 then raise exception 'Description is too long' using errcode='22023'; end if;
 if v_task.status not in ('pending','in_progress','review','completed') or v_task.status is null then raise exception 'Invalid status' using errcode='22023'; end if;
 if v_task.priority not in ('low','medium','high') or v_task.priority is null then raise exception 'Invalid priority' using errcode='22023'; end if;
 if v_task.assignee_ids is null or v_task.label_ids is null then raise exception 'Assignees and labels must be arrays' using errcode='22023'; end if;
 if p_task is null then insert into tasks select v_task.* returning * into v_task;
 else update tasks set title=trim(v_task.title),description=v_task.description,status=v_task.status,priority=v_task.priority,assignee_ids=v_task.assignee_ids,due_date=v_task.due_date,label_ids=v_task.label_ids where id=p_task returning * into v_task;
 end if;
 return to_jsonb(v_task);
end $$;

create or replace function public.workspace_snapshot(p_board uuid default null) returns jsonb language plpgsql security definer set search_path=public as $$
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
  'labels',(select coalesce(jsonb_agg(to_jsonb(l) order by l.name),'[]') from board_labels l where board_id=v_board),
  'tasks',(select coalesce(jsonb_agg(to_jsonb(t) order by t.created_at,t.id),'[]') from tasks t where board_id=v_board)
 );
end $$;
revoke all on function public.create_board_label(uuid,text,text) from public;
grant execute on function public.create_board_label(uuid,text,text) to authenticated;
notify pgrst,'reload schema';
commit;
