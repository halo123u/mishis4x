-- Gates signup behind an invite code instead of open public signup. There
-- is no self-service minting: everything starts with someone submitting
-- the public "request an invite" form (email_address + a freshly
-- generated code, status 'requested'), the owner then approves or denies
-- it via the invite-approve/invite-deny CLI commands (be/cmd/invite.go),
-- and approval is what actually emails the code out. See
-- handlers.UserCreate for how a code gets redeemed at signup.
--
-- id is a plain surrogate key (AUTO_INCREMENT, matching users.id) - unlike
-- code, it's never handed to an untrusted party, so there's no reason for
-- it to carry any entropy of its own; the owner refers to a request by
-- this id when approving/denying it (see invite-list).
--
-- code is the actual bearer credential (43 chars - base64 URL-safe
-- encoding of 32 random bytes, same shape/entropy as a real session
-- token, see persist.NewSessionToken) - generated up front at request
-- time, not at approval time, so approving is just a status flip plus
-- sending the email, not a second code-generation step.
--
-- status is the security gate, checked and flipped atomically on every
-- transition (see RedeemInvite/ApproveInvite/DenyInvite - all single
-- UPDATEs with a WHERE on the expected current status, not a
-- check-then-write done in application code):
--   requested -> approved (owner approves; email goes out)
--             -> denied   (owner denies; code never leaves the DB)
--   approved  -> redeemed (code used to complete signup)
--
-- redeemed_by_user_id is nullable and purely an audit trail of who
-- redeemed the code, not itself part of the gate - it can't be set until
-- the new user row exists, which happens after the code is already
-- claimed (see UserCreate's doc comment for why a failed signup after a
-- successful claim burns the code rather than un-claiming it).
CREATE TABLE invites (
    id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    code CHAR(43) NOT NULL UNIQUE,
    status VARCHAR(20) NOT NULL DEFAULT 'requested',
    email_address VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    redeemed_at TIMESTAMP NULL,
    redeemed_by_user_id INT NULL,
    FOREIGN KEY (redeemed_by_user_id) REFERENCES users(id)
);
