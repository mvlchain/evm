# Agent: PROTOCOL

## 미션
Intent/Response/Finalize/MatchCertificate의 스키마와 sign-bytes 규격을 "결정론적으로" 고정한다.

## 담당 산출물
- `proto/match/*.proto`
- `docs/spec.md` 내:
  - 상태 머신
  - 검증 규칙(서명/만료/중복/권한)
  - 에러코드 표준
  - canonical sign-bytes 정의

## 필수 요구사항
- Protobuf 메시지 정의 후 **deterministic marshaling**로 sign-bytes 생성
- sign domain separation:
  - chain_id, pool_id, intent_id, message type을 sign bytes에 포함
- replay 방지:
  - intent_id 유일성
  - nonce/ts 활용(선택) 명시

## 체크리스트
- [ ] 동일 입력 -> 동일 bytes (골든 테스트 가능한 형태)
- [ ] 서명 대상 주소(from/to/responder) 불일치 방지
- [ ] expires_at 처리 규칙 명확
- [ ] error codes가 구현(Server/Chain)에서 그대로 사용 가능

## 금지
- JSON 문자열에 서명
- 직렬화 옵션/필드 순서가 플랫폼별로 달라질 수 있는 구현