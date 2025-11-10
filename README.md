# Conect Work — Telegram Freelance Bot

This repository contains a Telegram bot written in Go designed for match-making between clients and freelancers.

## Features
- Roles: Executor (profile) and Client (orders/requests)
- Executors: create profile (150-200 chars), optional photo, edit via /my_profile
- Clients: create exactly one active order; choose category (design/programming/content)
- Orders posted to real Telegram groups with buttons: Connect and Complain
- Complaints counted; >=10 complaints → order deleted, author notified
- Storage: Postgres (recommended) or JSON file fallback for testing
- Webhook-based (recommended for Render.com)

## How to use
1. Set environment variables:
   - `TELEGRAM_BOT_TOKEN` (required)
   - `WEBHOOK_SECRET` (required; random string)
   - `TELEGRAM_WEBHOOK_URL` (optional; your public URL)
   - `DATABASE_URL` (optional; if empty JSON file storage used)
   - `DESIGN_GROUP_ID`, `PROGRAMMING_GROUP_ID`, `CONTENT_GROUP_ID` (chat IDs, e.g. -100123456...)
   - `PORT` (optional)

2. Build and run:
```bash
go build
./conectwork
```

3. On Render.com:
- Create a Web Service (Docker or native Go)
- Provide env vars above
- Ensure bot is added to groups and has permission to send messages

## Note
JSON fallback is for quick tests only. For stability under load (1k-10k users) use Postgres and Render managed Postgres.

