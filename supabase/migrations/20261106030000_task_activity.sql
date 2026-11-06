-- Database triggers keep history atomic even when a different API changes a task.
begin;
create table public.task_activity (
 id uuid primary key default gen_random_uuid(),
 task_id text not null references public.tasks(id) on delete cascade,
 board_id uuid not null references public.boards(id) on delete cascade,
 actor_id uuid references auth.users(id), actor_name text not null,
 kind text not null, description text not null, created_at timestamptz not null default now()
);
create index activity_task_time_idx on public.task_activity(task_id,created_at,id);
alter table public.task_activity enable row level security;
create policy activity_read on public.task_activity for select to authenticated using(can_access_board(board_id));
grant select on public.task_activity to authenticated;

create function public.status_display(p_status text) returns text language sql immutable set search_path=public as $$
 select case p_status when 'pending' then 'To do' when 'in_progress' then 'In progress' when 'review' then 'Review' when 'completed' then 'Complete' else p_status end
$$;
create function public.record_task_activity() returns trigger language plpgsql security definer set search_path=public as $$
declare v_actor text; v_change text; v_field text; v_old jsonb; v_new jsonb:=to_jsonb(new);
begin
 if new.board_id is null then return new; end if;
 select name into v_actor from board_members where board_id=new.board_id and user_id=auth.uid();
 v_actor:=coalesce(v_actor,case when auth.uid() is null then 'System' else 'Board member' end);
 if TG_OP='INSERT' then
  insert into task_activity(task_id,board_id,actor_id,actor_name,kind,description) values(new.id,new.board_id,auth.uid(),v_actor,'created','Created the task');
 else
  v_old:=to_jsonb(old);
  foreach v_field in array array['status','title','description','priority','assignee_ids','due_date','label_ids'] loop
   if (v_old->v_field) is distinct from (v_new->v_field) then
    v_change:=case v_field
     when 'status' then 'Moved from '||status_display(old.status)||' to '||status_display(new.status)
     when 'title' then 'Renamed task to '||new.title
     when 'description' then 'Updated the description'
     when 'priority' then 'Changed priority to '||new.priority
     when 'assignee_ids' then 'Updated the assignees'
     when 'due_date' then case when new.due_date is null then 'Removed the due date' else 'Changed due date to '||to_char(new.due_date at time zone 'UTC','YYYY-MM-DD') end
     else 'Updated the labels' end;
    insert into task_activity(task_id,board_id,actor_id,actor_name,kind,description) values(new.id,new.board_id,auth.uid(),v_actor,v_field,v_change);
   end if;
  end loop;
 end if;
 return new;
end $$;
create trigger task_history after insert or update on public.tasks for each row execute function public.record_task_activity();

create or replace function public.task_details(p_board uuid,p_task text) returns jsonb language plpgsql security definer set search_path=public as $$
begin
 if not can_access_board(p_board) then raise exception 'Board access denied' using errcode='42501'; end if;
 if not exists(select 1 from tasks where id=p_task and board_id=p_board) then raise exception 'Task not found' using errcode='P0002'; end if;
 return jsonb_build_object(
  'comments',(select coalesce(jsonb_agg(to_jsonb(c) order by c.created_at,c.id),'[]') from task_comments c where task_id=p_task),
  'activity',(select coalesce(jsonb_agg(to_jsonb(a) order by a.created_at,a.id),'[]') from task_activity a where task_id=p_task)
 );
end $$;
notify pgrst,'reload schema';
commit;
