# URL Shortener — UI Context

## What This App Does

A URL shortening service. Users create short links from long URLs, track clicks/analytics, and manage their links. Built for a clean, modern SaaS dashboard experience.

**Base URL:** `http://localhost:8080/api/v1`

---

## App Identity

- **Name:** URL Shortener
- **Purpose:** Create, manage, and track short URLs with analytics
- **Audience:** Individual users managing personal/work links
- **Design style:** Clean, modern SaaS — think Linear, Vercel, or Short.io dashboard

---

## User Roles

- **Regular user** — creates/edits/deletes own URLs, views own analytics, manages sessions
- **Admin** — same as regular + can block domains/IP ranges, purge data, manage users (note: any logged-in user can access admin features, there's no role enforcement)

---

## Pages & User Flows

### 1. Landing Page (public)
Hero section, feature highlights, sign up / login CTAs. No data fetching needed.

### 2. Register
- Fields: email, password, displayName (optional)
- Password rules: at least 1 uppercase, 1 lowercase, 1 number, max 8 characters — show a live strength indicator
- On success: store tokens, redirect to dashboard
- Error: 409 if email already exists

### 3. Login
- Fields: email, password
- Sends device info in headers (`X-Device-Type`, `X-Device-Name`)
- On success: store tokens, redirect to dashboard
- **Max Device Limit Flow (important):**
  - User can only be logged in on 2 devices at once
  - If they try to log in from a 3rd device, server returns 409 with a list of existing sessions
  - UI must show a modal/picker listing the active devices (device name, IP, last active time) so the user can pick one to kick off
  - After picking, resend login with the chosen session ID to revoke it
  - This is a core UX flow — build it as a clean modal, not an error page

### 4. Forgot Password
- Fields: email, current password, new password
- This is NOT an email-link flow — it validates the current password directly
- On success: returns new tokens (auto-login), redirect to dashboard

### 5. Dashboard (main view after login)
- **Top stats cards:** total URLs created, total clicks across all URLs, active URL count
- **Quick shorten bar:** paste a long URL, optional custom short code, one-click create button — should feel instant
- **Recent URLs table:** title, short code (with copy button), original URL (truncated, full on hover), status badge, click count, health indicator, created date
- **Pagination** at bottom

### 6. URL List
- Full table with all URL fields
- **Filter tabs:** All / Active / Disabled / Expired / Deleted
- **Create button** opens a form/modal:
  - original URL (required), custom code (optional, random 10-char if blank), title, description, expiry date
  - Show validation errors inline (https required, no localhost/private IPs, code already taken)
  - Show health check result after creation (Healthy badge or Unhealthy warning)
- **Edit:** pre-filled form, can change URL/status/expiry
  - Warning: changing the original URL creates a version server-side (not shown to user, just note it happens)
- **Delete flow (two-step):**
  1. First click: soft delete — URL moves to "Deleted" status
  2. Second action: "Permanently Delete" confirm button — hard deletes it
  3. This two-step prevents accidental permanent deletion

### 7. URL Detail & Analytics
- **Info card at top:** all URL fields with copy buttons for short code and full short URL
- **Stats overview:** total clicks, unique visitors, first click, last click
- **Charts:**
  - Daily clicks line/bar chart (requires date range to show data)
  - Top referrers horizontal bar chart
- **Date range picker** (from/to) — charts only populate when range is set
- **Click log table:** timestamp, IP address, browser, device type, referrer — paginated with date filters

### 8. Sessions / Devices
- List of all active sessions: device type, device name, IP address, login time, last active
- Mark current session with a badge
- Revoke button on each non-current session

### 9. Settings
- Change password form (current + new password)
- Show password age (days since last change) and a suggestion banner if it's old
- Display name (read-only for now)

### 10. Admin Panel
- **Blocked Domains:** table of blocked domains with reason, add/delete actions
- **Blocked IP Ranges:** table of CIDR ranges with description, add/delete actions
- **Maintenance:** buttons to purge old sessions and password history (with configurable days input)

---

## Status Visual Language

| State | Badge Color | Used For |
|---|---|---|
| Active / Healthy | Green | URL is live, destination reachable |
| Disabled / Revoked | Gray | URL turned off, session revoked |
| Expired | Yellow/Amber | URL past expiry, session expired |
| Deleted / Unhealthy | Red | URL soft-deleted, destination unreachable |

---

## Auth State Management

- Store `accessToken` and `refreshToken` after login/register
- Attach `Authorization: Bearer <accessToken>` to all protected requests
- On 401 response: auto-attempt token refresh with the stored refresh token
  - If refresh succeeds: retry the original request
  - If refresh fails: clear tokens, redirect to login
- Tokens expire: access in 15 minutes, refresh in 7 days

---

## Error Handling UX

| Scenario | UI Behavior |
|---|---|
| 400 validation error | Show `message` inline under the relevant field |
| 401 unauthorized | Auto-refresh token; if fails, redirect to login |
| 409 device limit | Show session picker modal (see Login flow above) |
| 409 code taken | Show "This code is already in use" under custom code field |
| Network error | Toast notification: "Something went wrong. Please try again." |
| 415 wrong content-type | Internal — handled by always sending JSON |

---

## Key UX Details

- **Copy to clipboard:** every short URL and short code gets a copy icon, show "Copied!" tooltip on click
- **Monospace font** for URLs, short codes, and user IDs
- **Truncated long URLs** with full URL on hover/tooltip
- **Skeleton loaders** for tables and stat cards during loading
- **Disable submit buttons** with spinner during API calls
- **Responsive:** sidebar nav on desktop, hamburger menu on mobile
- **Analytics charts** must resize responsively

---

## Notable Behaviors to Design Around

1. **URL validation is strict** — only `https://` URLs allowed, no localhost, no private/internal domains. Show clear error messages when validation fails.
2. **Health check on create/update** — after creating a URL, the server checks if the destination is reachable. Show the result (Healthy/Unhealthy) in the UI.
3. **Two-step delete** — soft delete first, then permanent delete. Design a clear flow so users understand the difference.
4. **Analytics charts need a date range** — they won't show daily breakdown data without `from` and `to`. Default to last 30 days.
5. **Click log pagination** is separate from the main pagination — it has its own page/total inside the data payload.
6. **Password max 8 chars** — unusual but intentional. The strength indicator should reflect this constraint.
7. **No CORS configured** — frontend dev server will need a proxy to the backend during development.

---

## Environment

```
API_BASE_URL=http://localhost:8080/api/v1
```
