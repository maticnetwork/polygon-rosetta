.PHONY: deps build run lint run-mainnet-online run-mainnet-offline run-testnet-online \
	run-testnet-offline check-comments add-license check-license shorten-lines \
	spellcheck salus build-local format check-format update-tracer test coverage coverage-local \
	update-bootstrap-balances mocks

ADDLICENSE_CMD=go run github.com/google/addlicense
ADDLICENCE_SCRIPT=${ADDLICENSE_CMD} -c "Coinbase, Inc." -l "apache" -v
BUILD_TARGET=bin
BUILD_SOURCE=main.go
SPELLCHECK_CMD=go run github.com/client9/misspell/cmd/misspell
GOLINES_CMD=go run github.com/segmentio/golines
GOLINT_CMD=go run golang.org/x/lint/golint
GOVERALLS_CMD=go run github.com/mattn/goveralls
GOIMPORTS_CMD=go run golang.org/x/tools/cmd/goimports
GO_PACKAGES=./services/... ./cmd/... ./configuration/... ./polygon/...
GO_FOLDERS=$(shell echo ${GO_PACKAGES} | sed -e "s/\.\///g" | sed -e "s/\/\.\.\.//g")
TEST_SCRIPT=go test ${GO_PACKAGES}
LINT_CONFIG=.golangci.yml
PWD=$(shell pwd)
NOFILE=100000

deps:
	go get ./...

test:
	${TEST_SCRIPT}

###############
#### build ####
###############
build-node-local:
	docker-compose -p polygon -f dockerfiles/docker-compose.yml build heimdall bor

build-rosetta-local:
	docker-compose -p polygon -f dockerfiles/docker-compose.yml build rosetta

build-local:
	docker-compose -p polygon -f dockerfiles/docker-compose.yml build

build-rosetta-local-bin:
	mkdir -p $(BUILD_TARGET) && GO build -o $(BUILD_TARGET) $(BUILD_SOURCE)

###############
#### update ###
###############
update-tracer:
	curl https://raw.githubusercontent.com/ethereum/go-ethereum/master/eth/tracers/js/internal/tracers/call_tracer_js.js -o polygon/call_tracer.js

update-bootstrap-balances:
	go run main.go utils:generate-bootstrap polygon/genesis_files/mainnet.json rosetta-cli-conf/mainnet/bootstrap_balances.json;
	go run main.go utils:generate-bootstrap polygon/genesis_files/testnet.json rosetta-cli-conf/testnet/bootstrap_balances.json;

###############
#### run ######
###############
run-node-testnet:
	NETWORK=TESTNET docker-compose -p polygon -f dockerfiles/docker-compose.yml up bor heimdall

run-node-mainnet:
	NETWORK=MAINNET docker-compose -p polygon -f dockerfiles/docker-compose.yml up bor heimdall

run-mainnet-online:
	NETWORK=MAINNET docker-compose -p polygon -f dockerfiles/docker-compose.yml up rosetta

run-mainnet-remote:
	NETWORK=MAINNET BOR=$(bor) docker-compose -p polygon -f dockerfiles/docker-compose.yml up rosetta

run-mainnet-offline:
	NETWORK=MAINNET docker-compose -p polygon -f dockerfiles/docker-compose.yml up rosetta_offline

run-testnet-online:
	NETWORK=TESTNET docker-compose -p polygon -f dockerfiles/docker-compose.yml up rosetta

run-testnet-remote:
	NETWORK=TESTNET BOR=$(bor) docker-compose -p polygon -f dockerfiles/docker-compose.yml up rosetta

run-testnet-offline:
	NETWORK=TESTNET docker-compose -p polygon -f dockerfiles/docker-compose.yml up rosetta_offline

run-bin-mainnet-online:
	NETWORK=MAINNET MODE=ONLINE PORT=3000 BOR=$(bor) bin/main run

run-bin-mainnet-offline:
	NETWORK=MAINNET MODE=OFFLINE PORT=3001 bin/main run

run-bin-testnet-online:
	NETWORK=TESTNET MODE=ONLINE PORT=3000 BOR=$(bor) bin/main run

run-bin-testnet-offline:
	NETWORK=TESTNET MODE=OFFLINE PORT=3001 bin/main run

###############
#### stop #####
###############
down:
	docker-compose -p polygon -f dockerfiles/docker-compose.yml down

check-comments:
	${GOLINT_CMD} -set_exit_status ${GO_FOLDERS} .

lint: | check-comments
	golangci-lint run -v --config $(LINT_CONFIG)

add-license:
	${ADDLICENCE_SCRIPT} .

check-license:
	${ADDLICENCE_SCRIPT} -check .

shorten-lines:
	${GOLINES_CMD} -w --shorten-comments ${GO_FOLDERS} .

format:
	gofmt -s -w -l .
	${GOIMPORTS_CMD} -w .

check-format:
	! gofmt -s -l . | read
	! ${GOIMPORTS_CMD} -l . | read

salus:
	docker run --rm -t -v ${PWD}:/home/repo coinbase/salus

spellcheck:
	${SPELLCHECK_CMD} -error .

coverage:	
	if [ "${COVERALLS_TOKEN}" ]; then ${TEST_SCRIPT} -coverprofile=c.out -covermode=count; ${GOVERALLS_CMD} -coverprofile=c.out -repotoken ${COVERALLS_TOKEN}; fi

coverage-local:
	${TEST_SCRIPT} -cover

mocks:
	rm -rf mocks;
	mockery --dir services --all --case underscore --outpkg services --output mocks/services;
	mockery --dir polygon --all --case underscore --outpkg polygon --output mocks/polygon;
	${ADDLICENCE_SCRIPT} .;

clean:
	rm -rf bin;
	rm -rf cli-data;
	rm -rf polygon-data;