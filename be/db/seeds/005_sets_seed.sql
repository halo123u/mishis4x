-- Fixture set for the collection-tracker feature (see PR #77) - this is
-- what collection.spec.ts's e2e test actually exercises. Not real catalog
-- data (that comes from the CSV import job, #68/#70) - just enough of a
-- fixture to have something real to click through in tests/local dev.
INSERT INTO sets (id, name, card_count, release_date, status)
VALUES ('01900000-0000-7000-8000-000000000001', 'Brown Dust 2', 100, '2026-06-01', 'active');
