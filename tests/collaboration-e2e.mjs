import assert from 'node:assert/strict';

const origin = process.env.BOARD_APP_URL || 'http://localhost:3000';
const guests = [];
const records = [];
async function call(guest, route, method = 'GET', body) {
  const response = await fetch(origin + route, { method,
    headers: { Origin: origin, Cookie: guest?.cookie || '', 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(20000),
  });
  const set = response.headers.get('set-cookie');
  if (guest && set) guest.cookie = set.split(';')[0];
  return { status: response.status, data: await response.json() };
}
async function guest() {
  const user = { cookie: '' };
  assert.equal((await call(user, '/api/auth/guest', 'POST')).status, 200);
  guests.push(user);
  return user;
}
async function ok(user, route, method = 'GET', data) {
  const result = await call(user, '/api/collaboration/' + route, method, data);
  assert.equal(result.status, 200, `${method} ${route}: ${JSON.stringify(result.data)}`);
  return result.data;
}
let stream;
try {
  const owner = await guest();
  const other = await guest();
  const outsider = await guest();
  const workspace = await ok(owner, 'workspace');
  const board = workspace.board.id;
  records.push({ guest: owner, board });
  assert.equal((await call(other, `/api/collaboration/workspace?board=${board}`)).status, 403);
  const invitation = await ok(owner, `boards/${board}/members`, 'POST', { name: 'Integration teammate', color: '#71717a' });
  assert.equal(await ok(other, 'invitations/accept', 'POST', { token: invitation.token }), board);
  assert.equal((await call(outsider, '/api/collaboration/invitations/accept', 'POST', { token: invitation.token })).status, 400);
  const joined = await ok(other, `workspace?board=${board}`);
  records.push({ guest: other, board: joined.boards.find(item => item.owner_id === joined.user_id).id });
  assert.ok(joined.members.some(member => member.name === 'Integration teammate' && member.user_id === joined.user_id));
  const label = await ok(owner, `boards/${board}/labels`, 'POST', { name: 'Integration label', color: '#52525b' });
  const task = await ok(owner, `boards/${board}/tasks`, 'POST', {
    title: 'Temporary collaboration check', description: 'Removed automatically after verification.',
    status: 'pending', priority: 'high', assignee_ids: workspace.members.map(member => member.id).concat(invitation.member.id),
    label_ids: [label.id], due_date: '2026-11-05T12:00:00Z',
  });
  assert.equal(task.assignee_ids.length, 2);
  assert.deepEqual(task.label_ids, [label.id]);
  const abort = new AbortController();
  stream = { abort };
  const response = await fetch(`${origin}/api/collaboration/boards/${board}/events`, { headers: { Cookie: other.cookie }, signal: abort.signal });
  assert.equal(response.status, 200);
  assert.match(response.headers.get('content-type'), /text\/event-stream/);
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  async function event(name) {
    let text = '';
    const timeout = setTimeout(() => abort.abort(), 15000);
    try { while (!text.includes(`event: ${name}`)) { const part = await reader.read(); if (part.done) throw new Error('SSE closed early'); text += decoder.decode(part.value); } }
    finally { clearTimeout(timeout); }
  }
  await event('ready');
  await ok(owner, `boards/${board}/tasks/${task.id}`, 'PATCH', { status: 'in_progress' });
  await event('board_changed');
  assert.equal((await ok(other, `workspace?board=${board}`)).tasks.find(item => item.id === task.id).status, 'in_progress');
  await ok(other, `boards/${board}/tasks/${task.id}/comments`, 'POST', { body: 'A teammate update.' });
  await ok(other, `boards/${board}/tasks/${task.id}`, 'PATCH', { assignee_ids: [invitation.member.id], title: 'Updated collaboratively' });
  const details = await ok(owner, `boards/${board}/tasks/${task.id}`);
  assert.equal(details.comments[0].author_name, 'Integration teammate');
  assert.ok(details.activity.some(item => item.kind === 'status'));
  assert.ok(details.activity.some(item => item.kind === 'assignee_ids'));
  assert.ok(details.activity.some(item => item.kind === 'title'));
  assert.equal((await call(outsider, `/api/collaboration/boards/${board}/tasks/${task.id}`)).status, 403);
  assert.equal((await call(outsider, `/api/collaboration/boards/${board}/tasks/${task.id}`, 'PATCH', { status: 'completed' })).status, 403);
  assert.equal((await call(other, `/api/collaboration/boards/${board}/members`, 'POST', { name: 'Not allowed' })).status, 403);
  assert.equal((await call(other, `/api/collaboration/boards/${board}`, 'DELETE', {})).status, 403);
  await ok(owner, `boards/${board}/tasks/${task.id}`, 'PATCH', { status: 'completed' });
  await ok(other, `boards/${board}/tasks/${task.id}`, 'PATCH', { status: 'review' });
  assert.equal((await ok(owner, `workspace?board=${board}`)).tasks.find(item => item.id === task.id).status, 'review');
  console.log('Live collaboration passed: invitations, multiple assignees, labels, due dates, SSE, comments, activity, last-write-wins, and access isolation.');
} finally {
  stream?.abort.abort();
  // Only newly created private test boards are removed. Cascades remove their
  // task, comments, labels, invitations and activity; no user workspace is touched.
  for (const record of records) {
    const result = await call(record.guest, `/api/collaboration/boards/${record.board}`, 'DELETE', {});
    assert.equal(result.status, 200, 'Could not clean up a temporary test board');
  }
  console.log('Temporary collaboration records removed.');
}

