begin;

-- Revisions are already bumped transactionally for every board change. Publish
-- just this small row; Gin subscribes with the caller JWT and board RLS applies.
do $$ begin
 if not exists(select 1 from pg_publication_tables where pubname='supabase_realtime' and schemaname='public' and tablename='boards') then
  alter publication supabase_realtime add table public.boards;
 end if;
end $$;

alter table public.board_invitations add column if not exists email text;
alter table public.board_invitations add column if not exists declined_at timestamptz;

drop function public.invite_board_member(uuid,text,text,uuid);
create function public.invite_board_member(p_board uuid,p_name text,p_color text default '#71717a',p_member uuid default null,p_email text default null) returns jsonb
language plpgsql security definer set search_path=public,extensions as $$
declare v_member board_members; v_token text:=encode(gen_random_bytes(24),'hex'); v_email text:=lower(trim(p_email));
begin
 if not exists(select 1 from boards where id=p_board and owner_id=auth.uid()) then raise exception 'Only the board owner can invite members' using errcode='42501'; end if;
 if v_email is null or length(v_email)>254 or v_email !~ '^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$' then raise exception 'Enter a valid invitation email address' using errcode='22023'; end if;
 if exists(select 1 from board_invitations where board_id=p_board and email=v_email and accepted_by is null and declined_at is null and expires_at>now()) then raise exception 'An invitation is already pending for this email' using errcode='22023'; end if;
 if p_member is null then
  if (select count(*) from board_members where board_id=p_board)>=20 then raise exception 'A board supports up to 20 members' using errcode='22023'; end if;
  insert into board_members(board_id,name,color) values(p_board,coalesce(nullif(trim(p_name),''),split_part(v_email,'@',1)),p_color) returning * into v_member;
 else select * into v_member from board_members where id=p_member and board_id=p_board and user_id is null for update;
  if not found then raise exception 'Member is already connected or does not exist' using errcode='22023'; end if;
 end if;
 update board_invitations set expires_at=now() where member_id=v_member.id and accepted_by is null;
 insert into board_invitations(board_id,member_id,token_hash,email) values(p_board,v_member.id,encode(digest(v_token,'sha256'),'hex'),v_email);
 return jsonb_build_object('member',to_jsonb(v_member),'email',v_email,'delivery','workspace');
end $$;

-- Read recipient identity from Auth, never from a caller-supplied email or JWT
-- metadata. This also works immediately after email verification with an old JWT.
create function public.my_board_invitations() returns jsonb language sql security definer set search_path=public as $$
 select coalesce(jsonb_agg(jsonb_build_object('id',i.id,'board_name',b.name,'email',i.email,'expires_at',i.expires_at) order by i.expires_at),'[]'::jsonb)
 from board_invitations i join boards b on b.id=i.board_id join auth.users u on u.id=auth.uid()
 where u.email_confirmed_at is not null and lower(u.email)=i.email and i.accepted_by is null and i.declined_at is null and i.expires_at>now()
$$;

create function public.respond_board_invitation(p_invitation uuid,p_accept boolean) returns uuid language plpgsql security definer set search_path=public as $$
declare v_invite board_invitations;
begin
 select i.* into v_invite from board_invitations i join auth.users u on u.id=auth.uid()
 where i.id=p_invitation and lower(u.email)=i.email and u.email_confirmed_at is not null for update of i;
 if not found or v_invite.expires_at<=now() or v_invite.declined_at is not null then raise exception 'Invitation unavailable for this account' using errcode='42501'; end if;
 if v_invite.accepted_by=auth.uid() then return v_invite.board_id; end if;
 if v_invite.accepted_by is not null then raise exception 'Invitation already used' using errcode='22023'; end if;
 if not p_accept then update board_invitations set declined_at=now() where id=p_invitation; return null; end if;
 if not can_access_board(v_invite.board_id) then
  update board_members set user_id=auth.uid() where id=v_invite.member_id and user_id is null;
  if not found then raise exception 'Member already connected' using errcode='22023'; end if;
 end if;
 update board_invitations set accepted_by=auth.uid() where id=p_invitation;
 return v_invite.board_id;
end $$;

-- Retire bearer-link acceptance. Existing unaddressed links cannot bypass the
-- verified-recipient check in the workspace flow.
revoke execute on function public.accept_board_invitation(text) from authenticated;
revoke all on function public.invite_board_member(uuid,text,text,uuid,text),public.my_board_invitations(),public.respond_board_invitation(uuid,boolean) from public;
grant execute on function public.invite_board_member(uuid,text,text,uuid,text),public.my_board_invitations(),public.respond_board_invitation(uuid,boolean) to authenticated;
notify pgrst,'reload schema';
commit;
