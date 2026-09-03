-- Gates signup behind a one-time invite link instead of open public
-- signup - see be/cmd/invite.go for how a token gets minted (owner-run
-- CLI, same convention as process-set/sync-prices), and
-- handlers.UserCreate for how it gets redeemed.
--
-- token is the primary key itself (43 chars - base64 URL-safe encoding
-- of 32 random bytes, same shape/entropy as a real session token, see
-- persist.NewSessionToken) rather than a separate surrogate id - there's
-- nothing else that would ever need to reference an invite row by a
-- different key.
--
-- used_at IS NULL is the actual security gate (an invite is redeemable
-- exactly once, atomically - see RedeemInvite). used_by_user_id is
-- nullable and purely an audit trail of who redeemed it, not itself part
-- of the gate - it can't be set until the new user row exists, which
-- happens after the invite is already claimed (see UserCreate's doc
-- comment for why a failed signup after a successful claim burns the
-- invite rather than un-claiming it).
CREATE TABLE invites (
    token CHAR(43) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    used_at TIMESTAMP NULL,
    used_by_user_id INT NULL,
    PRIMARY KEY (token),
    FOREIGN KEY (used_by_user_id) REFERENCES users(id)
);
