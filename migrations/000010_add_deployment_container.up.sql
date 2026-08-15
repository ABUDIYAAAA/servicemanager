ALTER TABLE deployments
    ADD COLUMN container_name VARCHAR(128),
    ADD COLUMN host_port INT;
