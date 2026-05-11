-- Add structured-display widgets column to conversation_messages.
--
-- widgets stores an array of {id, kind, payload} objects produced by
-- the render_chart / render_card tools. Existing rows backfill with an
-- empty array so the frontend can treat the column as required without
-- a null branch.
ALTER TABLE conversation_messages
    ADD COLUMN widgets JSONB NOT NULL DEFAULT '[]'::jsonb;
