main.go :116
store 에 openstore 확인
일반 db.Exec가 아닌 
store.db.Exec가 명령어 

main.go :144
s.db.Exec(`INSERT INTO users (username, name, email, phone, password, balance, is_admin)VALUES (?, ?, ?, ?, ?, 0, 0);`, request.Username, request.Name, request.Email, request.Phone, request.Password)
Sqlinjection을 피하기 위한 ?사용으로 들어가는 내용 자체를 문자열로 인식하게 변환 후 입력

main.go:229
회원 탈퇴 시에 패스워드가 맞는지를 확인하기 위해 
request.Password == user.Password
조건만족시 탈퇴 가능 4번 계정 삭제시 1 2 3 5번 확인 완료

main.go:289
돈 입금 금액이 0원을 넘는지 확인 
if request.Amount > 0
조건에 넘는다면 입금 진행

실시간으로 user에 값이 변하지않고 이전에 지닌값만 비교하기에 장애 발생하여 user가 가져오는 값을 refresh하는 함수 발견
user, ok, err = store.findUserByUsername(user.Username)
이후 실시간으로 금액 늘어나는내용 확인 

main.go:326
출금은 조건 2개가 필요 
request.Amount > 0 출금액이 음수가 되지않을것, 
user.Balance > request.Amount,가진 금액보다 출금을 많이하지 못하게 할것,

main.go:370
user.Balance < request.Amount 내가 가진금액보다 돈을 더 많이 보내려 할때 체크가 필요하고
request.Amount > 0 음수의 금액을 보내진 않는가
마지막으로 해당 유저가 DB상으로 실제로 존재하는가 의 체크가 필요하나 마지막은 능력이 부족하여 하지 못하였습니다. 

main.go:432
위의 users테이블과 다르게 POST posts테이블에 원하는 데이터를 넣고 GET /posts 시에 출력
하기위해 
main.go:613 func (s *Store) findposts(id_ int) (PostView, bool, error) 
을 만들어 posts테이블에 원하는 데이터 삽입 

main.go:411 에서 하나씩 배열로 집어넣고 생성을 하고싶었으나 시간부족으로 개발 불가

성적 평가 이후 나머지의 개발과 아래의 파일로 나누어 가져오고 아직 미처 확인하지 못했던 middleware설정을 하고
현재는 로그인이 안돼어 있어도 authorization token 만을 비교하기에 로그인하지 못하면 다른 링크는 접속못하거나 login으로 연결되게 하는 설정
로그인이 되어있다고 해도 admin만이 조작가능한 세팅들을 추가할 예정에 있습니다. 


생각하던 구조는 SECUREDEVELOPER/cmd/server/main.go
                            /src/sources/AuthHandler.go
                            /           /UserHandler.go
                            /           /Banksystem.go
                            /           /PostHandler.go
                            /
                            /src/js/account.js
                            /      /auth.js
                            /      /baking.js
                            /      /board.js
                            /      /common.js
                            /      /compose.js
                            /      /post.js
                            /      /router.js
                            /
                            /src/etc/index.html
                            /       /style.css
                            /
                            /src/db/app.db
                            /      /query_examples.sql
                            /      /schema.sql
                            /      /seed.sql
                            /
                            /logs/all/ALL.log


이런식으로 관리 할 것 같습니다. 