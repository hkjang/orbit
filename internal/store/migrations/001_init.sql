CREATE TABLE IF NOT EXISTS schema_migrations (
  version integer PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY,
  username text NOT NULL,
  email text NOT NULL DEFAULT '',
  display_name text NOT NULL,
  password_hash text NOT NULL DEFAULT '',
  role text NOT NULL DEFAULT 'member' CHECK (role IN ('admin','team_lead','member')),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
  oidc_subject text NOT NULL DEFAULT '',
  last_login_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_uidx ON users (lower(username));
CREATE UNIQUE INDEX IF NOT EXISTS users_oidc_subject_uidx ON users (oidc_subject) WHERE oidc_subject <> '';

CREATE TABLE IF NOT EXISTS sessions (
  token_hash text PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS settings (
  namespace text NOT NULL,
  key text NOT NULL,
  value jsonb NOT NULL DEFAULT '{}',
  encrypted_value text NOT NULL DEFAULT '',
  updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(namespace, key)
);

CREATE TABLE IF NOT EXISTS user_preferences (
  user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  theme text NOT NULL DEFAULT 'dark',
  locale text NOT NULL DEFAULT 'ko-KR',
  font_scale numeric(3,2) NOT NULL DEFAULT 1.00,
  reduce_motion boolean NOT NULL DEFAULT false,
  rediscover_frequency text NOT NULL DEFAULT 'weekly',
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_key_versions (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  version integer NOT NULL,
  wrapped_key text NOT NULL,
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','retired','revoked')),
  rotated_by uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  retired_at timestamptz,
  UNIQUE(user_id, version)
);
CREATE UNIQUE INDEX IF NOT EXISTS user_key_active_uidx ON user_key_versions(user_id) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS key_permissions (
  id uuid PRIMARY KEY,
  key_version_id uuid NOT NULL REFERENCES user_key_versions(id) ON DELETE CASCADE,
  principal_type text NOT NULL CHECK (principal_type IN ('owner','role','user','service')),
  principal_id text NOT NULL,
  permissions jsonb NOT NULL DEFAULT '["decrypt","encrypt"]',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(key_version_id, principal_type, principal_id)
);

CREATE TABLE IF NOT EXISTS people (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  display_name text NOT NULL,
  company text NOT NULL DEFAULT '',
  role_title text NOT NULL DEFAULT '',
  avatar_url text NOT NULL DEFAULT '',
  email_cipher text NOT NULL DEFAULT '',
  phone_cipher text NOT NULL DEFAULT '',
  note_cipher text NOT NULL DEFAULT '',
  key_version integer NOT NULL,
  first_met date,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS people_user_name_idx ON people(user_id, display_name);

CREATE TABLE IF NOT EXISTS relationships (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  person_id uuid NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  importance numeric(5,4) NOT NULL DEFAULT .5 CHECK (importance >= 0 AND importance <= 1),
  closeness numeric(5,4) NOT NULL DEFAULT .5 CHECK (closeness >= 0 AND closeness <= 1),
  momentum numeric(5,4) NOT NULL DEFAULT 0 CHECK (momentum >= -1 AND momentum <= 1),
  interaction_score numeric(5,4) NOT NULL DEFAULT 0,
  stable_x numeric(8,4) NOT NULL DEFAULT 0,
  stable_y numeric(8,4) NOT NULL DEFAULT 0,
  categories jsonb NOT NULL DEFAULT '[]',
  relationship_label text NOT NULL DEFAULT '',
  last_interaction_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(user_id, person_id)
);

CREATE TABLE IF NOT EXISTS interactions (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  person_id uuid NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN ('meeting','call','message','note','other')),
  occurred_at timestamptz NOT NULL,
  weight numeric(5,2) NOT NULL DEFAULT 1,
  summary_cipher text NOT NULL DEFAULT '',
  key_version integer NOT NULL,
  source text NOT NULL DEFAULT 'manual',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS interactions_person_time_idx ON interactions(user_id, person_id, occurred_at DESC);

CREATE TABLE IF NOT EXISTS memories (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  person_id uuid REFERENCES people(id) ON DELETE CASCADE,
  title text NOT NULL,
  content_cipher text NOT NULL,
  key_version integer NOT NULL,
  occurred_at timestamptz,
  source_type text NOT NULL DEFAULT 'manual',
  source_reference text NOT NULL DEFAULT '',
  topics jsonb NOT NULL DEFAULT '[]',
  status text NOT NULL DEFAULT 'approved' CHECK (status IN ('draft','pending','approved','rejected')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS memories_user_time_idx ON memories(user_id, occurred_at DESC);

CREATE TABLE IF NOT EXISTS contexts (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  context_type text NOT NULL DEFAULT 'group',
  color text NOT NULL DEFAULT '#7c6cf2',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(user_id, name)
);

CREATE TABLE IF NOT EXISTS person_contexts (
  person_id uuid NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  context_id uuid NOT NULL REFERENCES contexts(id) ON DELETE CASCADE,
  PRIMARY KEY(person_id, context_id)
);

CREATE TABLE IF NOT EXISTS approval_requests (
  id uuid PRIMARY KEY,
  requester_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  reviewer_id uuid REFERENCES users(id) ON DELETE SET NULL,
  resource_type text NOT NULL,
  resource_id uuid NOT NULL,
  action text NOT NULL,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','cancelled')),
  request_note text NOT NULL DEFAULT '',
  review_note text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  reviewed_at timestamptz
);
CREATE INDEX IF NOT EXISTS approvals_status_idx ON approval_requests(status, created_at DESC);

CREATE TABLE IF NOT EXISTS api_keys (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  prefix text NOT NULL,
  secret_hash text NOT NULL,
  scopes jsonb NOT NULL DEFAULT '[]',
  expires_at timestamptz,
  last_used_at timestamptz,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS api_keys_prefix_idx ON api_keys(prefix);

CREATE TABLE IF NOT EXISTS audit_logs (
  id bigserial PRIMARY KEY,
  actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
  action text NOT NULL,
  resource_type text NOT NULL,
  resource_id text NOT NULL DEFAULT '',
  ip_address text NOT NULL DEFAULT '',
  detail jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS audit_logs_time_idx ON audit_logs(created_at DESC);

