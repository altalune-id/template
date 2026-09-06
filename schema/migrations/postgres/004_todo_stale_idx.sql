-- +goose Up
-- +goose StatementBegin

{{if .Role}}SET ROLE {{.Role}};{{end}}

CREATE INDEX {{.TablePrefix}}todos_org_stale_idx
  ON {{.Schema}}.{{.TablePrefix}}todos (org_id, created_at)
  WHERE done = false;
{{if .Role}}RESET ROLE;{{end}}

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

{{if .Role}}SET ROLE {{.Role}};{{end}}

DROP INDEX IF EXISTS {{.Schema}}.{{.TablePrefix}}todos_org_stale_idx;
{{if .Role}}RESET ROLE;{{end}}

-- +goose StatementEnd
