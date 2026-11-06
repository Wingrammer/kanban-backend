-- The owner can remove a board and its dependent records. Collaborators cannot.
begin;
create function public.delete_owned_board(p_board uuid) returns jsonb language plpgsql security definer set search_path=public as $$
begin
 delete from boards where id=p_board and owner_id=auth.uid();
 if not found then raise exception 'Only the board owner can delete this board' using errcode='42501'; end if;
 return jsonb_build_object('ok',true);
end $$;
revoke all on function public.delete_owned_board(uuid) from public;
grant execute on function public.delete_owned_board(uuid) to authenticated;
notify pgrst,'reload schema';
commit;
