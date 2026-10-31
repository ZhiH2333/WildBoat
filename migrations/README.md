# Migrations

Versioned SQL migrations live in this directory and are embedded by `migrations/embed.go`.
The API applies them on startup. Do not mutate applied production migrations in place.
