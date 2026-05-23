# Comments Service

The comments service enables inline commenting on markdown documents. Users can select text, add comments, reply to comments, and resolve them.

## Architecture

The system uses a **section-based architecture** with Datastar for reactivity:

| Concern | Managed By | Why |
|---------|------------|-----|
| Button visibility & position | Frontend signals | Instant feedback, no latency |
| Form rendering | Backend SSE | Patches section target with form |
| Data persistence | Backend | SQLite database |
| Section comment updates | Backend SSE | Re-renders specific section target |

### Frontend Signals (Dot-Notation)

Defined in `page_markdown.templ` using Datastar dot-notation signals:

```javascript
{
  "comment_selection.text": "",      // Selected text
  "comment_selection.sectionId": "", // Section where text was selected
  "comment_selection.top": 0,        // Y position for button placement
  "comment_text": "",                // Comment form textarea value
  "pageSessionId": "..."             // Session identifier
}
```

**Important:** These are dot-notation signals, not object properties. This allows `data-bind` to work correctly with hidden form inputs.

### Per-Section Signals

Created dynamically by the backend when a section is interacted with:

```javascript
{
  "section_{sectionId}_form_open": false,  // Form is showing in this section
  "section_{sectionId}_expanded": false    // Comment panel is expanded
}
```

### Backend SSE

Form actions return Server-Sent Events that patch the specific section's comment target:

- `SectionCommentTarget` - Section toggle without form (default state)
- `SectionCommentTargetWithForm` - Section toggle with pending comment form

## Routes

All routes require authentication and are under `/forms/*`:

| Route | Handler | Purpose |
|-------|---------|---------|
| `POST /forms/comments/show` | `HandleShowCommentForm` | Show form with selected text |
| `POST /forms/comments` | `HandleCommentForm` | Create new comment |
| `POST /forms/comments/cancel` | `HandleCancelCommentForm` | Cancel and hide form |
| `POST /forms/replies` | `HandleReplyForm` | Add reply to comment |
| `POST /forms/resolve` | `HandleResolveComment` | Mark comment as resolved |

## Data Flow

### 1. Text Selection (frontend only)

```
User selects text in markdown content
    ↓
mouseup event in page_markdown.templ
    ↓
Check: is selection inside comment area?
  - If yes: clear signals, no button
  - If no: continue
    ↓
Sets frontend signals (dot-notation):
  $comment_selection.text = "selected text"
  $comment_selection.sectionId = "section-id"
  $comment_selection.top = position
    ↓
Trigger button appears (data-show="$comment_selection.text && $comment_selection.sectionId != ''")
```

No backend call - pure frontend.

### 2. Show Comment Form

```
User clicks trigger button
    ↓
Form submits with data-bind values:
  sectionId from $comment_selection.sectionId
  selectedText from $comment_selection.text
    ↓
@post('/forms/comments/show', {contentType: 'form'})
    ↓
HandleShowCommentForm:
  1. Parse form with BindFormToProto → ShowCommentFormRequest
  2. Get user email from session
  3. Extract filePath from Referer header
  4. Fetch existing comments for section
  5. SSE patch: SectionCommentTargetWithForm
  6. SSE patch signals: section_{sectionId}_form_open = true, _expanded = true
```

### 3. Submit Comment

```
User fills form and submits
    ↓
@post('/forms/comments', {contentType: 'form'})
    ↓
HandleCommentForm:
  1. Parse form with BindFormToProto → CreateCommentRequest
  2. Create comment in database
  3. Fetch updated comments for section
  4. SSE patch: SectionCommentTarget (form gone, new comment visible)
  5. SSE patch signals: section_{sectionId}_form_open = false
```

### 4. Cancel Form

```
User clicks Cancel
    ↓
@post('/forms/comments/cancel', {contentType: 'form'})
    ↓
HandleCancelCommentForm:
  1. Get sectionId from form
  2. Fetch existing comments for section
  3. SSE patch: SectionCommentTarget (form removed)
  4. SSE patch signals: section_{sectionId}_form_open = false
```

### 5. Reply to Comment

```
User types reply and submits
    ↓
@post('/forms/replies', {contentType: 'form'})
    ↓
HandleReplyForm:
  1. Parse form with BindFormToProto → CreateReplyRequest
  2. Get filePath from hidden field
  3. Create reply in database
  4. Get parent comment's sectionId
  5. Fetch updated comments for section
  6. SSE patch: SectionCommentTarget (reply now visible)
```

### 6. Resolve Comment

```
User clicks Resolve button
    ↓
@post('/forms/resolve', {contentType: 'form'})
    ↓
HandleResolveComment:
  1. Get comment's sectionId before resolving
  2. Mark comment as resolved in database
  3. Fetch updated comments for section
  4. SSE patch: SectionCommentTarget
```

## Components

### Templates (`templates.templ`)

| Component | Purpose |
|-----------|---------|
| `CommentForm` | The comment input form with hidden fields |
| `CommentCard` | Single comment with replies and resolve button |
| `ReplyItem` | Single reply display |
| `ReplyInput` | Reply input form |
| `SectionCommentTarget` | SSE-patchable container for section toggle + menu |
| `SectionCommentTargetWithForm` | Section target with comment form |
| `SectionCommentsToggle` | Collapsed view with stacked avatars |
| `SectionCommentsToggleWithForm` | Toggle with form in expanded panel |
| `SectionMenu` | Triple-dot dropdown for adding comments |
| `CommentSidebar` | Spacer div that reserves width for comment panels |

### Form Field Names

Forms use camelCase field names matching proto JSON mappings:

**ShowCommentForm (inline trigger):**

- `sectionId`, `selectedText`

**ShowCommentForm (menu trigger):**

- `sectionId` (static value), `selectedText` (empty)

**CommentForm:**

- `filePath`, `selectedText`, `sectionId`
- `startLine`, `startColumn`, `endLine`, `endColumn`
- `commentText`

**ReplyInput:**

- `commentId`, `filePath`, `replyText`

## Form Handling Pattern

Uses `BindFormToProto` from `server/utils/proto_form.go`:

```go
var req commentsv1.ShowCommentFormRequest
if err := utils.BindFormToProto(c, &req, ""); err != nil {
    return echo.NewHTTPError(http.StatusBadRequest, "invalid form data")
}
```

This utility:

1. Parses form-encoded data
1. Converts to JSON
1. Uses protojson to unmarshal into proto message
1. Accepts both camelCase and snake_case field names

## Inline Comment Trigger

The inline trigger uses a form with hidden inputs that bind to dot-notation signals:

```html
<form data-on-submit="@post('/forms/comments/show', {contentType: 'form'})">
    <input type="hidden" name="sectionId" data-bind="comment_selection.sectionId"/>
    <input type="hidden" name="selectedText" data-bind="comment_selection.text"/>
    <button type="submit">...</button>
</form>
```

**Key points:**

- `data-bind` uses signal name without `$` prefix
- Signals must be dot-notation (not object properties) for `data-bind` to work
- `data-show` condition requires both text AND sectionId to be non-empty
- Selections inside comment areas are excluded from triggering the button

## Section Menu Trigger

The section menu uses a form with static hidden input values:

```html
<form data-on-submit="@post('/forms/comments/show', {contentType: 'form'}); $menuId.open = false">
    <input type="hidden" name="sectionId" value={ sectionID }/>
    <input type="hidden" name="selectedText" value=""/>
    <button type="submit">Add comment</button>
</form>
```

## File Structure

```
server/services/comments/
├── README.md           # This file
├── service.go          # Service struct, database operations
├── form_handlers.go    # HTTP handlers for /forms/* routes
├── rpc_handlers.go     # Connect RPC handlers
├── handler.go          # REST API handlers
├── args.go             # Request/response types
├── templates.templ     # UI components
└── templates_test.go   # Form field tests
```

## Proto Messages

Defined in `proto/comments/v1/comments.proto`:

- `ShowCommentFormRequest` - For opening comment form (sectionId, selectedText)
- `CreateCommentRequest` - For creating comments
- `CreateReplyRequest` - For creating replies
