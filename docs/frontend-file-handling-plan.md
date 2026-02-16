# Frontend File Handling Plan

## Context

The CRM dashboard chat (Angular 21, at `C:\Users\egorh\GolandProjects\comex\frontend`) is text-only. The backend plan (`docs/file-handling-plan.md`) adds GridFS file storage, file download endpoint (`GET /api/v1/crm/files/{file_id}`), and manager file upload endpoint (`POST /api/v1/crm/chats/{platform}/{user_id}/send-file`). This plan covers the frontend changes needed to:

1. **Display** files/images sent by users (incoming attachments)
2. **Upload & send** files from the manager to users (outgoing attachments)
3. Support **multiple files per message**

---

## 1. Update Data Model

**File**: `src/app/admin/chat/models/chat.model.ts`

Add:
```typescript
export interface Attachment {
  fileId: string;
  filename: string;
  mimeType: string;
  size: number;
  url: string;  // populated by backend: "/api/v1/crm/files/{fileId}"
}
```

Update `ChatMessage`:
```typescript
export interface ChatMessage {
  // ... existing fields ...
  attachments?: Attachment[];  // new
}
```

---

## 2. Display Attachments in Messages

**Files**: `chat-window.component.html` + `chat-window.component.scss`

Inside each `.message-bubble`, after `.message-text`, add an attachments section:

```html
@if (msg.attachments?.length) {
  <div class="message-attachments">
    @for (att of msg.attachments; track att.fileId) {
      @if (isImage(att)) {
        <a [href]="getFileUrl(att)" target="_blank" class="attachment-image">
          <img [src]="getFileUrl(att)" [alt]="att.filename" loading="lazy" />
        </a>
      } @else {
        <a [href]="getFileUrl(att)" target="_blank" class="attachment-file" download>
          <span class="material-icons">{{ getFileIcon(att) }}</span>
          <div class="attachment-info">
            <span class="attachment-name">{{ att.filename }}</span>
            <span class="attachment-size">{{ formatFileSize(att.size) }}</span>
          </div>
          <span class="material-icons">download</span>
        </a>
      }
    }
  </div>
}
```

**Add helper methods to `chat-window.component.ts`**:
- `isImage(att)` — checks if `mimeType` starts with `image/`
- `getFileUrl(att)` — returns full URL: `baseUrl + att.url` (using ChatSettingsService)
- `getFileIcon(att)` — returns material icon name based on MIME type (description, picture_as_pdf, audiotrack, videocam, insert_drive_file)
- `formatFileSize(bytes)` — returns human-readable size (KB, MB)

**SCSS additions** to `chat-window.component.scss`:
```scss
.message-attachments {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  margin-top: var(--spacing-xs);
}

.attachment-image {
  display: block;
  max-width: 280px;
  border-radius: var(--radius-md);
  overflow: hidden;

  img {
    width: 100%;
    height: auto;
    display: block;
    cursor: pointer;
  }
}

.attachment-file {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm);
  border-radius: var(--radius-md);
  background: rgba(0, 0, 0, 0.06);
  text-decoration: none;
  color: inherit;
  min-width: 200px;
}

.attachment-info {
  flex: 1;
  overflow: hidden;
}

.attachment-name {
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  display: block;
}

.attachment-size {
  font-size: 11px;
  opacity: 0.7;
}
```

---

## 3. File Upload in Message Input

**Files**: `message-input.component.ts` + `message-input.component.html` + `message-input.component.scss`

### 3.1 HTML — add attach button + file preview

```html
<div class="message-input">
  <!-- File previews above input -->
  @if (selectedFiles.length) {
    <div class="file-previews">
      @for (file of selectedFiles; track file.name; let i = $index) {
        <div class="file-preview">
          @if (isImageFile(file)) {
            <img [src]="filePreviews[i]" alt="" />
          } @else {
            <span class="material-icons">insert_drive_file</span>
          }
          <span class="file-preview-name">{{ file.name }}</span>
          <button class="remove-btn" (click)="removeFile(i)">
            <span class="material-icons">close</span>
          </button>
        </div>
      }
    </div>
  }

  <div class="input-row">
    <!-- Hidden file input -->
    <input type="file" #fileInput multiple (change)="onFilesSelected($event)" hidden />

    <!-- Attach button -->
    <button class="attach-btn" (click)="fileInput.click()" [title]="'chat.attach' | translate">
      <span class="material-icons">attach_file</span>
    </button>

    <textarea ... (existing) ...></textarea>

    <button class="send-btn" (click)="onSend()"
      [disabled]="!text.trim() && !selectedFiles.length">
      <span class="material-icons">send</span>
    </button>
  </div>
</div>
```

### 3.2 Component logic

Update `message-input.component.ts`:

```typescript
@Output() send = new EventEmitter<string>();
@Output() sendFiles = new EventEmitter<{ files: File[]; caption: string }>();

selectedFiles: File[] = [];
filePreviews: string[] = [];  // data URLs for image previews

onFilesSelected(event: Event): void {
  const input = event.target as HTMLInputElement;
  if (!input.files) return;
  for (const file of Array.from(input.files)) {
    this.selectedFiles.push(file);
    if (file.type.startsWith('image/')) {
      const reader = new FileReader();
      reader.onload = (e) => this.filePreviews.push(e.target!.result as string);
      reader.readAsDataURL(file);
    } else {
      this.filePreviews.push('');
    }
  }
  input.value = '';  // reset so same file can be re-selected
}

removeFile(index: number): void {
  this.selectedFiles.splice(index, 1);
  this.filePreviews.splice(index, 1);
}

isImageFile(file: File): boolean {
  return file.type.startsWith('image/');
}

onSend(): void {
  const trimmed = this.text.trim();
  if (this.selectedFiles.length) {
    this.sendFiles.emit({ files: [...this.selectedFiles], caption: trimmed });
    this.selectedFiles = [];
    this.filePreviews = [];
    this.text = '';
    this.adjustHeight();
  } else if (trimmed) {
    this.send.emit(trimmed);
    this.text = '';
    this.adjustHeight();
  }
}
```

### 3.3 SCSS additions for file previews

```scss
.file-previews {
  display: flex;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md) 0;
  overflow-x: auto;
}

.file-preview {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  background: var(--color-bg-secondary);
  border-radius: var(--radius-md);
  max-width: 160px;

  img {
    width: 40px;
    height: 40px;
    object-fit: cover;
    border-radius: var(--radius-sm);
  }

  .file-preview-name {
    font-size: 12px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .remove-btn {
    background: none;
    border: none;
    cursor: pointer;
    padding: 0;
    color: var(--color-text-muted);
    .material-icons { font-size: 16px; }
  }
}

.input-row {
  display: flex;
  align-items: flex-end;
  gap: var(--spacing-sm);
}

.attach-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border: none;
  border-radius: 50%;
  background: none;
  color: var(--color-text-muted);
  cursor: pointer;
  flex-shrink: 0;
  transition: color 0.2s ease;

  &:hover {
    color: var(--color-primary);
  }
}
```

---

## 4. Chat Service — File Upload API

**File**: `src/app/admin/chat/services/chat.service.ts`

Add method:
```typescript
sendFiles(platform: string, userId: string, files: File[], caption: string): Observable<any> {
  const config = this.chatSettingsService.getConfig();
  if (!config?.base_url) return EMPTY;

  const endpoint = (config.send_file_endpoint || '/crm/chats/{platform}/{userId}/send-file')
    .replace('{platform}', encodeURIComponent(platform))
    .replace('{userId}', encodeURIComponent(userId));

  const formData = new FormData();
  for (const file of files) {
    formData.append('files', file, file.name);
  }
  if (caption) {
    formData.append('caption', caption);
  }

  return this.http.post(
    `${config.base_url}${endpoint}`,
    formData,
    { headers: new HttpHeaders({ Authorization: `Bearer ${this.chatSettingsService.getAuthToken()}` }) }
  );
}
```

**Note**: Do NOT set `Content-Type` header — the browser auto-sets `multipart/form-data` with boundary.

---

## 5. Wire Up in Chat Component

**File**: `src/app/admin/chat/chat.component.ts`

### 5.1 Handle `sendFiles` event from `MessageInputComponent`

```typescript
onSendFiles(event: { files: File[]; caption: string }): void {
  if (!this.activeChat) return;

  // Optimistic message with file placeholders
  const optimistic: ChatMessage = {
    id: 'temp-' + Date.now(),
    platform: this.activeChat.platform,
    user_id: this.activeChat.userId,
    direction: 'outgoing',
    sender: this.translationService.instant('chat.manager'),
    text: event.caption || '',
    created_at: new Date().toISOString(),
    attachments: event.files.map(f => ({
      fileId: '',
      filename: f.name,
      mimeType: f.type,
      size: f.size,
      url: ''
    }))
  };

  this.messages = [...this.messages, optimistic];
  this.cdr.markForCheck();

  this.chatService.sendFiles(
    this.activeChat.platform,
    this.activeChat.userId,
    event.files,
    event.caption
  ).subscribe({
    error: () => {
      this.messages = this.messages.filter(m => m.id !== optimistic.id);
      this.cdr.markForCheck();
    }
  });
}
```

### 5.2 Update template

**File**: `src/app/admin/chat/chat.component.html`

Update `<app-message-input>`:
```html
<app-message-input
  (send)="onSend($event)"
  (sendFiles)="onSendFiles($event)">
</app-message-input>
```

### 5.3 Update chat list last message for file messages

In `chat.service.ts` `handleNewMessage()`, update last_message for file-only messages:
```typescript
chat.last_message = msg.text || (msg.attachments?.length ? `[${msg.attachments.length} file(s)]` : '');
```

---

## 6. Auth Header on File Downloads

File URLs from the backend are relative (`/api/v1/crm/files/{id}`). The `<img>` tags and `<a>` download links need the auth token.

**Approach**: Build full URLs with a query param token, OR use an `HttpInterceptor` (already exists for API calls but doesn't help `<img src>`).

**Recommended**: In `getFileUrl()`, append the auth token as a query param:
```typescript
getFileUrl(att: Attachment): string {
  const config = this.chatSettingsService.getConfig();
  const token = this.chatSettingsService.getAuthToken();
  return `${config.base_url}${att.url}?token=${token}`;
}
```

**Backend note**: The file download endpoint needs to also accept `?token=` query param for auth (similar to the WebSocket endpoint). This is a small backend addition to `file.go`.

---

## Files Summary

| Action | File |
|--------|------|
| Modify | `models/chat.model.ts` — add `Attachment` interface, update `ChatMessage` |
| Modify | `components/chat-window/chat-window.component.ts` — add helper methods (isImage, getFileUrl, getFileIcon, formatFileSize) |
| Modify | `components/chat-window/chat-window.component.html` — render attachments in message bubbles |
| Modify | `components/chat-window/chat-window.component.scss` — attachment styles |
| Modify | `components/message-input/message-input.component.ts` — file selection, preview, sendFiles event |
| Modify | `components/message-input/message-input.component.html` — attach button, file previews, hidden input |
| Modify | `components/message-input/message-input.component.scss` — attach button + preview styles |
| Modify | `services/chat.service.ts` — `sendFiles()` method, update `handleNewMessage()` |
| Modify | `chat.component.ts` — `onSendFiles()` handler |
| Modify | `chat.component.html` — bind `(sendFiles)` event |

All files under: `src/app/admin/chat/`

## Implementation Order

1. `models/chat.model.ts` — Attachment interface + ChatMessage update
2. `chat-window` — display attachments (HTML + TS helpers + SCSS)
3. `message-input` — file selection UI + sendFiles output (HTML + TS + SCSS)
4. `chat.service.ts` — sendFiles() API method + handleNewMessage update
5. `chat.component` — wire onSendFiles handler (TS + HTML)

## Verification

- Open CRM dashboard, select a chat where a user sent a photo from Telegram
- Verify image renders inline in the message bubble with clickable link
- Verify non-image files show as download cards with icon + filename + size
- Click attach button, select multiple files, verify previews appear above input
- Send files with caption, verify optimistic message appears, then replaced by real WS message
- Send files without caption, verify it works
- Remove a file from preview before sending
- Check mobile responsive layout still works
