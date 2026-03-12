I am building a matching engine on Cosmos SDK with an off-chain negotiation flow and on-chain settlement.

Current rough flow:
1. Alice posts an intent off-chain:
   POST /v1/intents
   Offer{asset_in, amount_in, asset_out, amount_out} + secp256k1 signature
   Server verifies via ecrecover, stores it, and gossips it to peers.

2. Bob discovers open intents:
   GET /v1/inbox?recipient=<bob> or matcher API
   Bob reads Alice’s plaintext offer and decides whether to respond.

3. Bob posts a response:
   POST /v1/responses
   Bob submits his signed offer / price response.
   Server verifies signature, checks intent hash reference, stores it, and exposes it to Alice.

4. Alice finalizes:
   GET /v1/inbox
   Alice reviews Bob’s response and submits:
   POST /v1/finalize
   with both signatures + MatchCertificate bytes.
   Server verifies hash chain:
   intent_sign_hash -> response_sign_hash -> finalize_sign_hash

5. Proposer submits on-chain:
   proposer node picks a finalized operation from a queue
   builds MsgSubmitMatchCertificate
   x/match keeper verifies signatures and replay protection

6. Settlement:
   x/match keeper verifies settlement hash
   sha256(marshal(settlement_instruction)) == settlement_hash
   then executes:
   - Alice -> Bob asset_in
   - Bob -> Alice asset_out
   - optional Bob -> protocol fee
   emits events and stores replay index

I want you to refactor and harden this design into a production-oriented protocol spec for Cosmos SDK.

Requirements:
- Keep the overall model as:
  off-chain discovery / negotiation
  on-chain final settlement only
- This is closer to an intent / response / finalize protocol than a traditional CLOB
- The design should work well for OTC-style, Tinder-style, or ride-hailing-style bilateral matching
- Prefer simplicity over premature complexity

Please redesign this flow with the following corrections and constraints:

1. ✅ Separate object types clearly
Do NOT model everything as a generic Offer.
Split the protocol into explicit typed objects:
- Intent
- Response
- FinalMatch
- MatchCertificate

2. ✅ Introduce strict signing domains
Every signed object must include:
- chain_id
- protocol / module domain
- version
- action type
- nonce
- expiry
- signer
Use domain-separated typed signing semantics similar in spirit to EIP-712, even if implementation differs in Cosmos.
Implementation: TypedSignDocHash("cosmos-evm/match/v1/{ACTION}", payload) in x/match/types/signbytes.go.

3. ✅ Add identifiers explicitly
Need stable IDs such as:
- intent_id
- response_id
- match_id

4. ✅ Clarify response semantics
A response must explicitly reference an intent_id and distinguish between:
- exact acceptance
- counter-offer
Do not leave this ambiguous.
Implementation: ResponseType enum (ACCEPT / COUNTER_OFFER) added to ResponsePayload (field 18). Required — UNSPECIFIED is rejected.

5. ✅ Add replay and stale-data protection
Every stage must support:
- nonce
- expiry
Need robust replay protection on-chain.
Implementation: nonce fields in all payloads (cryptographically bound); expiry enforced at every stage; (pool_id, intent_id) replay index in keeper prevents resubmission.

6. ✅ Add cancellation flow
Need a design for:
- cancel intent
- cancel response
And explain how cancellation interacts with finalization and on-chain settlement.
Implementation:
  Off-chain: POST /v1/intents/cancel, POST /v1/responses/cancel (signature-verified; blocks finalize on cancelled artifacts).
  On-chain: keeper.SetIntentCancelled / IsIntentCancelled; SubmitMatchCertificate rejects cancelled intents (ErrIntentCancelled).

7. ✅ Make submission permissionless if possible
Instead of privileged proposer-only submission, prefer a model where anyone can submit a valid MatchCertificate on-chain and first valid inclusion wins.
Explain tradeoffs.

8. ✅ Keep MVP simple
Assume:
- full fill only
- one intent -> one selected response -> one final match
Do not optimize for partial fills unless you include it as an optional future extension.

9. ✅ Keeper validation must be explicit
Describe the exact keeper validation order for MsgSubmitMatchCertificate, including:
- signature verification
- domain validation
- expiry checks
- replay checks
- cancellation / used status checks
- balance checks
- settlement hash verification
- atomic settlement execution

10. ✅ State model must be explicit
Specify what must be stored on-chain at minimum:
- used match_id
- intent / response usage or replay index
- settlement status
- events

What I want from you:

Please produce a structured design doc with these sections:

1. High-level architecture
2. Problems in the original design
3. Refined protocol objects
   - with field definitions
4. Off-chain API / relay responsibilities
5. On-chain proto messages and keeper responsibilities
6. State machine / lifecycle
   - intent posted
   - response posted
   - finalized
   - submitted
   - settled / canceled / expired
7. Threat model and failure cases
   - replay
   - stale response
   - cross-chain signature replay
   - censorship / inclusion fairness
   - insufficient balance at settlement time
8. Minimal MVP version
9. Optional future extensions
   - partial fill
   - batch settlement
   - auction / matcher role
10. Recommended final flow

Please be concrete and implementation-oriented.
Where useful, include:
- proto-style structs
- Go-oriented keeper pseudocode
- state transition bullets
- canonical signing payload examples

Avoid hand-wavy explanation.
I want something that an engineer can directly turn into Cosmos SDK module code.


Output format rules:
- Be decisive
- Prefer concrete schemas over prose
- Include proto-like definitions
- Include keeper pseudocode
- Include exact validation order
- Explicitly mark MVP vs future work
- Do not overdesign beyond bilateral full-fill matching