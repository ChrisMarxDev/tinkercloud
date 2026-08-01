-- Browser control sessions and CLI bearer tokens are distinct credential types.
-- Legacy control challenges have no channel and deliberately fail closed.
ALTER TABLE otp_challenges ADD COLUMN control_channel TEXT CHECK(control_channel IS NULL OR control_channel IN ('browser','cli'));
