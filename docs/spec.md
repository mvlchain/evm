# Matching Protocol Specification

- Status: Draft
- Spec Version: `v1alpha1`
- Last Updated: `2026-03-06`
- SSOT: `docs/spec.md` + `proto/match/v1/*.proto`
- Normative words: `MUST`, `MUST NOT`, `SHOULD`, `MAY`

## Table of Contents

1. Purpose and Scope
2. Hard Invariants
3. Architecture Boundaries
4. Trust Boundaries
5. Data Model Boundaries
6. Canonical Serialization and Signing
7. Protocol Flow and State Machine
8. Validation Rules
9. Error Codes
10. Privacy and Logging Policy
11. Validation Checklist
12. Versioning and Compatibility

## 1. Purpose and Scope

본 문서는 "오프체인 의사 게시 + 양방향 서명 + finalize + (선택) 온체인 증빙" 매칭 프로토콜의 단일 기준(SSOT)을 정의한다.

포함 범위:
1. Intent/Response/Finalize/MatchCertificate 데이터 모델
2. deterministic protobuf 기반 서명 규격
3. 검증 규칙(서명/만료/리플레이/권한)
4. 오프체인 보드 API 및 선택적 온체인 제출 경계
5. 에러코드 체계

비포함 범위:
1. 비즈니스 랭킹 알고리즘
2. UI/UX 구현 상세
3. 체인별 가스 정책

## 2. Hard Invariants

1. Sign-bytes 규격은 하나만 사용한다: `Protobuf deterministic marshal (Deterministic=true)`.
2. JSON 문자열 또는 임의 직렬화 서명은 금지한다.
3. PII/민감정보 평문을 온체인/로그에 저장하지 않는다.
4. 리플레이 방지는 필수다: 중복 `(pool_id, intent_id)`는 거부한다.
5. 만료 검증은 필수다: `expires_at`(또는 `expires_unix`) 경과 시 거부한다.
6. 문서와 proto는 함께 변경한다.

## 3. Architecture Boundaries

### 3.1 Components

| Component | 책임 | 금지 |
|---|---|---|
| Signer Client (Maker/Responder) | 서명 가능한 proto payload 구성, deterministic marshal, 서명/검증 | JSON sign-bytes 생성 |
| Off-chain Board/Server (memboard) | 게시/조회 API, inbox/outbox 접근제어, rate-limit, short-term operation queue/canonical proposer view | 서명 위조, 평문 민감정보 로그 저장 |
| Optional On-chain Verifier (`x/match`) | certificate 검증(서명/만료/중복), Match 이벤트 emit | 평문 프로필/메시지 저장 |

### 3.2 Interface Boundaries

1. Client ↔ Server: signed protobuf artifact 전송
2. Server ↔ Chain(optional): `MsgSubmitMatchCertificate`
3. Chain/Event: 해시/식별자만 노출

## 4. Trust Boundaries

| Boundary | 신뢰 대상 | 비신뢰 대상 | 필수 제어 |
|---|---|---|---|
| Client key boundary | 개인키 보관/서명 | 서버의 무결성 보장 | 로컬 서명, 키 보호 |
| Client-server boundary | 전송 채널 가용성 | payload 진실성 | 서명 재검증 |
| Server storage boundary | 저장 가용성 | 민감정보 기밀성(기본) | 암호화 저장, 최소권한 |
| Server-chain boundary | tx relay | 개인정보 보호 | 최소 증빙만 제출 |

## 5. Data Model Boundaries

### 5.1 Canonical Objects

| Object | Proto Type | 주 저장소 | 온체인 허용 |
|---|---|---|---|
| Intent | `match.v1.SignedIntent` | Off-chain | `pool_id`, `intent_id`, `context_hash` |
| Response | `match.v1.SignedResponse` | Off-chain | 기본 비저장 |
| Finalize | `match.v1.SignedFinalize` | Off-chain | 기본 비저장 |
| MatchCertificate | `match.v1.MatchCertificate` | Off-chain (+ optional chain) | 해시/서명/ID |

### 5.2 Privacy Rule

1. 프로필/메시지/조건 원문은 오프체인에만 둔다.
2. 체인/로그/이벤트에는 `*_hash`와 식별자만 허용한다.
3. `context_hash`는 도메인 분리된 해시 정책(버전 포함)을 사용해야 한다.

## 6. Canonical Serialization and Signing

### 6.1 Signable Messages

다음 객체를 서명 대상으로 사용한다.
1. `IntentSignDoc`
2. `ResponseSignDoc`
3. `FinalizeSignDoc`
4. `CertificateSignDoc`

### 6.2 Canonical Sign-bytes Procedure (Normative)

1. 타입별 sign-doc 메시지를 구성한다.
2. `sign_doc_type`을 정확히 설정한다(`UNSPECIFIED` 금지).
3. sign-doc를 deterministic protobuf로 marshal 한다.
4. `sign_bytes_hash = SHA256(sign_bytes)`를 계산한다.
5. 서명 알고리즘에 따라 `sign_bytes_hash`를 서명한다.
6. 검증자는 동일 절차로 sign-bytes를 재생성해 검증한다.

### 6.3 Hash-Chaining Rule

1. `ResponsePayload.intent_sign_hash = SHA256(IntentSignDoc bytes)`
2. `FinalizePayload.intent_sign_hash/response_sign_hash`로 이전 단계를 바인딩
3. `CertificatePayload.intent_sign_hash/response_sign_hash/finalize_sign_hash`로 전체 플로우 바인딩

### 6.4 Strict Rules

1. JSON sign-bytes 입력은 즉시 거부한다.
2. 서명에 사용되는 proto에는 `map`, `float`, `double` 사용을 피한다.
3. 시간값은 unix seconds 정수 필드로 통일한다.

## 7. Protocol Flow and State Machine

1. `PublishIntent`: maker 서명 intent 게시
2. `PublishResponse`: responder 서명 response 게시
3. `PublishFinalize`: 양측 finalize 서명 게시
4. `BuildCertificate`: MatchCertificate 구성
5. `SubmitMatchCertificate`(선택): 온체인 검증/기록

상태:
`OPEN -> RESPONDED -> FINALIZED -> CERTIFIED_OFFCHAIN -> CERTIFIED_ONCHAIN(optional)`

### 7.1 dYdX-style Short-term Path (Matchboard Profile)

`short-term intent/response/finalize`는 기본적으로 **메모리 중심(memboard/memclob profile)** 으로 처리한다.

1. Signed payload(또는 sign-doc hash 바인딩 artifact)는 validator/board 노드 간 전파 가능한 형태여야 한다.
2. Proposer는 memboard의 pending set에서 **canonical 정렬 규칙**으로 operation batch를 선택해야 한다.
3. Canonical batch는 all-or-nothing 검증 경로를 가져야 하며, 한 항목이라도 실패하면 전체 batch를 롤백해야 한다.
4. 온체인에는 최종 검증 통과 결과와 replay 방지용 최소 상태(`(pool_id, intent_id)` 인덱스 + match_id + parties)만 남긴다.
5. 민감 본문은 계속 오프체인에 두고 체인/이벤트에는 `context_hash` 등 해시만 기록한다.

참고 구현 프로파일(P2P reactor + HTTP fallback):
- 기본: CometBFT P2P reactor 채널(`MATCHBOARD_GOSSIP`)로 intent envelope를 전파한다(validator peer topology 재사용, 별도 peer list 불필요).
- fallback: CometBFT 없이 `--grpc-only` 등으로 실행할 때만 HTTP internal gossip relay를 사용한다.
- `POST /v1/internal/gossip/{intents|responses|finalize}` : peer-to-peer signed payload relay (shared secret 보호, envelope 기반 TTL/hops/dedup 적용)
- `GET /v1/stream/intents?intent_type=request|accept|finalize&requester=&responder=&pool_id=` : SSE subscription 기반 intent delivery (polling 대체)
- `GET /v1/matcher/candidates?limit=N` : request/accept index 기반 deterministic candidate 조회(lowest score hash 우선)
- `GET /v1/proposer/matches?limit=N` : proposer pre-match candidate batch 조회(canonical_match_batch_hash 포함)
- `POST /v1/proposer/matches/build` : 선택한 match id에 대해 deterministic `MsgSubmitMatchCertificate` payload bytes를 생성(block builder input)
- `POST /v1/proposer/matches/commit` : proposer가 선택한 match id 집합을 atomic commit(실패 시 전체 롤백)
- `GET /v1/proposer/operations?limit=N` : canonical 순서의 pending short-term operations 조회
- `POST /v1/proposer/operations/commit` : proposer가 포함한 operation id 집합을 atomic commit
- `POST /v1/finalize` 는 선택적으로 `match_certificate`(deterministic protobuf bytes, base64 JSON)를 포함할 수 있으며, proposer-ABCI 경로에서 직접 `x/match` batch 검증에 사용된다.

gossip envelope 규칙:
1. `message_id`는 deterministic hash로 생성되어야 하며, 노드는 `seenIntentSet`(seen gossip cache)으로 중복 relay를 차단해야 한다.
2. `expiry_unix`(TTL) 경과 envelope는 저장/relay하지 않는다.
3. `hops`는 `gossip_max_hops`를 초과할 수 없다.

intent-based matcher 규칙:
1. request(intent)당 accept(response) 후보군에서 deterministic 점수(`score_hash`)가 가장 낮은 항목을 우선 선택한다.
2. score 동률 시 `created_unix -> response_id -> signer` 순으로 tie-break한다.
3. 생성된 `match_id`는 request/accept 바인딩 해시로 deterministic해야 한다.
4. expired intent/accept/finalize는 주기적으로 cleanup되어 pending/matcher index에서 제거되어야 한다.
5. matcher 엔진은 shard 병렬화(`MATCHBOARD_MATCHER_SHARDS`)를 사용할 수 있으나, 최종 정렬 결과는 단일 deterministic comparator를 적용해야 한다.

합의 훅(ABCI) 프로파일:
- `PrepareProposal`: local canonical pending operations를 proposal 뒤에 app-injected envelope로 추가 가능 (batch metadata 포함)
- `ProcessProposal`: injected envelope 디코드/순서/operation_id 재계산/`match_certificate` canonical 검증 + optional `MsgSubmitMatchCertificate` payload canonical/binding 검증 + `canonical_batch_hash`(match_result_hash 역할) 재계산 검증 후 ACCEPT/REJECT
- `FinalizeBlock`: injected finalize operation의 `match_certificate`(또는 prebuilt `MsgSubmitMatchCertificate` payload)의 batch를 `SubmitMatchCertificateBatch`로 one-fail-all-rollback 검증하고 성공 시 queue commit
- `FinalizeBlock` 결과는 `matchboard_injected_batch` event(`status`, `operation_count`, `certificate_count`, `canonical_batch_hash`, `canonical_match_build_hash`)로 노출된다.
- `matchboard.proposer-abci.enable=true`일 때만 활성화(기본 비활성)

### 7.2 Client Payload Example (`POST /v1/finalize` with `match_certificate`)

`match_certificate`는 JSON에서 **base64** 문자열로 전달한다(바이너리 bytes 필드).

예시:

```bash
curl -sS -X POST "http://127.0.0.1:8080/v1/finalize" \
  -H "Authorization: Bearer ${MATCHBOARD_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{
    \"pool_id\": \"pool-1\",
    \"intent_id\": \"intent-1\",
    \"response_id\": \"response-1\",
    \"finalize_id\": \"finalize-1\",
    \"sender\": \"0xYourInitiatorAddress\",
    \"recipient\": \"0xResponderAddress\",
    \"expires_unix\": 1767225600,
    \"digest_algorithm\": \"sha256\",
    \"intent_sign_hash\": \"<64-hex>\",
    \"response_sign_hash\": \"<64-hex>\",
    \"finalize_sign_hash\": \"<64-hex>\",
    \"context_hash\": \"<64-hex>\",
    \"initiator_signature\": {
      \"signer\": \"0xYourInitiatorAddress\",
      \"algorithm\": \"secp256k1\",
      \"signature\": \"<130-hex-recoverable-signature>\"
    },
    \"responder_signature\": {
      \"signer\": \"0xResponderAddress\",
      \"algorithm\": \"secp256k1\",
      \"signature\": \"<130-hex-recoverable-signature>\"
    },
    \"match_certificate\": \"<base64-deterministic-protobuf-match-certificate>\"
  }"
```

`match_certificate` 생성 규칙:
1. `match.v1.MatchCertificate` protobuf 메시지를 deterministic marshal한다.
2. marshal bytes를 base64 인코딩한다.
3. payload의 `pool_id/intent_id/response_id/finalize_id` 및 `*_sign_hash/context_hash`는 finalize 요청 본문과 일치해야 한다.

## 8. Validation Rules

### 8.1 Common

1. 필수 ID(`pool_id`, `intent_id`, signer) 존재
2. `expires_unix >= now_unix`
3. `digest_algorithm` 및 `*_hash` 길이 유효성
4. signer 필드와 signature signer 일치
5. sign-doc를 deterministic protobuf로 재직렬화해 `sign_bytes_hash`를 재계산하고 일치해야 한다
6. `secp256k1`는 EVM 주소(`0x...`) signer를 기준으로 `ecrecover` 검증이 성공해야 한다
7. `board_signature.signer`(attestor)와 tx `submitter`(relay fee payer)는 분리될 수 있다

### 8.2 Off-chain Board

1. 중복 `(pool_id, intent_id)` 거부 정책 적용
2. inbox/outbox 조회는 대상자 또는 인증 토큰 소유자만 허용
3. 게시/조회 API에 rate-limit 및 스팸 완화 적용
4. principal이 EVM 주소일 때 `secp256k1` 서명은 `*_sign_hash` 기준으로 즉시 검증해야 한다
5. proposer operation 조회는 canonical ordering을 반환해야 하며 노드 간 결정론이 보장되어야 한다
6. proposer operation 커밋은 all-or-nothing(atomic)이어야 하며, invalid/missing operation이 있으면 전체 롤백해야 한다
7. `match_certificate`가 제공된 finalize artifact는 deterministic protobuf canonical bytes여야 하며 finalize hash/context와 바인딩되어야 한다
8. gossip relay는 TTL + dedup(`message_id`) + hop bound를 강제해야 한다
9. responder discovery는 polling(`outbox`) 대신 subscription(`stream/intents`) 경로를 우선 사용해야 한다
10. expired artifact는 matcher/proposer active set에서 제거되어야 하며, cleanup 이후 stale candidate가 노출되면 안 된다

### 8.3 Optional On-chain Verifier

1. certificate 스키마/버전 지원 여부 확인
2. 서명 세트 완전성, signer-role binding, hash-chaining 확인
3. 만료 확인
4. 중복 제출(리플레이) 거부
5. 이벤트에 평문 민감정보 미포함
6. proposer/batch 경로를 사용할 경우 one-fail-all-rollback 원칙을 유지해야 한다

## 9. Error Codes

| Enum | Suggested Code | 의미 |
|---|---|---|
| `ERROR_CODE_INVALID_REQUEST` | `MCH-1000` | 잘못된 입력/필수 필드 누락 |
| `ERROR_CODE_REPLAY_DETECTED` | `MCH-1202` | 중복 제출 감지 |
| `ERROR_CODE_EXPIRED` | `MCH-1201` | 만료 |
| `ERROR_CODE_INVALID_SIGNATURE` | `MCH-1101` | 서명 검증 실패 |
| `ERROR_CODE_SIGNER_MISMATCH` | `MCH-1102` | signer 역할 불일치 |
| `ERROR_CODE_HASH_MISMATCH` | `MCH-1200` | hash binding 실패 |
| `ERROR_CODE_FORBIDDEN` | `MCH-1300` | inbox/outbox 접근 거부 |
| `ERROR_CODE_RATE_LIMITED` | `MCH-1301` | 요청 제한 |
| `ERROR_CODE_CHAIN_REJECTED` | `MCH-1400` | 온체인 검증 실패 |

참고: `x/match/types`의 envelope 수준 서명 필드 검증(예: signer/algorithm/signature 누락, 미지원 algorithm)은 `ERROR_CODE_INVALID_REQUEST`로 분류한다. `ERROR_CODE_INVALID_SIGNATURE`는 암호학적 서명 검증 실패에 사용한다.

## 10. Privacy and Logging Policy

1. 로그 허용: IDs, hash, 에러코드, latency, correlation_id
2. 로그 금지: 이름/연락처/주소/자유 텍스트 메시지/프로필 원문
3. 시크릿(개인키/API 키/토큰/내부 URL) 커밋 금지
4. 온체인 상태/이벤트는 해시/식별자 중심 최소 공개

### 10.1 Matchboard Proposer Observability Counters (Operational)

Proposer-ABCI 경로에서 운영 관측을 위해 다음 카운터를 기록한다.

1. `matchboard_proposal_reject_total{reason=...}`
   - `ProcessProposal` 단계에서 injected batch를 REJECT 할 때 증가
2. `matchboard_finalize_block_rollback_total{reason=...}`
   - `FinalizeBlock` 단계에서 injected batch를 롤백할 때 증가

`reason` 예시:
- `missing_batch_meta`
- `batch_meta_decode_failed`
- `batch_operation_count_mismatch`
- `batch_hash_mismatch`
- `operation_certificate_invalid`
- `certificate_batch_rejected`

PromQL 예시:

```promql
sum by (reason) (increase(matchboard_proposal_reject_total[5m]))
```

```promql
sum by (reason) (increase(matchboard_finalize_block_rollback_total[5m]))
```

```promql
sum(increase(matchboard_proposal_reject_total{reason="batch_hash_mismatch"}[15m]))
```

운영 권장:
1. `batch_hash_mismatch`는 proposer canonicalization 또는 gossip consistency 이상 신호로 즉시 알림 설정
2. `certificate_batch_rejected` 증가는 `x/match` 검증 규칙/만료/리플레이 정책과의 불일치 가능성 점검
3. 배포 직후 15~30분 동안 rejection/rollback reason 분포를 baseline으로 저장
4. Grafana/Prometheus 패널 템플릿은 `docs/matchboard-observability.md` 참고

## 11. Validation Checklist

- [ ] 동일 입력 -> 동일 sign-bytes 골든 테스트
- [ ] 서명 검증 재현성 테스트
- [ ] JSON signing 거부 테스트
- [ ] `(pool_id, intent_id)` 재제출 거부 테스트
- [ ] 만료 거부 테스트
- [ ] inbox/outbox 접근 권한 테스트
- [ ] proposer canonical ordering 일관성 테스트
- [ ] proposer operation atomic rollback 테스트
- [ ] 체인 이벤트 민감정보 누출 테스트

## 12. Versioning and Compatibility

1. `protocol_version` 필드는 서명 payload에 포함해야 한다.
2. 서명 규격 변경은 breaking change이며 새 메이저 버전이 필요하다.
3. 하위 호환이 필요한 동안 deprecated 필드를 유지한다.
4. breaking change 시 migration 문서와 에러 코드 변경 내역을 동시 제공한다.
