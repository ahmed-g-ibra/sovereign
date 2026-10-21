-- Seed the control plane's target list with what Phase 0 verified by hand.
-- Idempotent: re-running updates in place rather than failing.

INSERT INTO targets (id, provider, region, country, city, host, status, is_control, note) VALUES
  ('aws-me-south-1',   'aws',   'me-south-1',   'BH', 'Manama',    's3.me-south-1.amazonaws.com',   'live', false, 'DNS resolves; TCP timed out from the first vantage point on 2026-10-21. Keep probing before concluding anything.'),
  ('aws-me-central-1', 'aws',   'me-central-1', 'AE', 'Dubai',     's3.me-central-1.amazonaws.com', 'live', false, NULL),
  ('aws-eu-central-1', 'aws',   'eu-central-1', 'DE', 'Frankfurt', 's3.eu-central-1.amazonaws.com', 'live', true,  'Control. The number a local region has to beat.'),
  ('aws-sa-pending',   'aws',   'unconfirmed',  'SA', 'unconfirmed','s3.me-central-2.amazonaws.com','announced', false, 'AWS targets December 2026. Region code is a guess; confirm at launch.'),
  ('gcp-me-central1',  'gcp',   'me-central1',  'QA', 'Doha',      'storage.me-central1.rep.googleapis.com',  'live', false, 'Confirmed 2026-10-21.'),
  ('gcp-me-central2',  'gcp',   'me-central2',  'SA', 'Dammam',    'storage.me-central2.rep.googleapis.com',  'live', false, 'Confirmed 2026-10-21.'),
  ('gcp-europe-west3', 'gcp',   'europe-west3', 'DE', 'Frankfurt', 'storage.europe-west3.rep.googleapis.com', 'live', true,  NULL),
  ('oci-me-dubai-1',   'oci',   'me-dubai-1',   'AE', 'Dubai',     'objectstorage.me-dubai-1.oraclecloud.com',    'live', false, 'Confirmed 2026-10-21.'),
  ('oci-me-abudhabi-1','oci',   'me-abudhabi-1','AE', 'Abu Dhabi', 'objectstorage.me-abudhabi-1.oraclecloud.com', 'live', false, 'Confirmed 2026-10-21.'),
  ('oci-me-jeddah-1',  'oci',   'me-jeddah-1',  'SA', 'Jeddah',    'objectstorage.me-jeddah-1.oraclecloud.com',   'live', false, 'Confirmed 2026-10-21.'),
  ('oci-me-riyadh-1',  'oci',   'me-riyadh-1',  'SA', 'Riyadh',    'objectstorage.me-riyadh-1.oraclecloud.com',   'live', false, 'Confirmed 2026-10-21.'),
  ('oci-me-dcc-muscat-1','oci', 'me-dcc-muscat-1','OM','Muscat',   'objectstorage.me-dcc-muscat-1.oraclecloud.com','rejected', false, 'NXDOMAIN. Dedicated Region, no public endpoint: Oman has no measurable in-country region today.'),
  ('azure-global-arm', 'azure', 'global',       'XX', 'anycast',   'management.azure.com', 'live', true, 'Global anycast control only. Never present as a regional Azure number.'),
  ('azure-uaenorth-blob','azure','uaenorth',    'AE', 'Dubai',     'uaenorth.blob.core.windows.net',     'rejected', false, 'NXDOMAIN. Azure publishes no per-region public endpoint; see ADR 0005.'),
  ('azure-qatarcentral-blob','azure','qatarcentral','QA','Doha',   'qatarcentral.blob.core.windows.net', 'rejected', false, 'Resolves, but it is a stranger''s storage account, not an Azure endpoint. See ADR 0005.')
ON CONFLICT (id) DO UPDATE SET
    provider   = EXCLUDED.provider,
    region     = EXCLUDED.region,
    country    = EXCLUDED.country,
    city       = EXCLUDED.city,
    host       = EXCLUDED.host,
    status     = EXCLUDED.status,
    is_control = EXCLUDED.is_control,
    note       = EXCLUDED.note,
    updated_at = now();
