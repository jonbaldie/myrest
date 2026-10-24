-- Minimal Postgres copy of the myrest fixture shapes used in this pass.
CREATE SCHEMA myrest_fixture;
SET search_path = myrest_fixture;
CREATE TABLE items (id bigserial PRIMARY KEY, name text NOT NULL);
INSERT INTO items (name) VALUES ('alpha'), ('beta');
CREATE TABLE orders (id bigserial PRIMARY KEY, item_id bigint NOT NULL REFERENCES items(id));
INSERT INTO orders (item_id) VALUES (1), (1), (2);
CREATE TABLE colors (id bigserial PRIMARY KEY, name text NOT NULL, tone text NOT NULL DEFAULT 'plain');
CREATE TABLE notes (id bigserial PRIMARY KEY, title text, body text);
CREATE ROLE anon NOLOGIN;
GRANT USAGE ON SCHEMA myrest_fixture TO anon;
GRANT ALL ON ALL TABLES IN SCHEMA myrest_fixture TO anon;
GRANT ALL ON ALL SEQUENCES IN SCHEMA myrest_fixture TO anon;
