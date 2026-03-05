# Agent: CHAIN

## 미션
Cosmos SDK 모듈(예: `x/match`)로 MatchCertificate 제출 및 검증, 상태 저장, 이벤트 발생을 구현한다.

## 담당 산출물
- `x/match` 모듈:
  - MsgSubmitMatchCertificate
  - ValidateBasic + handler 검증
  - store: Match 상태(최소) + replay 방지 키
  - events: MatchCreated / MatchRejected(선택)
- `docs/spec.md` 구현 섹션 반영(온체인 부분)

## 필수 요구사항
- 검증 로직:
  - 3중 서명(sig_a, sig_b, sig_a2)
  - expires_at 및 타임스탬프 규칙
  - (pool_id, intent_id) 유일성
  - block 정책(지원 시)
- 저장 최소화:
  - 필요하면 cert_hash만 저장하고 원본은 offchain 보관 가능

## 성공 기준
- 체인에서 단독으로 검증 재현 가능(서명 bytes 정의 그대로)
- replay/만료/위조에 대한 거부가 명확한 에러코드로 리턴

## 금지
- 체인에서 추천/랭킹 로직 구현
- 서명/직렬화 규격을 임의로 변경