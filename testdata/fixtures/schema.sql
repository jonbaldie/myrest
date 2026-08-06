-- Fixture databases, an authenticator login, and an anonymous database role.
CREATE DATABASE IF NOT EXISTS myrest_fixture;
CREATE DATABASE IF NOT EXISTS myrest_hidden;

USE myrest_fixture;

CREATE TABLE items (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name VARCHAR(255) NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO items (name) VALUES ('alpha'), ('beta');

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
CREATE USER IF NOT EXISTS 'authenticator'@'%' IDENTIFIED BY 'secret';
GRANT 'myrest_anon' TO 'authenticator'@'%';
SET DEFAULT ROLE NONE TO 'authenticator'@'%';

GRANT SELECT ON myrest_fixture.items TO 'myrest_anon';
GRANT SELECT ON myrest_hidden.outside_items TO 'myrest_anon';
