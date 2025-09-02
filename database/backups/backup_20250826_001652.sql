--
-- PostgreSQL database dump
--

\restrict LgMBES1oZ4XAYlWADRU2Lx7jOJ7mg6HRUfqoM5OeHHgjqgfY3lK9iQnSdlhLtRF

-- Dumped from database version 15.14 (Debian 15.14-1.pgdg13+1)
-- Dumped by pg_dump version 15.14 (Debian 15.14-1.pgdg13+1)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: uuid-ossp; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;


--
-- Name: EXTENSION "uuid-ossp"; Type: COMMENT; Schema: -; Owner: 
--

COMMENT ON EXTENSION "uuid-ossp" IS 'generate universally unique identifiers (UUIDs)';


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: flyway_schema_history; Type: TABLE; Schema: public; Owner: ezhealth_user
--

CREATE TABLE public.flyway_schema_history (
    installed_rank integer NOT NULL,
    version character varying(50),
    description character varying(200) NOT NULL,
    type character varying(20) NOT NULL,
    script character varying(1000) NOT NULL,
    checksum integer,
    installed_by character varying(100) NOT NULL,
    installed_on timestamp without time zone DEFAULT now() NOT NULL,
    execution_time integer NOT NULL,
    success boolean NOT NULL
);


ALTER TABLE public.flyway_schema_history OWNER TO ezhealth_user;

--
-- Name: users; Type: TABLE; Schema: public; Owner: ezhealth_user
--

CREATE TABLE public.users (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    email character varying NOT NULL,
    password_hash character varying NOT NULL,
    first_name character varying NOT NULL,
    last_name character varying NOT NULL,
    role character varying NOT NULL,
    status character varying NOT NULL,
    email_verified boolean NOT NULL,
    email_verification_token character varying,
    password_reset_token character varying,
    password_reset_expires timestamp with time zone,
    last_login_at timestamp with time zone,
    last_login_ip inet,
    login_attempts integer NOT NULL,
    locked_until timestamp with time zone,
    data_consent_given boolean NOT NULL,
    data_consent_date timestamp with time zone,
    data_retention_until timestamp with time zone,
    data_anonymized boolean NOT NULL,
    gdpr_delete_requested boolean NOT NULL,
    gdpr_delete_requested_at timestamp with time zone,
    phone character varying,
    organization character varying,
    job_title character varying,
    department character varying,
    timezone character varying NOT NULL,
    locale character varying NOT NULL,
    preferences jsonb,
    created_by uuid,
    updated_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.users OWNER TO ezhealth_user;

--
-- Data for Name: flyway_schema_history; Type: TABLE DATA; Schema: public; Owner: ezhealth_user
--

COPY public.flyway_schema_history (installed_rank, version, description, type, script, checksum, installed_by, installed_on, execution_time, success) FROM stdin;
1	1	<< Flyway Baseline >>	BASELINE	<< Flyway Baseline >>	\N	ezhealth_user	2025-08-25 18:45:14.489567	0	t
\.


--
-- Data for Name: users; Type: TABLE DATA; Schema: public; Owner: ezhealth_user
--

COPY public.users (id, email, password_hash, first_name, last_name, role, status, email_verified, email_verification_token, password_reset_token, password_reset_expires, last_login_at, last_login_ip, login_attempts, locked_until, data_consent_given, data_consent_date, data_retention_until, data_anonymized, gdpr_delete_requested, gdpr_delete_requested_at, phone, organization, job_title, department, timezone, locale, preferences, created_by, updated_by, created_at, updated_at) FROM stdin;
\.


--
-- Name: flyway_schema_history flyway_schema_history_pk; Type: CONSTRAINT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.flyway_schema_history
    ADD CONSTRAINT flyway_schema_history_pk PRIMARY KEY (installed_rank);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: flyway_schema_history_s_idx; Type: INDEX; Schema: public; Owner: ezhealth_user
--

CREATE INDEX flyway_schema_history_s_idx ON public.flyway_schema_history USING btree (success);


--
-- PostgreSQL database dump complete
--

\unrestrict LgMBES1oZ4XAYlWADRU2Lx7jOJ7mg6HRUfqoM5OeHHgjqgfY3lK9iQnSdlhLtRF

