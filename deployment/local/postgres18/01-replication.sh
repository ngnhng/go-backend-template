#!/bin/sh
set -e

echo "Configuring replication user and pg_hba.conf for replica..."

# In 18+ PGDATA defaults to /var/lib/postgresql/18/docker
echo "host replication repl_user 0.0.0.0/0 scram-sha-256" >> "$PGDATA/pg_hba.conf"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" <<-EOSQL
  DO \$\$
  BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'repl_user') THEN
      CREATE ROLE repl_user WITH REPLICATION LOGIN PASSWORD 'repl_password';
    END IF;
  END
  \$\$;
EOSQL
