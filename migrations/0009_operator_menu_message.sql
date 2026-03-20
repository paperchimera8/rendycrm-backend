ALTER TABLE operator_bot_bindings
    ADD COLUMN IF NOT EXISTS last_menu_message_id BIGINT NOT NULL DEFAULT 0;
