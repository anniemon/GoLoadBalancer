## Load balancer with rate limits in GO

### Run

```go
go run .
```

### Test

- 테스트 코드로 테스트
```go
go test .
```

- curl을 이용하여 테스트
```shell
for i in {1..15}; do curl -X GET http://localhost:8080; done
```

또는
```shell
for i in {1..15}; do curl -X POST http://localhost:8080 -d "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."; done
```

더미 노드 서버의 스펙은 다음과 같이 하드코딩되어 있습니다.
```GO
[
  { ID: 1, URL: "http://localhost:8081", ReqLimit: 2, BodyLimit: 123},
  { ID: 2, URL: "http://localhost:8082", ReqLimit: 5, BodyLimit: 2 * 1024 * 1024},
  { ID: 3, URL: "http://localhost:8083", ReqLimit: 7, BodyLimit: 1 * 1024 * 1024}
]
```

따라서 위의 curl POST 커맨드를 입력하면 아래와 같은 출력이 나옵니다.

```shell
node1 on port http://localhost:8081
node2 on port http://localhost:8082
node3 on port http://localhost:8083
node2 on port http://localhost:8082
node3 on port http://localhost:8083
node2 on port http://localhost:8082
node3 on port http://localhost:8083
node2 on port http://localhost:8082
node3 on port http://localhost:8083
node2 on port http://localhost:8082
node3 on port http://localhost:8083
node3 on port http://localhost:8083
node3 on port http://localhost:8083
No available node
No available node
```

8080번 포트에서 실행되는 로드밸런서 서버의 로그를 통해 아래와 같은 동작을 확인할 수도 있습니다.
```console
2024/08/08 22:53:49 Starting node 2 on port 8082
2024/08/08 22:53:49 Starting node 3 on port 8083
2024/08/08 22:53:49 Starting node 1 on port 8081
Load Balancer running on :8080
2024/08/08 22:53:51 Forwarding request to node 1 (http://localhost:8081) - RPM: 1/2, BPM: 0/123
2024/08/08 22:53:51 Forwarding request to node 2 (http://localhost:8082) - RPM: 1/5, BPM: 0/2097152
2024/08/08 22:53:51 Forwarding request to node 3 (http://localhost:8083) - RPM: 1/7, BPM: 0/1048576
2024/08/08 22:53:51 Forwarding request to node 1 (http://localhost:8081) - RPM: 2/2, BPM: 0/123
2024/08/08 22:53:51 Forwarding request to node 2 (http://localhost:8082) - RPM: 2/5, BPM: 0/2097152
2024/08/08 22:53:51 Forwarding request to node 3 (http://localhost:8083) - RPM: 2/7, BPM: 0/1048576
2024/08/08 22:53:51 Rate limit hit for node 1 (http://localhost:8081) - RPM: 2/2, BodyLimit: 123, RequestBody: 0
2024/08/08 22:53:51 Forwarding request to node 2 (http://localhost:8082) - RPM: 3/5, BPM: 0/2097152
2024/08/08 22:53:51 Forwarding request to node 3 (http://localhost:8083) - RPM: 3/7, BPM: 0/1048576
2024/08/08 22:53:51 Rate limit hit for node 1 (http://localhost:8081) - RPM: 2/2, BodyLimit: 123, RequestBody: 0
2024/08/08 22:53:51 Forwarding request to node 2 (http://localhost:8082) - RPM: 4/5, BPM: 0/2097152
2024/08/08 22:53:51 Forwarding request to node 3 (http://localhost:8083) - RPM: 4/7, BPM: 0/1048576
2024/08/08 22:53:51 Rate limit hit for node 1 (http://localhost:8081) - RPM: 2/2, BodyLimit: 123, RequestBody: 0
2024/08/08 22:53:51 Forwarding request to node 2 (http://localhost:8082) - RPM: 5/5, BPM: 0/2097152
2024/08/08 22:53:51 Forwarding request to node 3 (http://localhost:8083) - RPM: 5/7, BPM: 0/1048576
2024/08/08 22:53:51 Rate limit hit for node 1 (http://localhost:8081) - RPM: 2/2, BodyLimit: 123, RequestBody: 0
2024/08/08 22:53:51 Rate limit hit for node 2 (http://localhost:8082) - RPM: 5/5, BodyLimit: 2097152, RequestBody: 0
2024/08/08 22:53:51 Forwarding request to node 3 (http://localhost:8083) - RPM: 6/7, BPM: 0/1048576
2024/08/08 22:53:51 Rate limit hit for node 1 (http://localhost:8081) - RPM: 2/2, BodyLimit: 123, RequestBody: 0
2024/08/08 22:53:51 Rate limit hit for node 2 (http://localhost:8082) - RPM: 5/5, BodyLimit: 2097152, RequestBody: 0
2024/08/08 22:53:51 Forwarding request to node 3 (http://localhost:8083) - RPM: 7/7, BPM: 0/1048576
2024/08/08 22:53:51 Rate limit hit for node 1 (http://localhost:8081) - RPM: 2/2, BodyLimit: 123, RequestBody: 0
2024/08/08 22:53:51 Rate limit hit for node 2 (http://localhost:8082) - RPM: 5/5, BodyLimit: 2097152, RequestBody: 0
2024/08/08 22:53:51 Rate limit hit for node 3 (http://localhost:8083) - RPM: 7/7, BodyLimit: 1048576, RequestBody: 0
2024/08/08 22:53:51 No available node
```

### How it works

#### Node
1. 각 노드의 rate limit은 하드코딩되어 있습니다. main.go에서 변경할 수 있습니다.
2. 각 노드의 분당 요청수(RPM) 또는 분당 요청 바디의 크기(BPM)는 매분마다 리셋됩니다.
3. 각 노드는 30초에 한 번씩 헬스 체크를 진행하며, 테스트를 위해 헬스 체크는 노드마다 약 30%의 확률로 실패합니다.
4. 각 노드가 바라보는 서버는 backend_server.go에서 확인할 수 있습니다. 루트와 `/health`라는 두 개의 엔드포인트를 갖습니다.

#### Load balancer
1. 로드밸런서는 라운드로빈 방식으로 동작합니다.
2. 선택한 노드가 healthy하다면 이를 반환하고, 아니면 다음 노드를 찾습니다.
3. 각 노드의 RPM이나 BPM 중 하나가 한도에 도달하면 rate limit이 초과되므로, 다음 노드를 찾습니다.
4. 모든 노드가 가용하지 않다면, 에러 메시지와 함께 503 상태 코드를 반환합니다.
