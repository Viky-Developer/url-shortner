# URL Shortener — API Documentation

This document provides complete technical specifications for the URL Shortener REST API.

---

## Table of Contents

- [Overview](#overview)
- [Authentication](#authentication)
- [Standard Response Formats](#standard-response-formats)
- [1. Health & Observability](#1-health--observability)
- [2. Redirection (Public)](#2-redirection-public)
- [3. Authentication & Session Management](#3-authentication--session-management)
- [4. URL Management & Analytics](#4-url-management--analytics)
- [5. Account Management](#5-account-management)
- [6. Admin Operations (Admin Role Only)](#6-admin-operations-admin-role-only)

---

## Overview

- **Base URL**: `http://localhost:8080` (or configured `SERVER_BASE_URL`)
- **API Prefix**: `/api/v1`
- **Default Content-Type**: `application/json`

---

## Authentication

Protected endpoints require a valid JWT Access Token passed in the `Authorization` request header:

```http
Authorization: Bearer <your_jwt_access_token>
```

- **Access Token Lifetime**: 15 minutes (configurable via `ACCESS_TOKEN_EXPIRY`)
- **Refresh Token Lifetime**: 7 days (configurable via `REFRESH_TOKEN_EXPIRY`)
- **Session Tracking**: Multi-device sessions are recorded in Redis and PostgreSQL. Users can view and revoke sessions remotely.

---

## Standard Response Formats

All JSON endpoints return unified response envelopes.

### Success Response
```json
{
  "statusCode": 200,
  "message": "Operation completed successfully",
  "data": [ ... ],
  "pagination": {
    "currentPage": 1,
    "pageSize": 10,
    "totalPages": 5,
    "totalItems": 48
  }
}
```
*(Note: `pagination` is omitted if the endpoint does not support paginated results.)*

### Error Response
```json
{
  "statusCode": 400,
  "message": "Invalid destination URL provided"
}
```

---

## 1. Health & Observability

### `GET /health`
Liveness probe to verify that the HTTP server is responsive.
- **Auth**: Public
- **Response**: `200 OK` (text/plain: `"OK"`)

### `GET /metrics`
Exposes system, runtime, and HTTP request metrics in Prometheus exposition text format.
- **Auth**: Public (scraped by Prometheus server)
- **Response**: `200 OK` (`text/plain; version=0.0.4`)

---

## 2. Redirection (Public)

### `GET /api/v1/{shortCode}`
Resolves a shortened URL code and redirects the client to the original target destination.

- **Auth**: Public
- **Path Parameters**:
  - `shortCode` *(string, required)*: The 10-character random code or custom alias (e.g. `docs`, `gh-repo`).
- **Behavior**:
  - **Cache Hit**: Returns in `< 1ms` directly from Redis (30-minute TTL). Zero database queries executed.
  - **Cache Miss**: Queries PostgreSQL, caches the target URL in Redis with a 30-minute TTL, and returns the redirect.
  - **Click Analytics**: Dispatches an asynchronous `ClickEvent` to RabbitMQ (`url.clicks.direct` exchange $\to$ `url.clicks` queue) to record IP, user-agent, and referrer in the background. If RabbitMQ is disabled or unavailable, falls back gracefully to synchronous transactional recording without failing the redirect.
- **Responses**:
  - `302 Found` / `307 Temporary Redirect`: Header `Location: https://original-destination.com`
  - `404 Not Found`: Short code does not exist.
  - `410 Gone`: URL has expired (`expires_at` is in the past).

---

## 3. Authentication & Session Management

### `POST /api/v1/auth/register`
Creates a new user account.
- **Auth**: Public
- **Request Body**:
  ```json
  {
    "email": "user@example.com",
    "password": "StrongPassword123!"
  }
  ```
- **Response**: `201 Created`

---

### `POST /api/v1/auth/login`
Authenticates user credentials, registers a new device session, and returns access and refresh tokens.
- **Auth**: Public
- **Request Body**:
  ```json
  {
    "email": "user@example.com",
    "password": "StrongPassword123!"
  }
  ```
- **Response**: `200 OK`
  ```json
  {
    "statusCode": 200,
    "message": "Login successful",
    "data": [
      {
        "accessToken": "eyJhbGciOiJIUzI1Ni...",
        "refreshToken": "eyJhbGciOiJIUzI1Ni...",
        "expiresIn": 900,
        "tokenType": "Bearer"
      }
    ]
  }
  ```

---

### `POST /api/v1/auth/refresh`
Exchanges a valid refresh token for a newly rotated access token.
- **Auth**: Bearer Token
- **Request Body**:
  ```json
  {
    "refreshToken": "eyJhbGciOiJIUzI1Ni..."
  }
  ```
- **Response**: `200 OK`

---

### `POST /api/v1/auth/forgot-password`
Initiates a password recovery request for the given email.
- **Auth**: Public
- **Request Body**:
  ```json
  {
    "email": "user@example.com"
  }
  ```
- **Response**: `200 OK`

---

### `POST /api/v1/auth/change-password`
Updates the account password. Enforces password history to prevent immediate reuse of recent passwords.
- **Auth**: Bearer Token
- **Request Body**:
  ```json
  {
    "oldPassword": "CurrentPassword123!",
    "newPassword": "NewStrongPassword456!"
  }
  ```
- **Response**: `200 OK`

---

### `POST /api/v1/auth/logout`
Invalidates the current session and removes cached tokens from Redis.
- **Auth**: Bearer Token
- **Response**: `200 OK`

---

### `GET /api/v1/auth/sessions`
Returns all active device sessions associated with the authenticated account.
- **Auth**: Bearer Token
- **Response**: `200 OK`
  ```json
  {
    "statusCode": 200,
    "message": "Active sessions retrieved",
    "data": [
      {
        "id": 14,
        "ipAddress": "192.168.1.10",
        "userAgent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
        "deviceType": "desktop",
        "isCurrent": true,
        "createdAt": "2026-09-22T06:00:00Z"
      }
    ]
  }
  ```

---

### `DELETE /api/v1/auth/sessions/{id}`
Revokes a specific remote device session by its ID.
- **Auth**: Bearer Token
- **Response**: `200 OK`

---

### `POST /api/v1/auth/sessions/revoke-others`
Revokes all active sessions across other devices, keeping only the current active session alive.
- **Auth**: Bearer Token
- **Response**: `200 OK`

---

### `POST /api/v1/auth/sessions/revoke-all`
Revokes all active sessions across all devices (forces full logout).
- **Auth**: Bearer Token
- **Response**: `200 OK`

---

## 4. URL Management & Analytics

### `POST /api/v1/shorten`
Shortens a long destination URL.
- **Auth**: Bearer Token
- **Validation**: Performs destination healthcheck, domain blacklist check, and CIDR range verification before creating.
- **Request Body**:
  ```json
  {
    "originalUrl": "https://example.com/very/long/path/to/resource",
    "customCode": "my-promo",
    "expiresAt": "2026-12-31T23:59:59Z"
  }
  ```
  *(Note: `customCode` and `expiresAt` are optional).*
- **Response**: `201 Created`
  ```json
  {
    "statusCode": 201,
    "message": "URL shortened successfully",
    "data": [
      {
        "id": "enc_user_id_42",
        "shortCode": "my-promo",
        "originalUrl": "https://example.com/very/long/path/to/resource",
        "shortUrl": "http://localhost:8080/api/v1/my-promo",
        "createdAt": "2026-09-22T07:00:00Z",
        "expiresAt": "2026-12-31T23:59:59Z"
      }
    ]
  }
  ```

---

### `GET /api/v1/urls`
Returns a paginated list of shortened links owned by the authenticated user.
- **Auth**: Bearer Token
- **Query Parameters**:
  - `page` *(int, default: 1)*
  - `limit` *(int, default: 10, max: 100)*
  - `search` *(string, optional)*: Filter by destination URL or short code
- **Response**: `200 OK` (with `pagination` metadata)

---

### `GET /api/v1/urls/{id}`
Retrieves detailed information for a single shortened URL by its ID.
- **Auth**: Bearer Token
- **Response**: `200 OK`

---

### `PATCH /api/v1/urls/{id}`
Updates the destination URL or expiration date. Automatically invalidates any existing cache in Redis.
- **Auth**: Bearer Token
- **Request Body**:
  ```json
  {
    "originalUrl": "https://new-destination.com/path",
    "expiresAt": "2027-01-01T00:00:00Z"
  }
  ```
- **Response**: `200 OK`

---

### `DELETE /api/v1/urls/{id}`
Initiates soft-deletion of a shortened URL.
- **Auth**: Bearer Token
- **Response**: `200 OK`

---

### `DELETE /api/v1/urls/{id}/approve`
Confirms and approves hard-deletion of a soft-deleted URL.
- **Auth**: Bearer Token
- **Response**: `200 OK`

---

### `GET /api/v1/urls/status-counts`
Returns URL counts grouped by lifecycle status (`active`, `expired`, `deleted`).
- **Auth**: Bearer Token
- **Response**: `200 OK`

---

### `GET /api/v1/urls/clicks`
Returns all individual click logs across all URLs owned by the user.
- **Auth**: Bearer Token
- **Query Parameters**: `page`, `limit`
- **Response**: `200 OK`

---

### `GET /api/v1/urls/{id}/clicks`
Returns individual click logs for a specific shortened URL.
- **Auth**: Bearer Token
- **Query Parameters**: `page`, `limit`
- **Response**: `200 OK`

---

### `GET /api/v1/urls/analytics`
Returns aggregated analytics across all user URLs, including top referrers, device breakdown (Desktop / Mobile / Tablet), and browser distributions.
- **Auth**: Bearer Token
- **Response**: `200 OK`

---

### `GET /api/v1/urls/{id}/analytics`
Returns detailed analytics for a single URL.
- **Auth**: Bearer Token
- **Response**: `200 OK`

---

### `GET /api/v1/urls/clicks/counts`
Returns cumulative time-series click aggregations for graphing and trend analysis.
- **Auth**: Bearer Token
- **Response**: `200 OK`

---

## 5. Account Management

### `DELETE /api/v1/account`
Requests self-service account deletion. Initiates a **30-day grace period** during which the user may cancel the request.
- **Auth**: Bearer Token
- **Response**: `200 OK`

---

### `POST /api/v1/account/cancel-deletion`
Cancels a pending account deletion and restores active account status.
- **Auth**: Bearer Token
- **Response**: `200 OK`

---

## 6. Admin Operations (Admin Role Only)

All endpoints in this section require authentication with a user possessing the `ADMIN` role.

### `GET /api/v1/admin/blocked-domains`
Lists all blacklisted target domains.
- **Response**: `200 OK`

### `POST /api/v1/admin/blocked-domains`
Adds a domain to the blacklist.
- **Request Body**:
  ```json
  {
    "domain": "phishing-target.com"
  }
  ```
- **Response**: `201 Created`

### `DELETE /api/v1/admin/blocked-domains/{id}`
Removes a domain from the blacklist.
- **Response**: `200 OK`

### `GET /api/v1/admin/blocked-ip-ranges`
Lists all blacklisted CIDR IP ranges.
- **Response**: `200 OK`

### `POST /api/v1/admin/blocked-ip-ranges`
Adds a CIDR block to the IP blacklist.
- **Request Body**:
  ```json
  {
    "cidr": "203.0.113.0/24",
    "reason": "Abusive bot traffic"
  }
  ```
- **Response**: `201 Created`

### `DELETE /api/v1/admin/blocked-ip-ranges/{id}`
Removes an IP CIDR block from the blacklist.
- **Response**: `200 OK`

### `DELETE /api/v1/admin/users/{id}/soft-delete`
Soft-deletes a user account and deactivates their shortened links.
- **Response**: `200 OK`

### `DELETE /api/v1/admin/users/{id}/hard-delete`
Permanently purges a user record and cascades deletion through their URLs and click logs.
- **Response**: `200 OK`

### `POST /api/v1/admin/maintenance/purge-sessions`
Maintenance job to purge expired and revoked tokens from the database.
- **Response**: `200 OK`

### `POST /api/v1/admin/maintenance/purge-password-history`
Maintenance job to clean up obsolete password history entries.
- **Response**: `200 OK`

