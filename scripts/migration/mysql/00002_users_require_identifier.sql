-- +goose Up
-- +goose StatementBegin
ALTER TABLE users
  ADD CONSTRAINT chk_users_identifier_present
  CHECK (username IS NOT NULL OR phone IS NOT NULL OR email IS NOT NULL);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP CHECK chk_users_identifier_present;
-- +goose StatementEnd
