# Auth Foundation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the React/Ant Design frontend, Go backend, local Docker environment, and a minimal registration/login/session flow without registration review, departments, roles, or business permissions.

**Architecture:** A modular Go HTTP service stores accounts and refresh sessions in PostgreSQL and exposes REST endpoints. A React SPA implements the supplied login/register designs and communicates through Nginx in Docker Compose. New registrations are immediately active only for this scoped milestone; the account status column remains available for the later review workflow.

**Tech Stack:** React, TypeScript, Vite, Ant Design, TanStack Query, Go, Gin, pgx, PostgreSQL, JWT, bcrypt, Docker Compose, Nginx.

---

## Scope guard

Included: project scaffolding, database migrations, health checks, register, login, refresh, logout, current-user endpoint, frontend auth pages, protected placeholder page, local deployment and tests.

Excluded: registration review, departments, roles, RBAC, data scopes, password reset, remember-me persistence beyond refresh cookie, business modules, MinIO and report generation.

## Tasks

1. Create Compose, environment examples, backend/frontend Dockerfiles and Nginx routing.
2. Write failing Go tests for account registration, duplicate student number, login, invalid credentials, refresh, logout and current-user access.
3. Implement migrations, repositories, password hashing, JWT access tokens, rotating refresh sessions and auth handlers.
4. Write failing frontend tests for registration/login validation and authenticated routing.
5. Implement the supplied login/register visual design, API client, session bootstrap and placeholder home page.
6. Run backend tests, frontend tests/build, Compose config validation and container integration smoke tests.
7. Update README with local setup, endpoints, test commands and explicit deferred scope.

