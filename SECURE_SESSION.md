# Secure Session Management (Backend)

This document describes the secure refresh token and silent session renewal implementation for the Go backend of RapidLink.

---

## Overview
- Implements secure refresh tokens for session renewal.
- Refresh tokens are generated on login/register, stored as a hash in the database, and sent to the client as HttpOnly, Secure cookies.
- Access tokens (JWT) are short-lived (1 hour) and sent in the response body.
- The `/auth/refresh` endpoint rotates the refresh token and issues a new access token.

## Key Features
- **HttpOnly, Secure Cookies:** Refresh tokens are never accessible to JavaScript, mitigating XSS risks.
- **Token Rotation:** Each refresh operation issues a new refresh token and invalidates the old one.
- **Revocation:** Refresh tokens can be revoked on logout or password change by clearing the DB field and cookie.
- **Short-lived Access Tokens:** JWTs are valid for 1 hour, reducing risk if leaked.

## Flow
1. **Login/Register:**
   - User authenticates.
   - Backend issues:
     - Access token (JWT) in response body.
     - Refresh token as HttpOnly, Secure cookie (7-day expiry).
     - Refresh token hash and expiry stored in user DB record.
2. **Session Renewal:**
   - Client calls `/auth/refresh` (no payload, cookie sent automatically).
   - Backend validates refresh token:
     - Checks hash and expiry in DB.
     - If valid, issues new access token and refresh token (rotated).
     - Sets new refresh token cookie and updates DB.
   - If invalid/expired, clears cookie and DB, returns 401.
3. **Logout:**
   - Backend clears refresh token in DB and cookie.

## Endpoints
- `POST /auth/login` — Issues access and refresh tokens.
- `POST /auth/register` — Issues access and refresh tokens.
- `POST /auth/refresh` — Rotates refresh token, issues new access token.
- `POST /auth/logout` (optional) — Revokes refresh token.

## Security Notes
- Refresh tokens are never exposed to JavaScript.
- Rotation and revocation are enforced server-side.
- Access tokens are short-lived and only valid with a valid refresh token.
- All sensitive operations use HTTPS and secure cookies.

---

This approach provides robust, secure, and user-friendly session management for modern web applications.
