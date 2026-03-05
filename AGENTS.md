# AGENTS.md

이 저장소는 "오프체인 의사 게시 + 양방향 서명 + 최종 서명 finalize + (선택) 온체인 증빙" 기반의 범용 매칭 시스템을 구현한다.

## 0) 절대 규칙 (Hard rules)
1. **직렬화/서명 규격은 단 하나로 고정한다.**
   - 기본: **Protobuf + deterministic marshaling**(Deterministic=true) 기반 sign-bytes.
   - JSON 문자열/임의 직렬화에 서명 금지.
2. **개인정보/민감정보 평문 저장 금지.**
   - 프로필/메시지/조건 등은 오프체인 저장소에 두고, 체인/로그에는 `context_hash` 등 해시만 기록.
3. **시크릿 커밋 금지.**
   - 개인키, 토큰, API 키, 내부 URL, 인증서 등은 커밋하면 즉시 실패로 간주.
   - `.env.example`만 허용.
4. **테스트/린트/타입체크 통과 전엔 머지 불가.**
   - CI가 실패하면 작업은 "미완료"다.
5. **문서가 단일 소스 오브 트루스(SSOT).**
   - `docs/spec.md`(프로토콜/검증/에러코드), `proto/*`가 제품의 기준이다.
   - 구현이 문서와 다르면 문서를 먼저/함께 수정한다.

## 1) 브랜치/PR 규칙
- `main` 직접 커밋 금지. `feature/*`, `fix/*`, `chore/*` 브랜치 사용.
- PR에는 아래가 필수:
  - 변경 요약 (3~8줄)
  - 테스트 결과 (`make test` / `make lint` / `make typecheck`)
  - 위험/호환성 영향 (breaking change 여부)
  - 관련 문서/스펙 변경 링크

## 2) 프로젝트 범위 (Scope)
### 오프체인
- Intent 게시, Response 서명 응답, Finalize 서명, MatchCertificate 구성/조회
- Inbox/Outbox 조회(수신자 접근제어 포함)
- Rate-limit / 스팸 방지 옵션

### 온체인(선택)
- MatchCertificate 제출 메시지
- 서명/만료/중복 검증 후 Match 생성 이벤트 emit

## 3) 공통 산출물 포맷
- 설계/스펙: `docs/*.md`
- Protobuf: `proto/match/*.proto`
- 서버: `server/*` (언어/프레임워크는 레포 표준을 따른다)
- 체인 모듈: `x/match/*` (Cosmos SDK 관례 준수)
- 테스트: 단위/통합/E2E 포함

## 4) 공통 품질 게이트
- 골든 테스트:
  - 동일 입력 -> 동일 sign-bytes
  - 서명 검증 재현성
- 리플레이 방지 테스트:
  - 동일 `(pool_id, intent_id)` 재제출 거부
- 만료 테스트:
  - `expires_at` 이후 cert 거부
- 권한 테스트:
  - inbox는 대상만 조회 가능(또는 인증 토큰 필요)

## 5) 의사결정 우선순위
1) 보안/프라이버시
2) 결정론(서명/직렬화)
3) 확장성(풀/정책 분리)
4) UX(서명 흐름 간소화는 옵션)

## 6) 에이전트 역할
- Architect: 전체 아키텍처/모듈 경계/데이터 모델/SSOT 문서
- Protocol: proto, sign-bytes, 상태 머신, 에러코드
- Server: 오프체인 게시판 API/DB/권한/레이트리밋
- Chain: Cosmos SDK 모듈/Msg/검증/이벤트
- QA: 테스트 전략/퍼징/E2E/CI
- Security: 위협모델/악용시나리오/완화책
- Release: 릴리즈 체크리스트/버전/마이그레이션 가이드