-- 1. Create the custom ENUM type FIRST
-- Use a DO block to safely handle "IF NOT EXISTS" in PostgreSQL
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'user_role') THEN
        CREATE TYPE user_role AS ENUM ('admin', 'user');
    END IF;
END $$;

-- 2. Now create the tables that reference user_role
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(100) NOT NULL UNIQUE,
    user_role user_role NOT NULL DEFAULT 'user',
    password_hash VARCHAR(255) NOT NULL,
    password_reset_token VARCHAR(255) UNIQUE,
    password_reset_token_expiration TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS invites (
    id SERIAL PRIMARY KEY,
    email VARCHAR(100) NOT NULL UNIQUE,
    token VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);