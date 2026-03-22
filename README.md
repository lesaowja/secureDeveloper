# Go Secure Coding Practice

## 현재 상태
- 전체적인 기능이 구현되어 있습니다
- 추후 관리를 편의를 위해 코드 모듈화를 진행했습니다
  - 기본적인 로직 순서로는 middleware - handler - service - ext(db, cache, logger) 로 흐름을 추적하면 됩니다
- traceId 기반으로 로그를 추적하도록 설정되어있습니다 
```
ex) 쉘 명령으로 확인 
cat logs/app.log | grep { 특정 요청의 traceId } 
```

## 주요 API

인증
- `POST /api/auth/register`
- `POST /api/auth/login`
- `POST /api/auth/logout`
- `POST /api/auth/withdraw`

사용자
- `GET /api/me`

게시판
- `GET /api/posts`
- `GET /api/posts/:id`
- `POST /api/posts`
- `PUT /api/posts/:id`
- `DELETE /api/posts/:id`

금융
- `POST /api/banking/deposit`
- `POST /api/banking/withdraw`
- `POST /api/banking/transfer`

## 참고 파일
- `schema.sql`
- `seed.sql`
- `query_examples.sql`

## 실행 방법

프로젝트 루트에서 실행합니다.

```powershell
go run ./cmd/server
```
처음 상태로 다시 시작하고 싶으면 `app.db`를 지운 뒤 다시 실행하면 됩니다.

