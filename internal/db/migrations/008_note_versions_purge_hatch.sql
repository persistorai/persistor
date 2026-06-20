-- +goose Up
-- A tenant hard-delete ("delete my account") must be able to purge the
-- append-only note_versions history — the one legitimate exception to the
-- append-only guard. Gate it on a tx-local GUC (app.purge='on') so ordinary
-- write paths can never rewrite history: only an operator DELETE/TRUNCATE issued
-- with the purge flag set is permitted. The flag is set by Store.DeleteTenant
-- inside its transaction and never leaks (set_config(..., true) is tx-scoped).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION note_versions_append_only()
RETURNS TRIGGER AS $$
BEGIN
    IF current_setting('app.purge', true) = 'on' THEN
        RETURN OLD; -- purge permitted (OLD is NULL for the statement-level TRUNCATE trigger)
    END IF;
    RAISE EXCEPTION 'note_versions is append-only: % not permitted', TG_OP;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION note_versions_append_only()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'note_versions is append-only: % not permitted', TG_OP;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
