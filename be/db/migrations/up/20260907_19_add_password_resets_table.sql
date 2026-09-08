-- "Forgot password" recovery - a requested reset gets a fresh row here,
-- emailed out as a link (see email.Service.SendPasswordResetEmail); the
-- token IS the credential, same as sessions.id and invites.code (a
-- crypto/rand opaque value, nothing to sign or verify beyond the
-- server-side row lookup itself - see persist.NewPasswordResetToken).
--
-- Single-use (used_at) and time-limited (expires_at, see
-- passwordResetTTL) - unlike a session or an invite code, this is a
-- short-lived credential meant to be spent within the next hour, not
-- something that should stay redeemable indefinitely. Both conditions are
-- checked together, atomically, by ConsumePasswordReset's own UPDATE - not
-- a check-then-write in application code, the same concurrency-safety
-- shape RedeemInvite already uses for its own single-use code.
--
-- No unique constraint on user_id: CreatePasswordReset deletes any of a
-- user's existing unused resets before inserting a fresh one (so a stale
-- link from an earlier request stops working once a newer one is
-- requested), but that's an application-level policy, not something the
-- schema itself needs to enforce.
CREATE TABLE password_resets (
    token CHAR(43) NOT NULL PRIMARY KEY,
    user_id INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    used_at TIMESTAMP NULL,
    INDEX idx_password_resets_user_id (user_id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);
