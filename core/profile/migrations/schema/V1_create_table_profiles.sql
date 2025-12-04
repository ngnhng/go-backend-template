-- Copyright 2025 Nguyen Nhat Nguyen
--
-- Licensed under the Apache License, Version 2.0 (the "License");
-- you may not use this file except in compliance with the License.
-- You may obtain a copy of the License at
--
--     http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing, software
-- distributed under the License is distributed on an "AS IS" BASIS,
-- WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
-- See the License for the specific language governing permissions and
-- limitations under the License.

CREATE EXTENSION citext;

CREATE TABLE profiles (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    version_number BIGINT NOT NULL,

    username TEXT,
    email CITEXT UNIQUE NOT NULL,
    age INTEGER,

    created_at TIMESTAMPTZ DEFAULT current_timestamp,
    updated_at TIMESTAMPTZ, -- application-managed
    deleted_at TIMESTAMPTZ,

    CONSTRAINT chk_valid_email CHECK (email ~* '^[^\s@]+@[^\s@]+\.[^\s@]+$'),
    CONSTRAINT chk_valid_age CHECK (age >= 1 AND age <= 150)
);

COMMENT ON COLUMN profiles.version_number IS 'Optimistic version control';
