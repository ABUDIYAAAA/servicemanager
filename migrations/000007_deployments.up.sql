CREATE TABLE IF NOT EXISTS deployments (
    id SERIAL PRIMARY KEY,
    service_id INT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    status VARCHAR(50) NOT NULL DEFAULT 'queued',
    trigger VARCHAR(50) NOT NULL DEFAULT 'manual',
    commit_sha VARCHAR(64),
    build_logs TEXT DEFAULT '' NOT NULL,
    runtime_logs TEXT DEFAULT '' NOT NULL,
    directory_path VARCHAR(512),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    finished_at TIMESTAMP
);

CREATE INDEX idx_deployments_service_id ON deployments(service_id);
