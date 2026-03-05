# Threat Model: Off-Chain Matching System

- Status: Draft
- Last Updated: `2026-03-05`
- Scope: Off-chain intent board + dual signatures + finalize + optional on-chain proof

## 1. Security Objectives

1. 서명 기반 진위성 보장
2. 리플레이/중복 제출 방지
3. 만료 이후 artifact 거부
4. 개인정보/민감정보 평문 비노출
5. deterministic protobuf 기반 검증 재현성 보장
6. inbox/outbox 접근제어 보장

## 2. Assets

| Asset | 보안 요구 | 주요 위협 |
|---|---|---|
| 사용자 개인키 | 기밀성/무결성 | 키 탈취, 악성코드 |
| sign-bytes 규격 | 무결성 | 직렬화 불일치, 파서 혼동 |
| off-chain intent/response/finalize 원문 | 기밀성/무결성 | DB 유출, 내부자 오남용 |
| MatchCertificate | 무결성/유일성 | 리플레이, 중복 제출 |
| `context_hash` | 무결성 | 저엔트로피 데이터 추측 |
| 접근 토큰 | 기밀성 | 토큰 탈취/재사용 |
| 로그/트레이스 | 프라이버시/무결성 | PII 누출, 로그 주입 |

## 3. Actors and Trust

| Actor | 역할 | 신뢰 수준 |
|---|---|---|
| Maker | Intent 작성/서명 | 부분 신뢰 |
| Responder | Response 작성/서명 | 부분 신뢰 |
| Finalizer | Finalize 트리거 | 부분 신뢰 |
| Off-chain operator | API/DB 운영 | 신뢰하지만 검증 필요 |
| On-chain verifier | certificate 검증 | 합의 기반 신뢰 |
| External attacker | 비인가 공격자 | 비신뢰 |

## 4. Trust Boundaries

1. Client key boundary: 개인키는 클라이언트 외부 반출 금지
2. Client-API boundary: TLS 및 인증 컨텍스트
3. API-Storage boundary: 최소권한 + 암호화 저장
4. Signing boundary: deterministic protobuf만 서명 허용
5. Off-chain/On-chain boundary: 증빙 정보만 전달
6. Observability boundary: 로그/메트릭에서 평문 민감정보 제거

## 5. Main Threats and Mitigations

| ID | Threat | 공격 방식 | 완화책 |
|---|---|---|---|
| T1 | 비결정적 직렬화 악용 | 구현별 sign-bytes 불일치 유도 | deterministic marshal 강제, 골든 테스트 |
| T2 | JSON 서명 혼동 | JSON 기반 위조/분쟁 유도 | JSON sign-bytes 전면 거부 |
| T3 | Cross-domain replay | 다른 풀/체인/환경에서 재사용 | `chain_id`,`pool_id`,`intent_id`,`nonce`,`version` 바인딩 |
| T4 | 만료 경계 악용 | 만료 직전/직후 경합 | 엄격한 `expires_unix` 검증, 시간 동기화 |
| T5 | 오프체인 원문 변조 | 운영자/공격자 원문 변경 | `context_hash`/hash-chaining + 양측 서명 |
| T6 | Inbox 무단열람 | IDOR/권한 우회 | 객체 단위 authz, 대상자 스코프 검증 |
| T7 | 로그 기반 개인정보 유출 | request body/PII 로깅 | 구조화 로그 allowlist + redaction 테스트 |
| T8 | 저엔트로피 해시 역추적 | 공개 해시 사전공격 | salt/nonce + domain separation |
| T9 | API 자원 고갈 | 대량 게시/검증 요청 | rate-limit, payload 크기 제한, 백프레셔 |
| T10 | 온체인 중복 제출 | 같은 certificate 반복 제출 | on-chain replay key/중복 거부 |

## 6. Abuse Cases

1. Intent 스팸 대량 게시로 큐/DB 압박
2. inbox/outbox 열거로 메타데이터 수집
3. 만료/중복 경계 조건을 노린 재제출
4. 내부자 로그 조회를 통한 민감정보 수집

## 7. Mandatory Controls

1. 서명 입력은 deterministic protobuf 단일화
2. JSON/임의 직렬화 서명 검증 금지
3. replay 키 강제: `(pool_id,intent_id)` 및 단계별 고유 키
4. `expires_unix` 검증 필수
5. finalize는 양측 서명 모두 검증
6. 체인/로그에 평문 민감정보 금지
7. inbox/outbox 접근제어 강제
8. 비정상 요청 감지 및 제한(ratelimit/anti-spam)

## 8. Logging and Privacy Baseline

허용 필드:
1. 식별자(`pool_id`, `intent_id`, `response_id`, `certificate_id`)
2. 해시(`context_hash`, `sign_bytes_hash`)
3. 에러코드, 상태코드, 처리시간

금지 필드:
1. 이름/전화/이메일/주소
2. 프로필 원문/메시지 원문/비즈니스 조건 원문
3. 개인키/API 키/인증 토큰

## 9. CI Security Test Mapping

1. deterministic sign-bytes 골든 테스트
2. 서명 재현성 및 교차 검증 테스트
3. replay 거부 테스트
4. 만료 거부 테스트
5. inbox/outbox 권한 테스트
6. 로그 민감정보 누출 테스트
7. optional on-chain duplicate rejection 테스트

## 10. Residual Risks

1. 사용자 단말 키 탈취(서버 통제 외)
2. 해시 기반 메타데이터 연관성 분석
3. 오프체인 운영자 검열(지연/드롭)
4. 대규모 분산 DoS

## 11. Open Questions

1. `context_hash` 표준 구성(필드 정렬, salt 정책, 버전 명시)
2. 키 유출 이후 partial flow 취소/폐기 규칙
3. 운영환경에서 요구되는 검열 저항 수준
