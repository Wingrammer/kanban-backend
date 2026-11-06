-- Run in the SQL Editor. Every fixture is rolled back, including Auth users.
begin;
do $$
declare
 owner_id uuid:=gen_random_uuid(); recipient_id uuid:=gen_random_uuid(); outsider_id uuid:=gen_random_uuid();
 recipient_email text:='invite-test-'||recipient_id||'@example.invalid';
 board_id uuid; invitation_id uuid; snapshot jsonb; invitation jsonb;
begin
 insert into auth.users(id,email,email_confirmed_at,is_anonymous,aud,role)
 values(owner_id,'owner-'||owner_id||'@example.invalid',now(),false,'authenticated','authenticated'),
 (recipient_id,recipient_email,now(),false,'authenticated','authenticated'),
 (outsider_id,'outsider-'||outsider_id||'@example.invalid',now(),false,'authenticated','authenticated');
 perform set_config('request.jwt.claim.sub',owner_id::text,true);
 snapshot:=public.workspace_snapshot(); board_id:=(snapshot->'board'->>'id')::uuid;
 invitation:=public.invite_board_member(board_id,'Recipient','#71717a',null,recipient_email);
 if invitation->>'delivery'<>'workspace' or invitation ? 'token' then raise exception 'Incorrect delivery'; end if;
 select id into invitation_id from public.board_invitations where email=recipient_email limit 1;
 perform set_config('request.jwt.claim.sub',outsider_id::text,true);
 if jsonb_array_length(public.my_board_invitations())<>0 then raise exception 'Outsider saw invitation'; end if;
 begin
  perform public.respond_board_invitation(invitation_id,true);
  raise exception 'Outsider accepted invitation';
 exception when insufficient_privilege then null; end;
 perform set_config('request.jwt.claim.sub',recipient_id::text,true);
 if jsonb_array_length(public.my_board_invitations())<>1 then raise exception 'Recipient inbox missing'; end if;
 if public.can_access_board(board_id) then raise exception 'Access granted before acceptance'; end if;
 if public.respond_board_invitation(invitation_id,true)<>board_id then raise exception 'Wrong board joined'; end if;
 if not public.can_access_board(board_id) then raise exception 'No access after acceptance'; end if;
 if jsonb_array_length(public.my_board_invitations())<>0 then raise exception 'Accepted invitation still pending'; end if;
 if public.respond_board_invitation(invitation_id,true)<>board_id then raise exception 'Acceptance not idempotent'; end if;
 raise notice 'PASS: verified recipient inbox, outsider denial, explicit acceptance, membership, idempotence';
end $$;
rollback;

