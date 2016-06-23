SET statement_timeout = 0;
SET lock_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SET check_function_bodies = false;
SET client_min_messages = warning;

--
-- Name: plpgsql; Type: EXTENSION; Schema: -; Owner: 
--

CREATE EXTENSION IF NOT EXISTS plpgsql WITH SCHEMA pg_catalog;


--
-- Name: EXTENSION plpgsql; Type: COMMENT; Schema: -; Owner: 
--

COMMENT ON EXTENSION plpgsql IS 'PL/pgSQL procedural language';


--
-- Name: uuid-ossp; Type: EXTENSION; Schema: -; Owner: 
--

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;


--
-- Name: EXTENSION "uuid-ossp"; Type: COMMENT; Schema: -; Owner: 
--

COMMENT ON EXTENSION "uuid-ossp" IS 'generate universally unique identifiers (UUIDs)';


SET search_path = public, pg_catalog;

CREATE TYPE relationship_type AS ENUM (
    'equal',
    'notEqual',
    'empty',
    'notEmpty',
    'contain',
    'notContain',
    'regExp',
    'greaterThan',
    'lessThan'
);


CREATE FUNCTION update_time() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
      BEGIN
      NEW.updated_at := CURRENT_TIMESTAMP;
      RETURN NEW;
      END;
      $$;


SET default_tablespace = '';

SET default_with_oids = false;

CREATE TABLE assertions (
    check_id character varying(255) DEFAULT ''::character varying NOT NULL,
    customer_id uuid NOT NULL,
    key character varying(255) DEFAULT ''::character varying NOT NULL,
    relationship relationship_type NOT NULL,
    value character varying(255) DEFAULT ''::character varying,
    operand character varying(255) DEFAULT ''::character varying
);

CREATE TABLE check_state_memos (
    check_id character varying(255) NOT NULL,
    customer_id uuid NOT NULL,
    bastion_id uuid NOT NULL,
    failing_count integer NOT NULL,
    response_count integer NOT NULL,
    last_updated timestamp with time zone NOT NULL
);

CREATE TABLE check_states (
    check_id character varying(255) NOT NULL,
    customer_id uuid NOT NULL,
    state_id integer NOT NULL,
    state_name character varying(255) NOT NULL,
    time_entered timestamp with time zone NOT NULL,
    last_updated timestamp with time zone NOT NULL,
    failing_count integer NOT NULL,
    response_count integer NOT NULL
);

CREATE TABLE checks (
    id character varying(255) NOT NULL,
    "interval" integer,
    target_id character varying(255) NOT NULL,
    check_spec jsonb,
    customer_id uuid NOT NULL,
    name character varying(255) NOT NULL,
    target_name character varying(255),
    target_type character varying(255) NOT NULL,
    execution_group_id uuid NOT NULL,
    min_failing_count integer DEFAULT 1 NOT NULL,
    min_failing_time integer DEFAULT 90 NOT NULL
);

CREATE TABLE credentials (
    id integer NOT NULL,
    provider character varying(20),
    access_key_id character varying(60),
    secret_key character varying(60),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    customer_id uuid NOT NULL
);

CREATE SEQUENCE credentials_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE credentials_id_seq OWNED BY credentials.id;

CREATE TABLE databasechangelog (
    id character varying(255) NOT NULL,
    author character varying(255) NOT NULL,
    filename character varying(255) NOT NULL,
    dateexecuted timestamp with time zone NOT NULL,
    orderexecuted integer NOT NULL,
    exectype character varying(10) NOT NULL,
    md5sum character varying(35),
    description character varying(255),
    comments character varying(255),
    tag character varying(255),
    liquibase character varying(20)
);

CREATE TABLE databasechangeloglock (
    id integer NOT NULL,
    locked boolean NOT NULL,
    lockgranted timestamp with time zone,
    lockedby character varying(255)
);

ALTER TABLE ONLY credentials ALTER COLUMN id SET DEFAULT nextval('credentials_id_seq'::regclass);

ALTER TABLE ONLY check_states
    ADD CONSTRAINT pk_check_states PRIMARY KEY (check_id);

ALTER TABLE ONLY checks
    ADD CONSTRAINT pk_checks PRIMARY KEY (id);

ALTER TABLE ONLY credentials
    ADD CONSTRAINT pk_credentials PRIMARY KEY (id);

ALTER TABLE ONLY databasechangeloglock
    ADD CONSTRAINT pk_databasechangeloglock PRIMARY KEY (id);

CREATE INDEX bastion_id_idx ON check_state_memos USING btree (bastion_id);

CREATE INDEX cust_execution_group_id_idx ON checks USING btree (customer_id, execution_group_id);

CREATE INDEX execution_group_id_idx ON checks USING btree (execution_group_id);

CREATE INDEX idx_assertions_check_id_and_customer_id ON assertions USING btree (check_id, customer_id);

CREATE INDEX idx_checks_customer_id ON checks USING btree (customer_id);

CREATE INDEX idx_credentials_customer_id ON credentials USING btree (customer_id);

CREATE UNIQUE INDEX idx_memos_bastion_id_check_id ON check_state_memos USING btree (check_id, bastion_id);

CREATE INDEX idx_memos_check_id ON check_state_memos USING btree (check_id);

CREATE TRIGGER update_credentials BEFORE UPDATE ON credentials FOR EACH ROW EXECUTE PROCEDURE update_time();

REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT ALL ON SCHEMA public TO PUBLIC;


--
-- PostgreSQL database dump complete
--

