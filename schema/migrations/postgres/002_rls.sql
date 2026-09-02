-- +goose Up
-- +goose StatementBegin

{{if .Role}}SET ROLE {{.Role}};{{end}}

{{if .RLSEnforce}}
ALTER TABLE {{.Schema}}.{{.TablePrefix}}orgs ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}orgs FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}orgs_tenant
  ON {{.Schema}}.{{.TablePrefix}}orgs
  USING (id = current_setting('app.current_org_id', true)::uuid);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}memberships FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}memberships_tenant
  ON {{.Schema}}.{{.TablePrefix}}memberships
  USING (org_id = current_setting('app.current_org_id', true)::uuid);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}projects FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}projects_tenant
  ON {{.Schema}}.{{.TablePrefix}}projects
  USING (org_id = current_setting('app.current_org_id', true)::uuid);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}invites ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}invites FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}invites_tenant
  ON {{.Schema}}.{{.TablePrefix}}invites
  USING (org_id = current_setting('app.current_org_id', true)::uuid);

ALTER TABLE {{.Schema}}.{{.TablePrefix}}todos ENABLE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}todos FORCE ROW LEVEL SECURITY;
CREATE POLICY {{.TablePrefix}}todos_tenant
  ON {{.Schema}}.{{.TablePrefix}}todos
  USING (org_id = current_setting('app.current_org_id', true)::uuid);
{{end}}

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

{{if .Role}}SET ROLE {{.Role}};{{end}}

{{if .RLSEnforce}}
DROP POLICY IF EXISTS {{.TablePrefix}}todos_tenant ON {{.Schema}}.{{.TablePrefix}}todos;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}todos NO FORCE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}todos DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS {{.TablePrefix}}invites_tenant ON {{.Schema}}.{{.TablePrefix}}invites;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}invites NO FORCE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}invites DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS {{.TablePrefix}}projects_tenant ON {{.Schema}}.{{.TablePrefix}}projects;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}projects NO FORCE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}projects DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS {{.TablePrefix}}memberships_tenant ON {{.Schema}}.{{.TablePrefix}}memberships;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}memberships NO FORCE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}memberships DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS {{.TablePrefix}}orgs_tenant ON {{.Schema}}.{{.TablePrefix}}orgs;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}orgs NO FORCE ROW LEVEL SECURITY;
ALTER TABLE {{.Schema}}.{{.TablePrefix}}orgs DISABLE ROW LEVEL SECURITY;
{{end}}

-- +goose StatementEnd
