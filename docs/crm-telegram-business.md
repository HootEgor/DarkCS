# CRM: Telegram Business (premium accounts) chats and multi-channel contacts

This is the specification for the CRM frontend. The backend is already implemented.

## 1. What changed and why

B2B clients write only to the private messages of our Telegram **Premium** accounts. Each premium account connects our user bot as its *Telegram Business chatbot*. From then on, the bot receives every message of that account's private chats.

These chats are **human-only**:
- there is no AI, no menu and no registration;
- the bot only relays messages to the CRM;
- managers reply either from the CRM (the message is sent *as the premium account*) or from the Telegram app on the phone.

One person can therefore have several chats:

| Source | `platform` | `channel` |
|---|---|---|
| Our Telegram bot | `telegram` | — (absent) |
| Instagram | `instagram` | — |
| WhatsApp | `whatsapp` | — |
| Premium account #1 | `telegram_business` | owner id of account #1, e.g. `"5551112222"` |
| Premium account #2 | `telegram_business` | owner id of account #2 |

### Chat identity
**A chat is now identified by `platform + user_id + channel`.**
- `channel` is absent or empty for every old chat, so existing chats behave exactly as before.
- For `telegram_business` chats, `user_id` is the client's Telegram id, the same id their bot chat uses. `channel` is the Telegram user id of the premium account owner.
- **Always send `channel` back** (as `?channel=` or in WS data) for any chat that has one.

## 2. API changes

All routes need the `crm` scope, as before. Responses use the usual envelope `{data, success, status_message, timestamp}`.

### 2.1 `GET /api/v1/crm/chats`
Each `ChatSummary` gains three fields:

```json
{
  "platform": "telegram_business",
  "user_id": "123456789",
  "channel": "5551112222",
  "channel_name": "Олена (Sales)",
  "contact_key": "tg:123456789",
  "user_name": "Іван Петренко",
  "messenger_name": "@ivan_p",
  "last_message": "Доброго дня, є питання по замовленню",
  "last_time": "2026-10-01T09:12:44Z",
  "unread": 2
}
```

| Field | Meaning |
|---|---|
| `channel` | Omitted for non-business chats. |
| `channel_name` | Display name of the premium account; only for `telegram_business`. |
| `contact_key` | **The grouping key: one person = one key.** |

How `contact_key` is built:
- The user UUID when the person is a registered user. This links bot, Instagram, WhatsApp and premium chats of the same person.
- `tg:<telegram id>` for unregistered Telegram people. Bot chats and premium chats share it.
- `<platform>:<user_id>` otherwise.

`user_name` / `messenger_name` for premium clients who never used the bot come from their Telegram profile (first and last name, `@username`).

### 2.2 Chat routes take `?channel=`

```
GET  /api/v1/crm/chats/{platform}/{user_id}/messages?limit=50&offset=0&channel=5551112222
POST /api/v1/crm/chats/{platform}/{user_id}/send?channel=5551112222          {"text": "..."}
POST /api/v1/crm/chats/{platform}/{user_id}/send-file?channel=5551112222     multipart: files[], caption
```

- Without `channel`, the default channel is used. This is what old chats need.
- **New error on send:** **HTTP 409** with `status_message` = `premium account is disconnected or the bot has no right to reply`. It is returned when the premium account disconnected the bot or did not grant it the reply right. Show this message to the manager.
- Telegram also allows a business bot to reply only in chats that had an incoming message in the last 24 hours. Outside that window Telegram rejects the send, and the API returns the generic 500 `Failed to send message`. A hint in the UI is enough: "If sending fails, reply from the phone".

New optional fields on `ChatMessage`:

| Field | Meaning |
|---|---|
| `channel` | Same meaning as in the summary. |
| `tg_message_id` | The Telegram message id. Informational only. |
| `imported` | `true` for messages loaded from a history export (see 2.5). |
| `edited_at` | Set when the message was edited in Telegram (by the client or the premium account). `text` holds the new text. |
| `original_text` | The text before the first edit. Present only on edited messages. |
| `deleted_at` | Set when the message was deleted in Telegram. The message stays stored so the CRM keeps the record (see 2.6). |
| `contact_key` | Present on WS `new_message` events, so live messages can be routed into a contact group without refetching. |

Messages from a premium chat:
- **Client wrote:** `direction: "incoming"`, `sender: "user"`.
- **Manager wrote** from the CRM *or* the premium account owner typed in the Telegram app: `direction: "outgoing"`, `sender: "manager"`.
- `sender: "bot"` never occurs in premium chats.

### 2.3 `GET /api/v1/crm/business-accounts` (new)

```json
[
  {
    "owner_user_id": 5551112222,
    "name": "Олена (Sales)",
    "username": "olena_sales",
    "is_enabled": true,
    "can_reply": true,
    "updated_at": "2026-10-01T08:00:00Z"
  }
]
```

To match an account to a chat, compare `String(owner_user_id)` with the chat's `channel`.

Use this endpoint for:
- channel labels and filters;
- disabling the composer when `is_enabled` or `can_reply` is false.

### 2.4 WebSocket

| Event | Change |
|---|---|
| `new_message` | `data` is a `ChatMessage`. Now includes `channel` and `contact_key`. |
| `read_receipt` | `data` = `{username, platform, user_id, channel?}`. `channel` is present only for chats that have one. |
| `history_imported` (new) | `data` = `{platform, user_id, channel, count}`. Reload that chat if it is open. |
| `message_edited` (new) | `data` is the full updated `ChatMessage` (with `edited_at` and `original_text`). Replace the message with the same `id`. |
| `messages_deleted` (new) | `data` = `{platform, user_id, channel, ids, tg_message_ids, deleted_at}`. `ids` are CRM message `id`s. Mark those messages deleted. |
| client → server `mark_read` | `data` = `{platform, user_id, channel?}`. **Send `channel` for premium chats**, otherwise the unread count of the wrong chat is cleared. |

Unread counters are per chat, which includes the channel.

### 2.5 History import (Telegram Desktop export)

The Telegram Bot API cannot read past messages. The bot sees a premium chat only from the moment it was connected. Older history is imported **once per chat** from a Telegram Desktop JSON export.

```
GET  /api/v1/crm/chats/{platform}/{user_id}/history-status?channel=...
POST /api/v1/crm/chats/{platform}/{user_id}/import-history?channel=...    multipart: file (result.json, ≤ 20 MB)
```

`history-status` response:

```json
{
  "importable": true,
  "imported": false,
  "imported_at": null,
  "imported_count": 0,
  "history_before": "2026-09-20T10:00:00Z",
  "owner_name": "Олена (Sales)",
  "owner_username": "olena_sales",
  "client_name": "Іван Петренко",
  "client_username": "@ivan_p"
}
```

| Field | Meaning |
|---|---|
| `importable` | `true` only for `telegram_business` chats. For other platforms the response is `{"importable": false, "imported": false}`. |
| `history_before` | Time of the first message the bot received. Imported history ends before it. |
| `imported`, `imported_at`, `imported_count` | Whether an import has been done, when, and how many messages it brought in. |

`import-history` response:

```json
{ "imported": 312, "skipped": 4, "oldest_at": "2025-03-02T08:11:00Z", "newest_at": "2026-09-20T09:58:00Z" }
```

- `skipped` counts messages that were already imported or that overlap live history.
- Re-importing is safe: duplicates are skipped.
- **HTTP 400** with a readable `status_message` when the file is rejected:
  - `... file is not a Telegram Desktop JSON export (result.json)`
  - `... export is not a personal chat; export a single private chat`
  - `... export belongs to a different chat`
  - `... export was made from a different Telegram account; log in as the premium account owner`
- **HTTP 413:** the file is larger than 20 MB.

Imported messages are ordinary chat messages with `imported: true`. After an import they page in through the normal `/messages` endpoint. Media in imports is replaced by placeholders such as `[Фото]`, `[Файл: invoice.pdf]` or `[Голосове повідомлення]`.

Business chats are **never trimmed**. Other chats keep the old limits of 100 messages and the 30-day cleanup.

### 2.6 Edits and deletions (Telegram Business only)

Edits and deletions made in a premium chat are mirrored into the CRM. This covers changes by the client and by the premium account owner in the Telegram app, including changes the owner makes to replies that were sent from the CRM.

- **Edit:** the stored message gets the new `text`, `edited_at`, and `original_text`, which keeps the text before the *first* edit. A `message_edited` WS event follows. For a media message the caption is edited; a replaced photo or file is not mirrored.
- **Delete:** the message is **not removed**. It gets `deleted_at` and a `messages_deleted` WS event follows. `/messages` keeps returning it.
- **Not matched, so ignored:**
  - messages from before the bot was connected, unless their history was imported;
  - files sent from the CRM;
  - the 2nd and later parts of a CRM text longer than 4096 characters, which Telegram splits into several messages.
- Edits and deletions are not mirrored for the bot, Instagram or WhatsApp.

## 3. UI suggestions

### 3.1 Chat list = contact list
- Group `/crm/chats` by `contact_key`. Each row shows one person.
- **Name:** `user_name`, falling back to `messenger_name`, then `user_id`.
- **Last message and time:** the newest across the group.
- **Unread:** the sum of `unread` across the group.
- **Small source icons** for each chat in the group. Premium chats get a ⭐ Telegram icon with the tooltip `channel_name`.
- **Sort** by the group's newest `last_time`.
- **Clicking a row:**
  - one chat in the group → open that chat directly, exactly like today;
  - more than one → open the **channel picker** (3.2).
- **Filter chips** above the list: All · Telegram bot · Instagram · WhatsApp · ⭐ \<each business account\> (from `/business-accounts`). Filtering by an account shows contacts that have a chat in it.
- **Live updates:** on WS `new_message`, take `contact_key` from the event, then update or create the group and move it to the top.

### 3.2 Channel picker (middle page)
```
← Back                        Іван Петренко  @ivan_p
────────────────────────────────────────────────────
[TG]  Telegram bot                        12:41   (1)
      "Дякую, отримав"
[⭐]  Олена (Sales)                        09:12   (2)
      "Доброго дня, є питання по замовленню"
[⭐]  Андрій (Opt)                        вчора
      "Рахунок у вкладенні"
[IG]  Instagram                            20.09
      "👍"
────────────────────────────────────────────────────
```
- **One card per chat:**
  - icon and label: "Telegram bot", "Instagram", "WhatsApp", or "⭐ \<channel_name\>";
  - last message preview, time and unread badge.
- **Sort** cards by time.
- **Click a card** to open that chat. The chat's back button returns to this page, not to the list.
- If the screen is wide, the picker can be a left sub-column next to the open chat instead of a separate page.

### 3.3 Chat view for a premium chat
- **Header:** person's name, a ⭐ badge "via \<channel_name\> (@username)" and the hint **"Human-only chat · no AI"**.
- **Bubbles:** manager messages (from the CRM or typed in the phone app) are manager bubbles. There are no bot bubbles.
- **Composer:**
  - Disabled when the account has `is_enabled=false` or `can_reply=false`, with the tooltip "Premium account disconnected the bot / no reply right — reply from the phone".
  - On a 409 response, show its `status_message`.
- **Pass `channel`** on every request and in every WS `mark_read` for this chat.
- **Edited messages** (`edited_at` set): show a small "edited" label next to the time. Show `original_text` in a tooltip or an expandable "show original".
- **Deleted messages** (`deleted_at` set): keep them in place, but dim them and strike through the text. Add the label "Deleted in Telegram · {deleted_at | time}". Hide any actions for them.
- **Chat list preview:** if the last message is deleted, prefix the preview with "🗑". If it is edited, show the new text.

### 3.4 History import panel
- **Trigger:** all of these hold:
  - the chat is `telegram_business`;
  - the manager scrolled to the top;
  - the last `/messages` page returned fewer than `limit` items, i.e. the start of the stored history was reached;
  - `history-status` returns `imported: false`.
- **Panel** at the top of the message list:

> **Earlier messages (before {history_before | date}) are not in the CRM yet.**
> They live in Telegram account **@{owner_username}** ({owner_name}). To import them:
> 1. Open **Telegram Desktop** (the phone apps cannot export) and log in as **@{owner_username}**.
> 2. Open the chat with **{client_name} ({client_username})**.
> 3. Click **⋮ → Export chat history**.
> 4. Untick all media (photos, videos, files…). Text only is enough.
> 5. Set **Format: Machine-readable JSON**, then click **Export**.
> 6. Take `result.json` from `Downloads/Telegram Desktop/ChatExport_<date>/` and drop it here.
>
> [ Drop result.json here or click to choose ]

- **Upload:**
  - Accept `.json` files up to 20 MB and show a progress state.
  - On success, show the toast "Imported {imported} messages", then reload the chat from offset 0.
  - On a 400, show `status_message` inline under the drop zone. Examples are the wrong account or the wrong chat; keep the panel open.
- **When `history-status.imported` is `true`:** do not show the panel. At the very top, show the small line "History imported {imported_at | date} · Re-import". "Re-import" opens the same panel, for example to load an export that covers a longer period.
- **Imported messages:** a subtle marker, such as a small "imported" label or a dimmed timestamp. Do not show delivery or read states for them.
- **On WS `history_imported`:** reload the open chat if it matches.

## 4. Telegram setup checklist (ops)
1. In **@BotFather**, open the user bot → *Bot Settings* → **Business Mode** → enable.
2. On each premium account: **Settings → Telegram Business → Chatbots**, then add the bot.
3. Grant **"Reply to messages"**, otherwise the CRM cannot answer and returns a 409.
4. Choose which chats the bot covers. "All 1-to-1 chats except…" is recommended.
5. The account appears in `/crm/business-accounts` as soon as it connects. Its chats appear in the CRM with the first message in each chat.

Limits to keep in mind:
- Telegram lets a business bot reply only within 24 hours of the client's last message. Older chats must be answered from the phone.
- Edits and deletions are mirrored (see 2.6), but a replaced photo or file is not.
