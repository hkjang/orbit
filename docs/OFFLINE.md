# Offline deployment

GitHub Releases contain only the compressed Orbit service image. For a tag
`v1.2.3`, the image and artifact names are exactly:

```text
orbit:v1.2.3
orbit-v1.2.3.tar.gz
```

Transfer that archive and a PostgreSQL image approved by your organization to
the offline network. Then load the service image:

```bash
docker load < orbit-v1.2.3.tar.gz
docker image inspect orbit:v1.2.3
```

Orbit reads exactly four runtime environment variables:

```text
DATABASE_URL
BOOTSTRAP_ADMIN
BOOTSTRAP_ADMIN_PASSWORD
ENCRYPTION_KEY
```

Generate `ENCRYPTION_KEY` once with `openssl rand -base64 32`, back it up in an
offline secrets manager, and never change it directly. Per-user rotation is
performed from **Personalization → Encryption key**. All other service settings
are managed from the administrator screen.

`BOOTSTRAP_ADMIN_PASSWORD` is only used when no administrator exists. Create
named administrators and rotate the bootstrap account password after the first
login. PostgreSQL backup and restoration must preserve the database and the
same `ENCRYPTION_KEY` together.
