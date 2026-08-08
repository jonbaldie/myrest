-- Fixture databases, an authenticator login, and an anonymous database role.
-- Drop first so a reused local mysqld (MYREST_MYSQL_HARNESS_PORT) starts clean.
DROP DATABASE IF EXISTS myrest_fixture;
DROP DATABASE IF EXISTS myrest_hidden;
CREATE DATABASE myrest_fixture;
CREATE DATABASE myrest_hidden;

USE myrest_fixture;

CREATE TABLE items (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name VARCHAR(255) NOT NULL,
  name_len INT GENERATED ALWAYS AS (CHAR_LENGTH(name)) VIRTUAL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
COMMENT='stock rows';

INSERT INTO items (name) VALUES ('alpha'), ('beta');

CREATE TABLE profiles (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  meta JSON NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
COMMENT='json path fixture rows';

INSERT INTO profiles (meta) VALUES
  (CAST('{"blood_type":"A-","tag":"Alpha","phones":[{"number":"917-929-5745"}]}' AS JSON)),
  (CAST('{"blood_type":"O+","tag":"Beta","phones":[{"number":"512-446-4988"}]}' AS JSON));

CREATE VIEW items_view AS SELECT id, name FROM items;

CREATE TABLE orders (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  item_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (id),
  CONSTRAINT orders_item FOREIGN KEY (item_id) REFERENCES items (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO orders (item_id) VALUES (1), (1), (2);

CREATE TABLE tags (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name VARCHAR(255) NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO tags (name) VALUES ('hot'), ('cold');

-- Join table for many-to-many: both FKs are part of the primary key.
CREATE TABLE item_tags (
  item_id BIGINT UNSIGNED NOT NULL,
  tag_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (item_id, tag_id),
  CONSTRAINT item_tags_item FOREIGN KEY (item_id) REFERENCES items (id),
  CONSTRAINT item_tags_tag FOREIGN KEY (tag_id) REFERENCES tags (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO item_tags (item_id, tag_id) VALUES (1, 1), (1, 2), (2, 1);

CREATE TABLE addresses (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  label VARCHAR(255) NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO addresses (label) VALUES ('from-here'), ('to-there');

-- Two foreign keys to the same table: embed needs disambiguation.
CREATE TABLE deliveries (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  from_address_id BIGINT UNSIGNED NOT NULL,
  to_address_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (id),
  CONSTRAINT deliveries_from FOREIGN KEY (from_address_id) REFERENCES addresses (id),
  CONSTRAINT deliveries_to FOREIGN KEY (to_address_id) REFERENCES addresses (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO deliveries (from_address_id, to_address_id) VALUES (1, 2);

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

-- MODIFIES SQL DATA: not read-safe for GET /rpc (rpc-004).
-- A procedure is used because MySQL with binary logging refuses a
-- MODIFIES SQL DATA function unless log_bin_trust_function_creators is on.
CREATE PROCEDURE write_marker()
  MODIFIES SQL DATA
BEGIN
  INSERT INTO addresses (label) VALUES ('rpc-write');
END;

CREATE PROCEDURE ping()
BEGIN
  DO 0;
END;

CREATE PROCEDURE echo_name(IN src VARCHAR(255), OUT dst VARCHAR(255))
BEGIN
  SET dst = src;
END;

CREATE PROCEDURE bump_label(INOUT label VARCHAR(255))
BEGIN
  SET label = CONCAT(label, '!');
END;

-- Row-set RPC result: one SELECT result set is a tabular body (rpc-005).
CREATE PROCEDURE list_items()
  READS SQL DATA
BEGIN
  SELECT id, name FROM items ORDER BY id;
END;

-- db-pre-request fixtures: a marker log and zero-argument procedures.
CREATE TABLE pre_request_log (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  note VARCHAR(255) NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE PROCEDURE before_request()
  MODIFIES SQL DATA
BEGIN
  INSERT INTO pre_request_log (note) VALUES ('hook');
END;

CREATE PROCEDURE before_request_fail()
BEGIN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'pre-request refused';
END;

-- EXECUTE is never granted to myrest_anon: proves the hook needs the grant.
CREATE PROCEDURE before_request_denied()
  MODIFIES SQL DATA
BEGIN
  INSERT INTO pre_request_log (note) VALUES ('denied');
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
GRANT SELECT ON myrest_fixture.profiles TO 'myrest_anon';
GRANT SELECT ON myrest_fixture.orders TO 'myrest_anon';
GRANT SELECT ON myrest_fixture.tags TO 'myrest_anon';
GRANT SELECT ON myrest_fixture.item_tags TO 'myrest_anon';
GRANT SELECT ON myrest_fixture.addresses TO 'myrest_anon';
GRANT SELECT ON myrest_fixture.deliveries TO 'myrest_anon';
GRANT INSERT ON myrest_fixture.items TO 'myrest_anon';
GRANT INSERT ON myrest_fixture.orders TO 'myrest_anon';
GRANT UPDATE ON myrest_fixture.items TO 'myrest_anon';
GRANT DELETE ON myrest_fixture.items TO 'myrest_anon';
GRANT SELECT ON myrest_fixture.items_view TO 'myrest_anon';
GRANT EXECUTE ON FUNCTION myrest_fixture.item_count TO 'myrest_anon';
GRANT EXECUTE ON FUNCTION myrest_fixture.add_them TO 'myrest_anon';
GRANT EXECUTE ON PROCEDURE myrest_fixture.write_marker TO 'myrest_anon';
GRANT EXECUTE ON PROCEDURE myrest_fixture.ping TO 'myrest_anon';
GRANT EXECUTE ON PROCEDURE myrest_fixture.echo_name TO 'myrest_anon';
GRANT EXECUTE ON PROCEDURE myrest_fixture.bump_label TO 'myrest_anon';
GRANT EXECUTE ON PROCEDURE myrest_fixture.list_items TO 'myrest_anon';
GRANT INSERT ON myrest_fixture.addresses TO 'myrest_anon';
GRANT SELECT, INSERT, DELETE ON myrest_fixture.pre_request_log TO 'myrest_anon';
GRANT EXECUTE ON PROCEDURE myrest_fixture.before_request TO 'myrest_anon';
GRANT EXECUTE ON PROCEDURE myrest_fixture.before_request_fail TO 'myrest_anon';
GRANT SHOW_ROUTINE ON *.* TO 'authenticator'@'%';
GRANT SELECT ON myrest_hidden.outside_items TO 'myrest_anon';
GRANT INSERT ON myrest_hidden.outside_items TO 'myrest_anon';
GRANT SELECT ON myrest_fixture.items TO 'web-anon';

-- A JWT role with SELECT on secrets, so auth-001 can prove grant switching.
CREATE ROLE IF NOT EXISTS 'myrest_user';
GRANT 'myrest_user' TO 'authenticator'@'%';
GRANT SELECT ON myrest_fixture.items TO 'myrest_user';
GRANT SELECT ON myrest_fixture.profiles TO 'myrest_user';
GRANT SELECT ON myrest_fixture.orders TO 'myrest_user';
GRANT SELECT ON myrest_fixture.secrets TO 'myrest_user';
