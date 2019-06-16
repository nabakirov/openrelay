PACKAGE  = github.com/notegio/openrelay
GOPATH   = $(CURDIR)/.gopath
BASE     = $(GOPATH)/src/$(PACKAGE)
GOSTATIC = go build -a -installsuffix cgo -ldflags '-extldflags "-static"'

all: bin nodesetup truffleCompile docker-cfg/ca-certificates.crt

$(BASE):
	@mkdir -p $(dir $@)
	@ln -sf $(CURDIR) $@

clean: dockerstop
	rm -rf bin/ .gopath/
	rm -rf js/build


dockerstop:
	docker stop `cat "$(BASE)/tmp/redis.containerid"` || true
	docker rm `cat "$(BASE)/tmp/redis.containerid"` || true
	rm $(BASE)/tmp/redis.containerid || true
	docker stop `cat "$(BASE)/tmp/postgres.containerid"` || true
	docker rm `cat "$(BASE)/tmp/postgres.containerid"` || true
	rm $(BASE)/tmp/postgres.containerid || true

nodesetup:
	cd js ; npm install


bin/ingest: $(BASE) cmd/ingest/main.go
	cd "$(BASE)" && $(GOSTATIC) -o bin/ingest cmd/ingest/main.go


bin: bin/ingest 

truffleCompile:
	cd js ; node_modules/.bin/truffle compile

$(BASE)/tmp/redis.containerid:
	mkdir -p $(BASE)/tmp
	docker run -d -p 6379:6379 redis  > $(BASE)/tmp/redis.containerid

$(BASE)/tmp/postgres.containerid:
	mkdir -p $(BASE)/tmp
	docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=secret postgres > $(BASE)/tmp/postgres.containerid

dockerstart: $(BASE) $(BASE)/tmp/redis.containerid $(BASE)/tmp/postgres.containerid

gotest: dockerstart test-funds test-channels test-accounts test-affiliates test-types test-ingest test-blocksmonitor test-allowancemonitor test-fillmonitor test-spendmonitor test-splitter test-search test-db test-metadata test-pool test-ws test-subscriptions

test-funds: $(BASE)
	cd "$(BASE)/funds" && go test
test-channels: $(BASE)
	cd "$(BASE)/channels" &&  REDIS_URL=localhost:6379 go test
test-accounts: $(BASE)
	cd "$(BASE)/accounts" &&  REDIS_URL=localhost:6379 go test
test-affiliates: $(BASE)
	cd "$(BASE)/affiliates" &&  REDIS_URL=localhost:6379 go test
test-types: $(BASE)
	cd "$(BASE)/types" && go test
test-ingest: $(BASE)
	cd "$(BASE)/ingest" && go test
test-blocksmonitor: $(BASE)
	cd "$(BASE)/monitor/blocks" && go test
test-allowancemonitor: $(BASE)
	cd "$(BASE)/monitor/allowance" && go test
test-erc721approval: $(BASE)
	cd "$(BASE)/monitor/erc721approval" && go test
test-canceluptomonitor: $(BASE)
	cd "$(BASE)/monitor/cancelupto" && go test
test-fillmonitor: $(BASE)
	cd "$(BASE)/monitor/fill" && go test
test-spendmonitor: $(BASE)
	cd "$(BASE)/monitor/spend" && go test
test-splitter: $(BASE)
	cd "$(BASE)/splitter" && go test
test-search: $(BASE)
	cd "$(BASE)/search" && POSTGRES_HOST=localhost POSTGRES_USER=postgres POSTGRES_PASSWORD=secret go test
test-affiliate: $(BASE)
	cd "$(BASE)/monitor/affiliate" && go test
test-db: $(BASE)
	cd "$(BASE)/db" &&  POSTGRES_HOST=localhost POSTGRES_USER=postgres POSTGRES_PASSWORD=secret go test
test-metadata: $(BASE)
	cd "$(BASE)/metadata" &&  POSTGRES_HOST=localhost POSTGRES_USER=postgres POSTGRES_PASSWORD=secret go test
test-pool: $(BASE)
	cd "$(BASE)/pool" &&  POSTGRES_HOST=localhost POSTGRES_USER=postgres POSTGRES_PASSWORD=secret go test
test-ws: $(BASE)
	cd "$(BASE)/channels/ws" &&  POSTGRES_HOST=localhost POSTGRES_USER=postgres POSTGRES_PASSWORD=secret go test
test-subscriptions: $(BASE)
	cd "$(BASE)/subscriptions" &&  POSTGRES_HOST=localhost POSTGRES_USER=postgres POSTGRES_PASSWORD=secret go test

docker-cfg/ca-certificates.crt:
	cp /etc/ssl/certs/ca-certificates.crt docker-cfg/ca-certificates.crt

test: $(BASE) $(BASE)/tmp/redis.containerid gotest dockerstop
test_no_docker: mock gotest
mock: $(BASE)
	mkdir -p $(BASE)/tmp
	touch $(BASE)/tmp/redis.containerid
	touch $(BASE)/tmp/postgres.containerid
newvendor:
	govendor add +external

0x-testrpc-snapshot.tar.gz:
	wget https://s3.amazonaws.com/testrpc-shapshots/965d6098294beb22292090c461151274ee6f9a26.zip -O testrpc-db.zip
	mkdir -p /tmp/testrpc-snapshot
	unzip testrpc-db.zip -d /tmp/testrpc-snapshot
	tar -czf 0x-testrpc-snapshot.tar.gz -C /tmp/testrpc-snapshot/0x_ganache_snapshot .
	rm testrpc-db.zip
	rm -rf /tmp/testrpc-snapshot
