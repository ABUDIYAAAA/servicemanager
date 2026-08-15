ALTER TABLE deployments
    DROP COLUMN IF EXISTS container_name,
    DROP COLUMN IF EXISTS host_port;
