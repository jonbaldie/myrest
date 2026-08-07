-- Fixture databases, an authenticator login, and an anonymous database role.
CREATE DATABASE IF NOT EXISTS myrest_fixture;
CREATE DATABASE IF NOT EXISTS myrest_hidden;

USE myrest_fixture;

CREATE TABLE items (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name VARCHAR(255) NOT NULL,
  name_len INT GENERATED ALWAYS AS (CHAR_LENGTH(name)) VIRTUAL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
COMMENT='stock rows';

INSERT INTO items (name) VALUES ('alpha'), ('beta');

CREATE VIEW items_view AS SELECT id, name FROM items;

CREATE TABLE orders (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  item_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (id),
  CONSTRAINT orders_item FOREIGN KEY (item_id) REFERENCES items (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO orders (item_id) VALUES (1);

CREATE FUNCTION item_count() RETURNS BIGINT
  DETERMINISTIC
  READS SQL DATA
  COMMENT 'how many items'
  RETURN (SELECT COUNT(*) FROM items);

CREATE FUNCTION add_them(a BIGINT, b BIGINT) RETURNS BIGINT
  DETERMINISTIC
  NO SQL
  COMMENT 'sum of two numbers'
  RETURN a + b;

-- No EXECUTE for the anonymous role: proves routine exposure needs the grant.
CREATE FUNCTION secret_count() RETURNS BIGINT
  DETERMINISTIC
  NO SQL
  RETURN 99;

CREATE PROCEDURE ping()
BEGIN
  DO 0;
END;

CREATE PROCEDURE echo_name(IN src VARCHAR(255), OUT dst VARCHAR(255))
BEGIN
  SET dst = src;
END;

-- In a configured database, but the anonymous database role gets no SELECT.
CREATE TABLE secrets (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  payload VARCHAR(255) NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO secrets (payload) VALUES ('top-secret');

USE myrest_hidden;

-- Outside the db-schemas list of the anonymous read tests.
CREATE TABLE outside_items (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name VARCHAR(255) NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO outside_items (name) VALUES ('hidden');

-- The authenticator logs in with no privileges of its own: every privilege
-- comes from the database role that myrest activates for the request.
CREATE ROLE IF NOT EXISTS 'myrest_anon';
-- MySQL takes a dash in a role name, so myrest must take one too.
CREATE ROLE IF NOT EXISTS 'web-anon';
CREATE USER IF NOT EXISTS 'authenticator'@'%' IDENTIFIED BY 'secret';
GRANT 'myrest_anon' TO 'authenticator'@'%';
GRANT 'web-anon' TO 'authenticator'@'%';
SET DEFAULT ROLE NONE TO 'authenticator'@'%';

GRANT SELECT ON myrest_fixture.items TO 'myrest_anon';
GRANT INSERT ON myrest_fixture.items TO 'myrest_anon';
GRANT INSERT ON myrest_fixture.orders TO 'myrest_anon';
GRANT SELECT ON myrest_fixture.items_view TO 'myrest_anon';
GRANT EXECUTE ON FUNCTION myrest_fixture.item_count TO 'myrest_anon';
GRANT EXECUTE ON FUNCTION myrest_fixture.add_them TO 'myrest_anon';
GRANT EXECUTE ON PROCEDURE myrest_fixture.ping TO 'myrest_anon';
GRANT EXECUTE ON PROCEDURE myrest_fixture.echo_name TO 'myrest_anon';
GRANT SHOW_ROUTINE ON *.* TO 'authenticator'@'%';
GRANT SELECT ON myrest_hidden.outside_items TO 'myrest_anon';
GRANT SELECT ON myrest_fixture.items TO 'web-anon';

-- A JWT role with SELECT on secrets, so auth-001 can prove grant switching.
CREATE ROLE IF NOT EXISTS 'myrest_user';
GRANT 'myrest_user' TO 'authenticator'@'%';
GRANT SELECT ON myrest_fixture.items TO 'myrest_user';
GRANT SELECT ON myrest_fixture.secrets TO 'myrest_user';
