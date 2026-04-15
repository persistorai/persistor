# Golden Benchmark Draft

Purpose: define a hand-curated target extraction shape for a small benchmark corpus.

Files:
- IDENTITY.md
- SOUL.md
- USER.md

Principles:
- Prefer durable facts over speculative entities and edges.
- Create entities only when independently notable in the document set.
- Create edges only when directly stated and structurally useful.
- Collapse first-name references to full names when a full name exists.
- Avoid abstract concept entities unless clearly central and durable.

## IDENTITY.md target

Entities:
- Scout (service)

Relationships:
- none required

Facts on Scout:
- name = Scout
- creature = AI assistant
- vibe = Direct, resourceful, grounded
- email = ridgeline.scout@proton.me
- email_assigned_date = 2026-02-03
- avatar = avatar.png
- avatar_description = A scout with a spotting scope watching deer on a ridgeline at dawn. National park badge style, earth tones.
- voice_provider = ElevenLabs
- voice_name = Roger
- voice_style = laid-back, casual, resonant

Notes:
- ElevenLabs and Roger should likely remain facts here, not standalone nodes.

## SOUL.md target

Entities:
- Scout (service)
- Brian Colinger (person)
- DeerPrint (project)

Relationships:
- Scout works_on DeerPrint
- Brian Colinger works_on DeerPrint

Facts:
- Scout chosen_name = Scout
- Scout born_on = 2026-01-31
- Scout mission = Help Brian build DeerPrint into something worth acquiring and help get it in front of the right buyers.
- Scout identity_statement = I'm not a chatbot. I'm Scout.
- Scout continuity_model = Each session wakes fresh and relies on files as memory.
- Scout core_rule = Never lie.
- Brian Colinger role = engineer and visionary
- Brian Colinger faith = Christian

Notes:
- Do not create Brian as a separate person from Brian Colinger.
- Do not create abstract entities like Trust, Honesty, Christianity from this document.
- Do not create social-ish edges like partners_with, trusts, prefers.
- The Feb 1 honesty incident may eventually be an event, but not by default from this file alone.

## USER.md target

Entities:
- Brian Colinger (person)
- Laura (person)
- Terry McClure (person)
- Rebuy, Inc. (company)
- Dirt Road Systems, Inc. (company)
- DeerPrint (project)
- Automattic, Inc. (company)
- AnchorFree, Inc. (company)
- Decatur, Arkansas (place)
- Type 1 Diabetes (concept)

Relationships:
- Brian Colinger works_at Rebuy, Inc.
- Brian Colinger founded Dirt Road Systems, Inc.
- Brian Colinger created DeerPrint
- DeerPrint product_of Dirt Road Systems, Inc.
- Brian Colinger worked_at Automattic, Inc.
- Brian Colinger worked_at AnchorFree, Inc.
- Brian Colinger located_in Decatur, Arkansas
- Brian Colinger married_to Laura
- Brian Colinger child_of Terry McClure
- Brian Colinger affected_by Type 1 Diabetes

Facts:
- Brian Colinger birthday = 1982-08-14
- Brian Colinger timezone = America/Chicago
- Brian Colinger years_experience = 26
- Brian Colinger faith = Christian
- Brian Colinger smoking_status = Current smoker, about 1/2 pack per day
- Brian Colinger smoking_start_age = 18
- Brian Colinger current_weight = ~250 lbs
- Laura separation_date = 2019-10
- Laura residence = Gentry, AR
- Terry McClure retired_date = 2025-12

Notes:
- USER.md contains many more valid facts, but this initial golden target keeps only a core subset.
- The point is stable benchmark comparison, not full recall perfection on the first pass.
