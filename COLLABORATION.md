# Finding your way around

Start at `app/page.tsx`. It renders the board shell in
`app/ui/session-panel.tsx`. That component owns view preferences and opens
dialogs. It delegates networking and optimistic state to `use-board.ts`.

## Frontend map

| File                                       | Responsibility                                                                 |
| ------------------------------------------ | ------------------------------------------------------------------------------ |
| `app/ui/board/use-board.ts`                | Guest initialization, active board, SSE, optimistic moves, reconciliation      |
| `app/ui/board/task-card.tsx`               | Folder card, local pointer animation, keyboard movement, avatars and due badge |
| `app/ui/board/task-form.tsx`               | Task fields, multiple assignees, multiple labels, due date                     |
| `app/ui/board/task-discussion.tsx`         | Chronological comments and activity; preserves comment drafts during sync      |
| `app/ui/board/team-dialog.tsx`             | Email-addressed invitations and team members                                   |
| `app/ui/board/invitations.tsx`             | Recipient inbox, explicit accept and decline                                   |
| `app/ui/board/labels-dialog.tsx`           | Board-specific label creation                                                  |
| `app/ui/board/board-stats.tsx`             | Whole-board totals, completed tasks and overdue tasks                          |
| `app/lib/filters.ts`                       | Intersection of text, status, priority, assignee and label filters             |
| `app/lib/dates.ts`                         | Local-calendar due-date comparisons                                            |
| `app/lib/board-api.ts`                     | Browser requests to Next.js, with typed errors                                 |
| `app/api/collaboration/[...path]/route.ts` | Allowlisted authenticated proxy to Gin, including SSE streaming                |
| `app/lib/session.ts`                       | Authenticated encryption and HttpOnly cookie settings                          |
| `app/lib/backend.ts`                       | Gin requests, token refresh, origin validation and response handling           |

The frontend never connects to Supabase. Only a selected board ID is stored in
localStorage. Access and refresh tokens stay inside an encrypted HttpOnly cookie.

## Follow a drag-and-drop operation

1. `TaskCard` records pointer movement locally. It renders a floating preview and
   highlights a target column. Pointer moves make **no API requests**.
2. On drop, `useBoard.move` immediately changes the visible status and records a
   pending optimistic move.
3. The browser PATCHes the Next.js collaboration proxy. Next.js reads the cookie
   and forwards the user's bearer token to Gin.
4. Gin calls a database function using that token. The function checks board
   membership, locks the task row, validates fields and commits the update.
5. Database triggers record the activity and advance the board revision in that
   same transaction. Supabase Realtime publishes that revision change to Gin's
   caller-authenticated WebSocket subscription. Gin relays an SSE invalidation.
6. Other clients fetch a fresh snapshot. A pending local move remains visible
   until its request settles. The originating client also reconciles after its
   request succeeds or rolls back after a failure.

### Conflict policy

This assessment uses **last committed write wins**. Two valid drops can both
succeed; whichever transaction commits last determines the status. Row locking
makes each mutation atomic. There are no long-lived drag locks, cursor broadcasts,
or claims that this implementation returns version-conflict 409s.

SSE reconnects reload the authoritative snapshot. Gin subscribes to Supabase
Postgres Changes for the selected board row with the caller JWT. The UI only
shows Live after the provider reports replication ready. Database publication
is configured by the backend migration. A 30-second revision check repairs missed
events and checks continued membership; it is not the primary event source.
Tabs on the same board elect one stream owner with Web Locks and distribute
invalidation events through BroadcastChannel. This avoids exhausting the browser's
HTTP/1 connection pool when multiple copies of the board are open.

## Sharing and authorization

Every guest starts with a personal board. The owner can add named members with
initials and an optional accent color. Several members can be assigned to one
task, even before those members connect their own accounts.

Invitations target an email address, expire after seven days, and appear in the
recipient's workspace. The inbox refreshes on focus and every 30 seconds. Accept
and decline verify the current user's confirmed email against Auth's user record,
not a supplied email or display name. Membership is granted only on acceptance.
No outbound email is sent; Resend is optional notification delivery. Legacy bearer
links are disabled. Display names and colors are presentation only.

## Saving a guest account

Gin requests an email change first. Supabase requires verification before setting
a password. The confirmation return hands its session fragment through a thin
Next.js proxy to Gin for validation; Next.js stores the returned session in an
HttpOnly cookie. Gin's account endpoint reads current Auth metadata so old JWT
claims cannot leave the header stuck in guest mode. Users then set a password,
and can sign in on another browser. Passwords are never cached pending verification.

Connected members can edit tasks, labels and comments. Only the owner can create
invitations or delete the board. Database membership checks and RLS enforce this
boundary independently of the UI. Task ownership and board IDs cannot be changed
through a task-edit payload. Assignees and labels must belong to the same board.

## Dates, comments, and history

Due dates represent calendar days. The UI compares dates in the viewer's local
calendar; a UTC midnight does not prematurely mark work overdue. Completed tasks
are excluded from overdue counts. The date indicators also use text and border
styles, so urgency is not communicated by color alone.

Comments live in a separate table and are ordered by creation time and ID.
Activity is append-only, generated by database triggers, and records creation,
status, title, description, priority, assignee, label and due-date changes. This
keeps history consistent with the committed task even if a request fails.

## Validation

- `pnpm test`: deterministic filter and date-boundary tests (Node 22.18+).
- `pnpm lint` and `pnpm build`: frontend checks.
- `go test ./...` in the backend: endpoint validation, caller JWT forwarding,
  board-scoped event delivery, and refusal of unauthorized SSE subscriptions.
- `supabase/tests/email_invitations.sql` in the backend: recipient-only inbox,
  outsider denial, explicit acceptance and idempotence. Run in SQL Editor; the
  transaction rolls back all fixtures, including Auth users.

All SQL migrations, provider clients and database-level tests belong in the Gin
repository. Apply its migrations in filename order before starting this version.
