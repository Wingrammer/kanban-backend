begin;
create table public.task_comments (
 id uuid primary key default gen_random_uuid(),
 task_id text not null references public.tasks(id) on delete cascade,
 board_id uuid not null references public.boards(id) on delete cascade,
 author_id uuid not null references auth.users(id), author_name text not null,
 body text not null check(length(trim(body)) between 1 and 2000),
 created_at timestamptz not null default now()
);
create index comments_task_time_idx on public.task_comments(task_id,created_at,id);
alter table public.task_comments enable row level security;
create policy comments_read on public.task_comments for select to authenticated using(can_access_board(board_id));
grant select on public.task_comments to authenticated;
create trigger comments_revision after insert on public.task_comments for each row execute function public.bump_board_revision();

create function public.task_details(p_board uuid,p_task text) returns jsonb language plpgsql security definer set search_path=public as $$
begin
 if not can_access_board(p_board) then raise exception 'Board access denied' using errcode='42501'; end if;
 if not exists(select 1 from tasks where id=p_task and board_id=p_board) then raise exception 'Task not found' using errcode='P0002'; end if;
 return jsonb_build_object('comments',(select coalesce(jsonb_agg(to_jsonb(c) order by c.created_at,c.id),'[]') from task_comments c where task_id=p_task));
end $$;
create function public.add_task_comment(p_board uuid,p_task text,p_body text) returns jsonb language plpgsql security definer set search_path=public as $$
declare v_comment task_comments; v_name text;
begin
 if not can_access_board(p_board) then raise exception 'Board access denied' using errcode='42501'; end if;
 if not exists(select 1 from tasks where id=p_task and board_id=p_board) then raise exception 'Task not found' using errcode='P0002'; end if;
 select name into v_name from board_members where board_id=p_board and user_id=auth.uid();
 insert into task_comments(task_id,board_id,author_id,author_name,body) values(p_task,p_board,auth.uid(),coalesce(v_name,'Board member'),trim(p_body)) returning * into v_comment;
 return to_jsonb(v_comment);
end $$;
revoke all on function public.task_details(uuid,text),public.add_task_comment(uuid,text,text) from public;
grant execute on function public.task_details(uuid,text),public.add_task_comment(uuid,text,text) to authenticated;
notify pgrst,'reload schema';
commit;
