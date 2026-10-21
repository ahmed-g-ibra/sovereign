---
country: AE
country_name: United Arab Emirates
status: draft
regulator: UAE Data Office (federal); DIFC Commissioner and ADGM Registration Authority in the financial free zones
instrument: Federal Decree-Law No. 45 of 2021 on the Protection of Personal Data
instrument_date: 2021-09-20
in_force: 2022-01-02
implementing_regulations: pending at the time of writing -- confirm current status
localisation_required: sectoral
cross_border_transfer: permitted_with_conditions
primary_source: https://u.ae/en/about-the-uae/digital-uae/data/data-protection-laws
secondary_sources: []
last_verified: null
verified_by: null
open_questions:
  - Whether the federal Executive Regulations have been issued, and what they require
  - Scope of health-data localisation under the federal health ICT law
  - How DIFC and ADGM regimes interact with the federal law for a company in a free zone
  - Central Bank requirements for financial institutions
---

The UAE has three overlapping regimes: the federal PDPL, and separate laws in
the DIFC and ADGM financial free zones. Which applies depends on where the
entity is established, not where the data sits, and getting that wrong is the
most common mistake in UAE cloud architecture.

The federal law permits cross-border transfer subject to conditions. The
sharper constraint for cloud selection is sectoral: health data and certain
financial data are reported to carry in-country requirements that the general
law does not impose. Those sector rules, not the PDPL, are usually what forces
a UAE region.

**Relevance to this project:** the UAE is the best-connected measurement target
in the Gulf, with AWS me-central-1, Azure UAE North and Central, and two Oracle
regions. For an Omani workload it is the nearest in-region option and the
default answer absent a reason to go elsewhere.
