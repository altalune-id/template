-- +goose Up
-- +goose StatementBegin

{{if .Role}}SET ROLE {{.Role}};{{end}}

CREATE TABLE {{.Schema}}.{{.TablePrefix}}bootstrap (
  id            INT PRIMARY KEY CHECK (id = 1),
  onboarded_at  TIMESTAMPTZ NOT NULL,
  onboarded_by  UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}users(id),
  method        TEXT NOT NULL CHECK (method IN ('env-genesis','web-onboard','cli-init'))
);
{{if .Role}}RESET ROLE;{{end}}

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

{{if .Role}}SET ROLE {{.Role}};{{end}}

DROP TABLE IF EXISTS {{.Schema}}.{{.TablePrefix}}bootstrap;
{{if .Role}}RESET ROLE;{{end}}

-- +goose StatementEnd
