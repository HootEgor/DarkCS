# CRM: Telegram groups and forum topics

This is the specification for the CRM frontend. The backend is already implemented. It builds on the multi-channel chats in `crm-telegram-business.md`.

## 1. What it does

An admin adds our user bot to a Telegram group, usually a **forum** (a group with topics). The bot mirrors every message to the CRM, and managers reply from the CRM. The bot posts the reply into the same topic.

These chats are **human-only**, the same as premium chats:
- there is no AI, no menu and no registration;
- the bot never answers by itself.

**Each forum topic is a separate CRM chat.** A group without topics is a single chat.

Anyone can add a bot to a group, so a new group is **pending** until a manager approves it in the CRM. Nothing is stored before approval. On reject the bot leaves the group. If someone adds the bot back later, it leaves again at once.

## 2. Chat identity

| | `platform` | `user_id` | `channel` |
|---|---|---|---|
| Forum topic | `telegram_group` | group chat id, e.g. `"-1001234567890"` | topic id, e.g. `"42"` |
| Forum General topic | `telegram_group` | group chat id | `"1"` |
| Group without topics | `telegram_group` | group chat id | absent |

- `user_id` is negative. URL-encoding is not needed, but keep it as a string.
- Always send `channel` back (`?channel=` or WS data), as for business chats.

## 3. API

All routes need the `crm` scope.

### 3.1 `GET /api/v1/crm/chats`

For group chats:

| Field | Value |
|---|---|
| `user_name` | Group title |
| `messenger_name` | Empty |
| `channel_name` | Topic name. `"General"` for topic 1, or `"Тема #<id>"` when the name is unknown (see 3.5). |
| `contact_key` | `telegram_group:<chat id>`. All topics of one group fall under one contact, so the existing channel picker lists the topics. |

### 3.2 Messages

Group messages carry the author (absent on manager replies):

| Field | Meaning |
|---|---|
| `author_id` | Telegram user id. For anonymous admins or channel posts it is the sender chat id. |
| `author_name` | Telegram display name (first and last name, or the chat title for anonymous admins). |
| `author_username` | `@username` without `@`. May be empty. |

Every message written in the group is `direction: "incoming"`, `sender: "user"`, **staff included**. Only replies sent from the CRM are `outgoing` / `manager`. Show `author_name` or `author_username` on each incoming bubble, and add a setting or toggle to switch between them.

Messages, sending and sending files use the existing routes with `?channel=`:

```
GET  /api/v1/crm/chats/telegram_group/-1001234567890/messages?channel=42
POST /api/v1/crm/chats/telegram_group/-1001234567890/send?channel=42         {"text": "..."}
POST /api/v1/crm/chats/telegram_group/-1001234567890/send-file?channel=42    multipart
```

A send returns **HTTP 409** when the group is not approved, the bot was removed, or the bot may not post in the group. The `status_message` is `telegram group is not approved, the bot was removed or cannot post there`.

History import is not available for groups (`history-status` returns `importable: false`).

### 3.3 Groups review (new)

```
GET  /api/v1/crm/groups?status=pending|approved|rejected|left      (no status = all)
POST /api/v1/crm/groups/{chat_id}/approve
POST /api/v1/crm/groups/{chat_id}/reject
```

`TelegramGroup`:

```json
{
  "chat_id": -1001234567890,
  "title": "Дилери PL",
  "username": "",
  "is_forum": true,
  "status": "pending",
  "can_post": true,
  "added_by_id": 5551112222,
  "added_by_name": "Олена К",
  "reviewed_by": "manager1",
  "reviewed_at": "2026-10-07T10:00:00Z",
  "created_at": "2026-10-07T09:58:00Z",
  "updated_at": "2026-10-07T10:00:00Z"
}
```

- `approve` and `reject` return the updated group, or 404 for an unknown `chat_id`.
- `left` means the bot was removed from the group. Adding it again makes the group `pending` again.

### 3.4 WebSocket

New event `group_updated`, whose `data` is a `TelegramGroup`. It is sent when the bot is added, approved, rejected, removed or migrated. Use it to show a "pending groups" badge.

`new_message` and `message_edited` work as before for group chats.

### 3.5 Topic names

```
GET /api/v1/crm/groups/{chat_id}/topics                       → [{chat_id, thread_id, name, closed, updated_at}]
PUT /api/v1/crm/groups/{chat_id}/topics/{thread_id}           {"name": "..."}
```

The Bot API cannot list a forum's topics. The bot learns a name when a topic is created or renamed while it is in the group. For topics that existed before the bot joined, the name is learned from the first ordinary message written there afterwards: Telegram attaches the topic's creation message to it. This gives the original name; a later rename in Telegram updates it. Until then, or if the topic was renamed before the bot joined, it shows `"Тема #<id>"` or the old name, and a manager can set it with `PUT`. `closed` reflects closing or reopening a topic in Telegram.

## 4. Edits, deletions, migration

- **Edits** in the group are mirrored, the same as for business chats: `edited_at`, `original_text`, and the WS `message_edited` event.
- **Deletions are not mirrored.** The Bot API does not report deletions in groups.
- **Migration:** when a plain group is upgraded to a supergroup (Telegram does this automatically, e.g. when topics are enabled), its chat id changes. The backend moves the group, its messages and read receipts to the new id. CRM clients should reload the chat list on `group_updated`.

## 5. Telegram setup checklist (ops)

1. **Make the bot a group admin.** Otherwise, with BotFather privacy mode on (the default), it only sees commands and replies to itself. The alternative is to disable privacy mode in BotFather (`/setprivacy`), but the bot must then be removed and re-added to existing groups.
2. The bot needs the right to send messages. Managing topics is not needed.
3. Approve the group in the CRM. Until then nothing is stored. Topic names created meanwhile are still remembered.
4. After an approved group removes the bot and adds it back, it has to be approved again.
