-- See 005_sets_seed.sql - fixture cards for the same set, referenced by
-- collection.spec.ts's e2e test.
INSERT INTO cards (id, set_id, name, code, rarity)
VALUES
  ('01900000-0000-7000-8000-000000000011', '01900000-0000-7000-8000-000000000001', 'Poolside Fairy Refithea', 'BRD/W139-001S', 'SR 3-star'),
  ('01900000-0000-7000-8000-000000000012', '01900000-0000-7000-8000-000000000001', 'Pool Party Angelica', 'BRD/W139-003S', 'SR 2-star'),
  ('01900000-0000-7000-8000-000000000013', '01900000-0000-7000-8000-000000000001', 'Dark Saint Liberta', 'BRD/W139-057S', 'SR 2-star'),
  ('01900000-0000-7000-8000-000000000014', '01900000-0000-7000-8000-000000000001', 'Celebrity Bunny Roen', 'BRD/W139-054S', 'SR 3-star'),
  ('01900000-0000-7000-8000-000000000015', '01900000-0000-7000-8000-000000000001', 'Beachside Angel Teresse', 'BRD/W139-075S', 'SR 3-star');
