-- +goose Up
-- +goose StatementBegin
ALTER TABLE users
  ADD CONSTRAINT chk_users_username_not_pure_digits
  CHECK (username IS NULL OR username REGEXP '[^0-9]');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP CHECK chk_users_username_not_pure_digits;
-- +goose StatementEnd
