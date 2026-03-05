# Agent: SERVER

## 미션
오프체인 "의사 게시판(CLOB-ish)" 서버를 production-ready로 구현한다.
(기능은 단순, 운영은 단단)

## 담당 산출물
- API:
  - POST /intent
  - POST /response
  - POST /finalize
  - GET /inbox?addr=&pool_id=
  - GET /outbox?addr=&pool_id=
  - GET /cert?pool_id=&intent_id=
- DB 스키마 및 인덱스
- 인증/권한 모델:
  - inbox는 수신자만 조회 가능(토큰/JWT/서명 챌린지 중 택1)
- Rate limiting / spam 방지 옵션
- 운영 문서:
  - 환경변수, 로깅, 마이그레이션, observability(최소)

## 필수 요구사항
- 요청 수신 시 즉시 검증:
  - proto decode
  - 서명 검증
  - 만료 확인
- idempotency:
  - 동일 intent_id 재제출은 안전하게 처리(AlreadyExists)
- 저장은 append-only 성격 유지 + 인덱스 테이블

## 성공 기준
- 데이터 무결성(서명 검증 통과한 것만 저장)
- 대규모 조회(inbox/outbox)에서 인덱스 효율
- 인증 우회 불가

## 금지
- inbox를 공개 검색 가능하게 만들지 말 것
- context 평문 저장/로그 출력