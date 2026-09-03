-- Nullable - existing/seeded accounts (e.g. the seeded test/test user)
-- predate this column and never had an email collected. New accounts get
-- it copied over from the invites row they redeemed at signup time (see
-- handlers.UserCreate) - not enforced UNIQUE for now, since nothing yet
-- stops two separate invite requests from using the same address and
-- both getting approved; revisit if that turns out to matter.
ALTER TABLE users ADD COLUMN email_address VARCHAR(255) NULL;
