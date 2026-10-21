---
country: OM
country_name: Oman
status: draft
regulator: Ministry of Transport, Communications and Information Technology
instrument: Personal Data Protection Law, Royal Decree No. 6/2022
instrument_date: 2022-02-09
in_force: 2023-02-13
implementing_regulations: Ministerial Decision No. 34/2024
localisation_required: conditional
cross_border_transfer: permitted_with_conditions
primary_source: https://decree.om/2022/rd20220006/
secondary_sources:
  - https://www.tamimi.com/news/new-omani-personal-data-protection-law-comes-into-force/
  - https://cms.law/en/omn/legal-updates/oman-personal-data-protection-law-entering-the-enforcement-phase
last_verified: null
verified_by: null
open_questions:
  - Exact conditions attached to cross-border transfer under the Executive Regulations
  - Whether any sector (banking, health, government) carries a hard localisation rule
  - Confirm the reported end of the transition period in February 2026
---

Oman has a general personal data protection law in force since 13 February 2023,
with Executive Regulations issued in 2024. Reporting indicates the transition
period ended in early 2026, making the regime fully enforceable.

For cloud region selection the practical question is whether personal data may
leave Oman and under what conditions. The law permits cross-border transfer
subject to conditions rather than prohibiting it outright, but the specific
conditions must be read from the Executive Regulations before anyone relies on
this entry.

**Relevance to this project:** Oman has no measurable in-country public cloud
region. Oracle's Muscat presence is a Dedicated Region with no public endpoint
(see ADR 0005), so Omani workloads necessarily land in UAE, Qatar, Saudi
Arabia or Europe. That makes the cross-border conditions the deciding factor
for anyone building here, not a footnote.
