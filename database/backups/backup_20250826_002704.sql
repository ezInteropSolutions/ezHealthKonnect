--
-- PostgreSQL database dump
--

\restrict nyj1NVrYxWKUJ1evKmWVfNHc6GRlNlVfasZkBSMCDDCBZsAU8rDi18K9fLXKLjw

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


--
-- Name: update_updated_at_column(); Type: FUNCTION; Schema: public; Owner: ezhealth_user
--

CREATE FUNCTION public.update_updated_at_column() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;


ALTER FUNCTION public.update_updated_at_column() OWNER TO ezhealth_user;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: audit_logs; Type: TABLE; Schema: public; Owner: ezhealth_user
--

CREATE TABLE public.audit_logs (
    id integer NOT NULL,
    user_id integer,
    action character varying(255) NOT NULL,
    resource_type character varying(100),
    resource_id integer,
    details jsonb DEFAULT '{}'::jsonb,
    ip_address inet,
    user_agent text,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


ALTER TABLE public.audit_logs OWNER TO ezhealth_user;

--
-- Name: audit_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: ezhealth_user
--

CREATE SEQUENCE public.audit_logs_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.audit_logs_id_seq OWNER TO ezhealth_user;

--
-- Name: audit_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: ezhealth_user
--

ALTER SEQUENCE public.audit_logs_id_seq OWNED BY public.audit_logs.id;


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
-- Name: interface_logs; Type: TABLE; Schema: public; Owner: ezhealth_user
--

CREATE TABLE public.interface_logs (
    id integer NOT NULL,
    interface_id integer,
    execution_id uuid DEFAULT public.uuid_generate_v4(),
    status character varying(50) NOT NULL,
    message_count integer DEFAULT 0,
    error_count integer DEFAULT 0,
    details jsonb DEFAULT '{}'::jsonb,
    started_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    completed_at timestamp without time zone,
    duration_ms integer
);


ALTER TABLE public.interface_logs OWNER TO ezhealth_user;

--
-- Name: interface_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: ezhealth_user
--

CREATE SEQUENCE public.interface_logs_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.interface_logs_id_seq OWNER TO ezhealth_user;

--
-- Name: interface_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: ezhealth_user
--

ALTER SEQUENCE public.interface_logs_id_seq OWNED BY public.interface_logs.id;


--
-- Name: interface_templates; Type: TABLE; Schema: public; Owner: ezhealth_user
--

CREATE TABLE public.interface_templates (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    source_type character varying(100) NOT NULL,
    target_type character varying(100) NOT NULL,
    default_config jsonb DEFAULT '{}'::jsonb,
    is_system_template boolean DEFAULT false,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


ALTER TABLE public.interface_templates OWNER TO ezhealth_user;

--
-- Name: interface_templates_id_seq; Type: SEQUENCE; Schema: public; Owner: ezhealth_user
--

CREATE SEQUENCE public.interface_templates_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.interface_templates_id_seq OWNER TO ezhealth_user;

--
-- Name: interface_templates_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: ezhealth_user
--

ALTER SEQUENCE public.interface_templates_id_seq OWNED BY public.interface_templates.id;


--
-- Name: interfaces; Type: TABLE; Schema: public; Owner: ezhealth_user
--

CREATE TABLE public.interfaces (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    source_type character varying(100) NOT NULL,
    target_type character varying(100) NOT NULL,
    source_connectivity character varying(100) NOT NULL,
    target_connectivity character varying(100) NOT NULL,
    source_config jsonb DEFAULT '{}'::jsonb,
    target_config jsonb DEFAULT '{}'::jsonb,
    mapping_rules jsonb DEFAULT '{}'::jsonb,
    status character varying(50) DEFAULT 'draft'::character varying,
    created_by integer,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT interfaces_status_check CHECK (((status)::text = ANY ((ARRAY['draft'::character varying, 'active'::character varying, 'inactive'::character varying, 'error'::character varying])::text[])))
);


ALTER TABLE public.interfaces OWNER TO ezhealth_user;

--
-- Name: interfaces_id_seq; Type: SEQUENCE; Schema: public; Owner: ezhealth_user
--

CREATE SEQUENCE public.interfaces_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.interfaces_id_seq OWNER TO ezhealth_user;

--
-- Name: interfaces_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: ezhealth_user
--

ALTER SEQUENCE public.interfaces_id_seq OWNED BY public.interfaces.id;


--
-- Name: system_settings; Type: TABLE; Schema: public; Owner: ezhealth_user
--

CREATE TABLE public.system_settings (
    id integer NOT NULL,
    key character varying(255) NOT NULL,
    value jsonb,
    category character varying(100) DEFAULT 'general'::character varying,
    description text,
    is_editable boolean DEFAULT true,
    updated_by integer,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


ALTER TABLE public.system_settings OWNER TO ezhealth_user;

--
-- Name: system_settings_id_seq; Type: SEQUENCE; Schema: public; Owner: ezhealth_user
--

CREATE SEQUENCE public.system_settings_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.system_settings_id_seq OWNER TO ezhealth_user;

--
-- Name: system_settings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: ezhealth_user
--

ALTER SEQUENCE public.system_settings_id_seq OWNED BY public.system_settings.id;


--
-- Name: users; Type: TABLE; Schema: public; Owner: ezhealth_user
--

CREATE TABLE public.users (
    id integer NOT NULL,
    email character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    password character varying(255) NOT NULL,
    role character varying(50) DEFAULT 'user'::character varying,
    status character varying(50) DEFAULT 'active'::character varying,
    last_login_at timestamp without time zone,
    last_login_ip inet,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT users_role_check CHECK (((role)::text = ANY ((ARRAY['admin'::character varying, 'user'::character varying, 'viewer'::character varying])::text[]))),
    CONSTRAINT users_status_check CHECK (((status)::text = ANY ((ARRAY['active'::character varying, 'inactive'::character varying, 'suspended'::character varying])::text[])))
);


ALTER TABLE public.users OWNER TO ezhealth_user;

--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: ezhealth_user
--

CREATE SEQUENCE public.users_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.users_id_seq OWNER TO ezhealth_user;

--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: ezhealth_user
--

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;


--
-- Name: audit_logs id; Type: DEFAULT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.audit_logs ALTER COLUMN id SET DEFAULT nextval('public.audit_logs_id_seq'::regclass);


--
-- Name: interface_logs id; Type: DEFAULT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.interface_logs ALTER COLUMN id SET DEFAULT nextval('public.interface_logs_id_seq'::regclass);


--
-- Name: interface_templates id; Type: DEFAULT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.interface_templates ALTER COLUMN id SET DEFAULT nextval('public.interface_templates_id_seq'::regclass);


--
-- Name: interfaces id; Type: DEFAULT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.interfaces ALTER COLUMN id SET DEFAULT nextval('public.interfaces_id_seq'::regclass);


--
-- Name: system_settings id; Type: DEFAULT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.system_settings ALTER COLUMN id SET DEFAULT nextval('public.system_settings_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);


--
-- Data for Name: audit_logs; Type: TABLE DATA; Schema: public; Owner: ezhealth_user
--

COPY public.audit_logs (id, user_id, action, resource_type, resource_id, details, ip_address, user_agent, created_at) FROM stdin;
\.


--
-- Data for Name: flyway_schema_history; Type: TABLE DATA; Schema: public; Owner: ezhealth_user
--

COPY public.flyway_schema_history (installed_rank, version, description, type, script, checksum, installed_by, installed_on, execution_time, success) FROM stdin;
1	1	schema only	SQL	V1__schema_only.sql	1825906216	ezhealth_user	2025-08-25 18:48:45.753624	188	t
2	2	default config	SQL	V2__default_config.sql	-1875180401	ezhealth_user	2025-08-25 18:48:46.111538	7	t
3	3	add notification settings	SQL	V3__add_notification_settings.sql	-982453393	ezhealth_user	2025-08-25 18:48:46.200921	3	t
\.


--
-- Data for Name: interface_logs; Type: TABLE DATA; Schema: public; Owner: ezhealth_user
--

COPY public.interface_logs (id, interface_id, execution_id, status, message_count, error_count, details, started_at, completed_at, duration_ms) FROM stdin;
\.


--
-- Data for Name: interface_templates; Type: TABLE DATA; Schema: public; Owner: ezhealth_user
--

COPY public.interface_templates (id, name, source_type, target_type, default_config, is_system_template, created_at) FROM stdin;
1	HL7 ADT to FHIR Patient	hl7	fhir	{"message_types": ["ADT^A01", "ADT^A04", "ADT^A08"], "target_resource": "Patient", "source_connectivity": "tcp", "target_connectivity": "http"}	t	2025-08-25 18:48:46.186307
2	HL7 ORM to FHIR ServiceRequest	hl7	fhir	{"message_types": ["ORM^O01"], "target_resource": "ServiceRequest", "source_connectivity": "tcp", "target_connectivity": "http"}	t	2025-08-25 18:48:46.186307
3	File Processing Template	hl7	fhir	{"batch_size": 100, "file_pattern": "*.hl7", "source_connectivity": "file", "target_connectivity": "database"}	t	2025-08-25 18:48:46.186307
\.


--
-- Data for Name: interfaces; Type: TABLE DATA; Schema: public; Owner: ezhealth_user
--

COPY public.interfaces (id, name, description, source_type, target_type, source_connectivity, target_connectivity, source_config, target_config, mapping_rules, status, created_by, created_at, updated_at) FROM stdin;
\.


--
-- Data for Name: system_settings; Type: TABLE DATA; Schema: public; Owner: ezhealth_user
--

COPY public.system_settings (id, key, value, category, description, is_editable, updated_by, updated_at) FROM stdin;
1	app_name	"ezHealthKonnect"	branding	Application display name	t	\N	2025-08-25 18:48:46.186307
2	app_version	"1.0.0"	system	Application version	f	\N	2025-08-25 18:48:46.186307
3	max_interfaces_per_user	50	limits	Maximum interfaces per user	t	\N	2025-08-25 18:48:46.186307
4	session_timeout_minutes	480	security	User session timeout	t	\N	2025-08-25 18:48:46.186307
5	enable_audit_logging	true	compliance	Enable audit trail	f	\N	2025-08-25 18:48:46.186307
6	default_source_format	"hl7"	defaults	Default source message format	t	\N	2025-08-25 18:48:46.186307
7	default_target_format	"fhir"	defaults	Default target message format	t	\N	2025-08-25 18:48:46.186307
8	password_min_length	8	security	Minimum password length	t	\N	2025-08-25 18:48:46.186307
9	backup_retention_days	30	maintenance	How long to keep backups	t	\N	2025-08-25 18:48:46.186307
\.


--
-- Data for Name: users; Type: TABLE DATA; Schema: public; Owner: ezhealth_user
--

COPY public.users (id, email, name, password, role, status, last_login_at, last_login_ip, created_at, updated_at) FROM stdin;
\.


--
-- Name: audit_logs_id_seq; Type: SEQUENCE SET; Schema: public; Owner: ezhealth_user
--

SELECT pg_catalog.setval('public.audit_logs_id_seq', 1, false);


--
-- Name: interface_logs_id_seq; Type: SEQUENCE SET; Schema: public; Owner: ezhealth_user
--

SELECT pg_catalog.setval('public.interface_logs_id_seq', 1, false);


--
-- Name: interface_templates_id_seq; Type: SEQUENCE SET; Schema: public; Owner: ezhealth_user
--

SELECT pg_catalog.setval('public.interface_templates_id_seq', 3, true);


--
-- Name: interfaces_id_seq; Type: SEQUENCE SET; Schema: public; Owner: ezhealth_user
--

SELECT pg_catalog.setval('public.interfaces_id_seq', 1, false);


--
-- Name: system_settings_id_seq; Type: SEQUENCE SET; Schema: public; Owner: ezhealth_user
--

SELECT pg_catalog.setval('public.system_settings_id_seq', 9, true);


--
-- Name: users_id_seq; Type: SEQUENCE SET; Schema: public; Owner: ezhealth_user
--

SELECT pg_catalog.setval('public.users_id_seq', 1, false);


--
-- Name: audit_logs audit_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.audit_logs
    ADD CONSTRAINT audit_logs_pkey PRIMARY KEY (id);


--
-- Name: flyway_schema_history flyway_schema_history_pk; Type: CONSTRAINT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.flyway_schema_history
    ADD CONSTRAINT flyway_schema_history_pk PRIMARY KEY (installed_rank);


--
-- Name: interface_logs interface_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.interface_logs
    ADD CONSTRAINT interface_logs_pkey PRIMARY KEY (id);


--
-- Name: interface_templates interface_templates_name_key; Type: CONSTRAINT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.interface_templates
    ADD CONSTRAINT interface_templates_name_key UNIQUE (name);


--
-- Name: interface_templates interface_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.interface_templates
    ADD CONSTRAINT interface_templates_pkey PRIMARY KEY (id);


--
-- Name: interfaces interfaces_name_key; Type: CONSTRAINT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.interfaces
    ADD CONSTRAINT interfaces_name_key UNIQUE (name);


--
-- Name: interfaces interfaces_pkey; Type: CONSTRAINT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.interfaces
    ADD CONSTRAINT interfaces_pkey PRIMARY KEY (id);


--
-- Name: system_settings system_settings_key_key; Type: CONSTRAINT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.system_settings
    ADD CONSTRAINT system_settings_key_key UNIQUE (key);


--
-- Name: system_settings system_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.system_settings
    ADD CONSTRAINT system_settings_pkey PRIMARY KEY (id);


--
-- Name: users users_email_key; Type: CONSTRAINT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_email_key UNIQUE (email);


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
-- Name: idx_audit_logs_created_at; Type: INDEX; Schema: public; Owner: ezhealth_user
--

CREATE INDEX idx_audit_logs_created_at ON public.audit_logs USING btree (created_at);


--
-- Name: idx_audit_logs_user_id; Type: INDEX; Schema: public; Owner: ezhealth_user
--

CREATE INDEX idx_audit_logs_user_id ON public.audit_logs USING btree (user_id);


--
-- Name: idx_interface_logs_interface_id; Type: INDEX; Schema: public; Owner: ezhealth_user
--

CREATE INDEX idx_interface_logs_interface_id ON public.interface_logs USING btree (interface_id);


--
-- Name: idx_interface_logs_started_at; Type: INDEX; Schema: public; Owner: ezhealth_user
--

CREATE INDEX idx_interface_logs_started_at ON public.interface_logs USING btree (started_at);


--
-- Name: idx_interface_templates_name; Type: INDEX; Schema: public; Owner: ezhealth_user
--

CREATE INDEX idx_interface_templates_name ON public.interface_templates USING btree (name);


--
-- Name: idx_interfaces_created_by; Type: INDEX; Schema: public; Owner: ezhealth_user
--

CREATE INDEX idx_interfaces_created_by ON public.interfaces USING btree (created_by);


--
-- Name: idx_interfaces_status; Type: INDEX; Schema: public; Owner: ezhealth_user
--

CREATE INDEX idx_interfaces_status ON public.interfaces USING btree (status);


--
-- Name: idx_system_settings_key; Type: INDEX; Schema: public; Owner: ezhealth_user
--

CREATE INDEX idx_system_settings_key ON public.system_settings USING btree (key);


--
-- Name: idx_users_email; Type: INDEX; Schema: public; Owner: ezhealth_user
--

CREATE INDEX idx_users_email ON public.users USING btree (email);


--
-- Name: idx_users_status; Type: INDEX; Schema: public; Owner: ezhealth_user
--

CREATE INDEX idx_users_status ON public.users USING btree (status);


--
-- Name: interfaces update_interfaces_updated_at; Type: TRIGGER; Schema: public; Owner: ezhealth_user
--

CREATE TRIGGER update_interfaces_updated_at BEFORE UPDATE ON public.interfaces FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: system_settings update_system_settings_updated_at; Type: TRIGGER; Schema: public; Owner: ezhealth_user
--

CREATE TRIGGER update_system_settings_updated_at BEFORE UPDATE ON public.system_settings FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: users update_users_updated_at; Type: TRIGGER; Schema: public; Owner: ezhealth_user
--

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON public.users FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: audit_logs audit_logs_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.audit_logs
    ADD CONSTRAINT audit_logs_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: interface_logs interface_logs_interface_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.interface_logs
    ADD CONSTRAINT interface_logs_interface_id_fkey FOREIGN KEY (interface_id) REFERENCES public.interfaces(id) ON DELETE CASCADE;


--
-- Name: interfaces interfaces_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: ezhealth_user
--

ALTER TABLE ONLY public.interfaces
    ADD CONSTRAINT interfaces_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- PostgreSQL database dump complete
--

\unrestrict nyj1NVrYxWKUJ1evKmWVfNHc6GRlNlVfasZkBSMCDDCBZsAU8rDi18K9fLXKLjw

